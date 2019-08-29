package server

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/blackjack/webcam"
	"github.com/frizinak/inbetween-go-homecam/crypto"
	"golang.org/x/crypto/scrypt"
)

var CommonSecret = []byte("HelloThereCamServer")

const (
	EncryptCost      = 17
	HandshakeCost    = 12
	HandshakeLen     = 128
	HandshakeHashLen = 256
)

type QualityConfig struct {
	MinFPS int
	MaxFPS int

	MinJPEGQuality int
	MaxJPEGQuality int

	DesiredTotalThroughput  float64
	DesiredClientThroughput float64

	MinResolution int
	MaxResolution int
}

type Server struct {
	l *log.Logger

	addr     string
	maxPeers int

	pass   []byte
	secret []byte

	reinitCam   bool
	device      string
	cam         *webcam.Webcam
	activeRes   int
	resolutions []webcam.FrameSize
	data        []byte

	frameCount uint64

	fps      int
	jpegOpts *jpeg.Options

	sem        sync.Mutex
	clients    int
	peers      int
	bytes      uint64
	since      time.Time
	throughput float64

	quality QualityConfig
}

func New(
	l *log.Logger,
	addr string,
	pass []byte,
	device string,
	quality QualityConfig,
	maxPeers int,
) *Server {
	return &Server{
		l:        l,
		addr:     addr,
		pass:     pass,
		device:   device,
		fps:      quality.MaxFPS,
		jpegOpts: &jpeg.Options{Quality: quality.MinJPEGQuality},
		since:    time.Now(),
		quality:  quality,
		maxPeers: maxPeers,
	}
}

func (s *Server) initCam() {
	var last time.Time
	for {
		err := s.tryInitCam()
		if err == nil {
			break
		}

		if time.Since(last) > time.Second*10 {
			last = time.Now()
			s.l.Printf("Initiating cam failed: %s, will keep trying", err)
		}
		time.Sleep(time.Second)
	}
}

func (s *Server) tryInitCam() error {
	var err error
	if s.cam != nil {
		if err = s.cam.Close(); err != nil {
			return err
		}
	}

	s.cam, err = webcam.Open(s.device)
	if err != nil {
		return err
	}

	formats := s.cam.GetSupportedFormats()
	// TODO
	// get first one for now
	var pix webcam.PixelFormat
	for i := range formats {
		pix = i
		break
	}

	if s.resolutions == nil {
		sizes := s.cam.GetSupportedFrameSizes(pix)
		s.resolutions = make([]webcam.FrameSize, 0, len(sizes))
		for i := range sizes {
			res := int(sizes[i].MaxWidth * sizes[i].MinHeight)
			if res < s.quality.MinResolution || res > s.quality.MaxResolution {
				continue
			}
			s.resolutions = append(s.resolutions, sizes[i])
		}
		s.activeRes = 0

		if len(s.resolutions) == 0 {
			for i := range sizes {
				s.l.Printf("%dx%d = %d", sizes[i].MaxWidth, sizes[i].MinHeight, sizes[i].MaxWidth*sizes[i].MinHeight)
			}
			return errors.New("No resolutions found, try adjusting the min/max requirments")
		}

		sort.Slice(s.resolutions, func(i, j int) bool {
			a, b := s.resolutions[i], s.resolutions[j]
			return a.MaxWidth*a.MinHeight > b.MaxWidth*b.MinHeight
		})
	}

	_, _, _, err = s.cam.SetImageFormat(
		pix,
		s.resolutions[s.activeRes].MaxWidth,
		s.resolutions[s.activeRes].MaxHeight,
	)

	if err != nil {
		return err
	}

	return s.cam.StartStreaming()
}

func (s *Server) connErr(err error) {
	if err != io.EOF {
		s.l.Println(err)
	}
}

func (s *Server) addBytes(bytes uint64) {
	s.sem.Lock()
	s.bytes += bytes
	since := time.Since(s.since).Seconds()
	if since > 3 {
		s.throughput = float64(s.bytes) / since
		s.since = time.Now()
		s.bytes = 0

		oFPS := s.fps
		oQuality := s.jpegOpts.Quality
		oActiveRes := s.activeRes

		desired := s.quality.DesiredTotalThroughput
		desiredClient := float64(s.clients) * s.quality.DesiredClientThroughput
		if desiredClient < desired {
			desired = desiredClient
		}

		factor := s.throughput / desired
		if factor < 0.01 {
			factor = 0.01
		} else if factor > 5 {
			factor = 5
		}

		if factor > 1.05 && s.fps == s.quality.MinFPS && s.jpegOpts.Quality == s.quality.MinJPEGQuality {
			s.activeRes++
		} else if factor < 0.9 && s.fps == s.quality.MaxFPS && s.jpegOpts.Quality == s.quality.MaxJPEGQuality {
			s.activeRes--
		} else if factor > 1.05 {
			if s.fps <= s.quality.MinFPS+(s.quality.MaxFPS-s.quality.MinFPS)/2 {
				s.jpegOpts.Quality = int(float64(s.jpegOpts.Quality) / factor)
			}
			//s.fps = int(float64(s.fps) / (factor / 2))
			s.fps -= int(factor)
		} else if factor < 0.9 {
			if s.fps <= s.quality.MinFPS+(s.quality.MaxFPS-s.quality.MinFPS)/2 ||
				s.jpegOpts.Quality >= s.quality.MaxJPEGQuality {
				s.fps++
			}

			s.jpegOpts.Quality = int(float64(s.jpegOpts.Quality) / factor)
		}

		if s.jpegOpts.Quality < s.quality.MinJPEGQuality {
			s.jpegOpts.Quality = s.quality.MinJPEGQuality
		} else if s.jpegOpts.Quality > s.quality.MaxJPEGQuality {
			s.jpegOpts.Quality = s.quality.MaxJPEGQuality
		}

		if s.fps < s.quality.MinFPS {
			s.fps = s.quality.MinFPS
		} else if s.fps > s.quality.MaxFPS {
			s.fps = s.quality.MaxFPS
		}

		if s.activeRes < 0 {
			s.activeRes = 0
		} else if s.activeRes >= len(s.resolutions) {
			s.activeRes = len(s.resolutions) - 1
		}

		if oActiveRes != s.activeRes {
			s.reinitCam = true
		}

		if s.fps != oFPS ||
			s.jpegOpts.Quality != oQuality ||
			oActiveRes != s.activeRes {
			s.l.Printf(
				"%.1fkB/s throughput => Quality adjustment: %dx%d @ %dfps (jpeg: %d)",
				s.throughput/1024,
				s.resolutions[s.activeRes].MaxWidth,
				s.resolutions[s.activeRes].MinHeight,
				s.fps,
				s.jpegOpts.Quality,
			)
		}
	}
	s.sem.Unlock()
}

func (s *Server) addClient(amount int) {
	s.sem.Lock()
	s.clients += amount
	s.sem.Unlock()
}

func (s *Server) addPeer(amount int) error {
	s.sem.Lock()
	if s.peers+amount > s.maxPeers {
		s.sem.Unlock()
		return fmt.Errorf("Max number of peers reached (%d)", s.maxPeers)
	}

	s.peers += amount
	s.sem.Unlock()
	return nil
}

func (s *Server) conn(c net.Conn) {
	b := make([]byte, 1)
	var frame uint64 = 0
	defer c.Close()
	if err := s.addPeer(1); err != nil {
		s.connErr(err)
		return
	}
	defer s.addPeer(-1)

	if err := c.SetDeadline(time.Now().Add(time.Second * 5)); err != nil {
		s.connErr(err)
		return
	}

	handshake := make([]byte, HandshakeLen)
	if _, err := rand.Read(handshake); err != nil {
		s.connErr(err)
		return
	}

	handshakeHash, err := scrypt.Key(
		CommonSecret,
		handshake,
		1<<HandshakeCost,
		8,
		1,
		HandshakeHashLen,
	)
	if err != nil {
		s.connErr(err)
		return
	}

	if _, err := c.Write(handshake); err != nil {
		s.connErr(err)
		return
	}

	remoteHandshakeHash := make([]byte, HandshakeHashLen)
	if _, err := io.ReadFull(c, remoteHandshakeHash); err != nil {
		s.connErr(err)
		return
	}

	if !bytes.Equal(remoteHandshakeHash, handshakeHash) {
		s.l.Println("Invalid handshake", c.RemoteAddr())
		return
	}

	s.addClient(1)
	defer s.addClient(-1)
	s.l.Println("New client", c.RemoteAddr())

	pass := append(handshakeHash, s.pass...)
	crypter, err := crypto.NewImmutableKeyEncrypter(pass, 60, EncryptCost)
	if err != nil {
		s.connErr(err)
		return
	}

	for {
		if err := c.SetDeadline(time.Now().Add(time.Second * 5)); err != nil {
			s.connErr(err)
			return
		}

		_, err := c.Read(b)
		if err != nil {
			s.connErr(err)
			return
		}

		if frame == s.frameCount {
			w := &countWriter{n: 0, w: c}
			if _, err = w.Write([]byte{0, 0, 0}); err != nil {
				s.connErr(err)
				return
			}

			if err = w.Flush(); err != nil {
				s.connErr(err)
				return
			}

			time.Sleep(time.Millisecond * 50)
			continue
		}

		frame = s.frameCount
		w := &countWriter{n: 0, w: c}
		err = crypter.Encrypt(bytes.NewBuffer(s.data), w)
		if err != nil {
			s.connErr(err)
			return
		}

		s.addBytes(uint64(w.n))
		if err = w.Flush(); err != nil {
			s.connErr(err)
			return
		}
	}
}

func (s *Server) Listen(output <-chan *bytes.Buffer) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		for d := range output {
			if s.jpegOpts.Quality < 100 {
				i, err := jpeg.Decode(d)
				if err != nil {
					s.l.Println(err)
					continue
				}
				d.Reset()
				if err := jpeg.Encode(d, i, s.jpegOpts); err != nil {
					s.l.Println(err)
					continue
				}
			}
			s.data = d.Bytes()
			s.frameCount++
			if s.frameCount > 1e16 {
				s.frameCount += 0
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			conn.Close()
			s.connErr(err)
			continue
		}

		go s.conn(conn)
	}
}

func (s *Server) Start() (<-chan *bytes.Buffer, <-chan error) {
	errs := make(chan error)
	output := make(chan *bytes.Buffer, 1)
	var last time.Time
	go func() {
		s.reinitCam = true
		for {
			if s.reinitCam {
				s.reinitCam = false
				s.initCam()
			}

			err := s.cam.WaitForFrame(1)
			switch err.(type) {
			case nil:
			case *webcam.Timeout:
				continue
			default:
				s.l.Printf("Failed waiting for cam frame: %s", err)
				s.reinitCam = true
				continue
			}

			if time.Since(last) < time.Second/time.Duration(s.fps) {
				s.cam.ReadFrame()
				continue
			}

			_d, err := s.cam.ReadFrame()
			if err != nil {
				s.l.Printf("Failed reading cam frame: %s", err)
				s.reinitCam = true
				continue
			}
			d := make([]byte, len(_d))
			copy(d, _d)

			last = time.Now()
			output <- bytes.NewBuffer(d)
		}
	}()

	return output, errs
}
