package server

import (
	"bytes"
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
	"github.com/frizinak/inbetween-go-homecam/protocol"
	"github.com/frizinak/inbetween-go-homecam/vars"
)

type Resolution struct {
	width  uint32
	height uint32
}

func (r Resolution) Resolution() uint32 {
	return r.width * r.height
}

type Config interface {
	MinimumFPS() int
	MaximumFPS() int

	MinimumJPEGQuality() int
	MaximumJPEGQuality() int

	DesiredTotalThroughput() float64
	DesiredClientThroughput() float64

	MinimumResolution() int
	MaximumResolution() int
}

type qualityConfig struct {
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

	sem sync.Mutex

	net struct {
		addr     string
		pass     []byte
		maxPeers int

		data []byte

		clients int
		peers   int
		bytes   uint64
		since   time.Time

		proto *protocol.Protocol
	}

	cam struct {
		reinit      bool
		device      string
		cam         *webcam.Webcam
		activeRes   int
		resolutions []Resolution
	}

	frameCount               uint64
	fps                      int
	jpegOpts                 *jpeg.Options
	lastResolutionAdjustment time.Time
	lastAdjustment           time.Time

	quality qualityConfig

	scryptRatelimit chan struct{}
}

func New(
	l *log.Logger,
	addr string,
	pass []byte,
	device string,
	quality Config,
	maxPeers int,
) *Server {
	q := qualityConfig{
		MinFPS:                  quality.MinimumFPS(),
		MaxFPS:                  quality.MaximumFPS(),
		MinJPEGQuality:          quality.MinimumJPEGQuality(),
		MaxJPEGQuality:          quality.MaximumJPEGQuality(),
		DesiredTotalThroughput:  quality.DesiredTotalThroughput(),
		DesiredClientThroughput: quality.DesiredClientThroughput(),
		MinResolution:           quality.MinimumResolution(),
		MaxResolution:           quality.MaximumResolution(),
	}

	s := &Server{
		l:               l,
		fps:             q.MaxFPS,
		jpegOpts:        &jpeg.Options{Quality: q.MaxJPEGQuality},
		quality:         q,
		scryptRatelimit: make(chan struct{}, 1),
	}

	s.cam.device = device
	s.net.addr = addr
	s.net.maxPeers = maxPeers
	s.net.since = time.Now()
	s.net.pass = pass
	s.net.proto = protocol.New(
		vars.HandshakeCost,
		vars.EncryptCost,
		vars.HandshakeLen,
		vars.HandshakeHashLen,
	)

	return s
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
	if s.cam.cam != nil {
		if err = s.cam.cam.Close(); err != nil {
			return err
		}
	}

	s.cam.cam, err = webcam.Open(s.cam.device)
	if err != nil {
		return err
	}

	formats := s.cam.cam.GetSupportedFormats()
	// TODO
	// get first one for now
	var pix webcam.PixelFormat
	for i := range formats {
		pix = i
		break
	}

	if s.cam.resolutions == nil {
		sizes := s.cam.cam.GetSupportedFrameSizes(pix)
		s.cam.resolutions = make([]Resolution, 0, len(sizes))
		for i := range sizes {
			res := int(sizes[i].MaxWidth * sizes[i].MinHeight)
			if res < s.quality.MinResolution || res > s.quality.MaxResolution {
				continue
			}
			s.cam.resolutions = append(
				s.cam.resolutions,
				Resolution{sizes[i].MaxWidth, sizes[i].MinHeight},
			)
		}
		s.cam.activeRes = len(s.cam.resolutions) - 1

		if len(s.cam.resolutions) == 0 {
			for i := range sizes {
				s.l.Printf(
					"%dx%d = %d",
					sizes[i].MaxWidth,
					sizes[i].MinHeight,
					sizes[i].MaxWidth*sizes[i].MinHeight,
				)
			}
			return errors.New("No resolutions found, try adjusting the min/max requirments")
		}

		sort.Slice(s.cam.resolutions, func(i, j int) bool {
			return s.cam.resolutions[i].Resolution() < s.cam.resolutions[j].Resolution()
		})
	}

	_, _, _, err = s.cam.cam.SetImageFormat(
		pix,
		s.cam.resolutions[s.cam.activeRes].width,
		s.cam.resolutions[s.cam.activeRes].height,
	)

	if err != nil {
		return err
	}

	return s.cam.cam.StartStreaming()
}

func (s *Server) connErr(err error) {
	if err != io.EOF {
		s.l.Println(err)
	}
}

func (s *Server) addBytes(bytes uint64) {
	s.sem.Lock()
	s.net.bytes += bytes
	since := time.Since(s.net.since).Seconds()
	iv := 3.0

	if time.Since(s.lastAdjustment).Seconds() < 2*iv {
		s.sem.Unlock()
		return
	}

	if since > iv {
		throughput := float64(s.net.bytes) / since
		s.net.since = time.Now()
		s.net.bytes = 0

		oFPS := s.fps
		oQuality := s.jpegOpts.Quality
		oActiveRes := s.cam.activeRes

		desired := s.quality.DesiredTotalThroughput
		desiredClient := float64(s.net.clients) * s.quality.DesiredClientThroughput
		if desiredClient < desired {
			desired = desiredClient
		}

		factor := throughput / desired
		if factor < 0.01 {
			factor = 0.01
		} else if factor > 20 {
			factor = 20
		}

		switch {
		case factor > 1.05 &&
			s.fps == s.quality.MinFPS &&
			s.jpegOpts.Quality == s.quality.MinJPEGQuality:

			if s.cam.activeRes > 0 {
				s.cam.activeRes--
			}

		case time.Since(s.lastResolutionAdjustment).Seconds() > 30 &&
			factor < 0.9 &&
			s.fps == s.quality.MaxFPS &&
			s.jpegOpts.Quality == s.quality.MaxJPEGQuality:

			if s.cam.activeRes+1 < len(s.cam.resolutions) {
				// TODO
				// very very rough guess to see if upscaling will not
				// immediately lead to a downscale
				current := s.cam.resolutions[s.cam.activeRes].Resolution()
				next := s.cam.resolutions[s.cam.activeRes+1].Resolution()
				result := float64(current) / float64(next)

				fps := float64(s.quality.MinFPS) / float64(s.quality.MaxFPS)
				q := float64(s.quality.MinJPEGQuality) / float64(s.quality.MaxJPEGQuality)
				start := factor * fps * q

				if result > start {
					s.fps = int(start / result * float64(s.fps))
					s.jpegOpts.Quality = int(start / result * float64(s.jpegOpts.Quality))
					s.cam.activeRes++
				}
			}

		case factor > 1.05:
			s.fps /= int(factor)
			if s.fps <= s.quality.MinFPS+(s.quality.MaxFPS-s.quality.MinFPS)/2 {
				s.jpegOpts.Quality = int(float64(s.jpegOpts.Quality) / factor)
			}

		case factor < 0.9:
			if s.fps <= s.quality.MinFPS+(s.quality.MaxFPS-s.quality.MinFPS)/2 ||
				s.jpegOpts.Quality >= s.quality.MaxJPEGQuality {
				s.fps += int(1 / factor)
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

		if oActiveRes != s.cam.activeRes {
			s.cam.reinit = true
		}

		if s.fps != oFPS ||
			s.jpegOpts.Quality != oQuality ||
			oActiveRes != s.cam.activeRes {

			s.lastAdjustment = time.Now()
			if oActiveRes != s.cam.activeRes {
				s.lastResolutionAdjustment = time.Now()
			}

			s.l.Printf(
				"%.1fkB/s throughput => Quality adjustment: %dx%d @ %dfps (jpeg: %d)",
				throughput/1024,
				s.cam.resolutions[s.cam.activeRes].width,
				s.cam.resolutions[s.cam.activeRes].height,
				s.fps,
				s.jpegOpts.Quality,
			)
		}
	}
	s.sem.Unlock()
}

func (s *Server) addClient(amount int) {
	s.sem.Lock()
	s.net.clients += amount
	s.sem.Unlock()
}

func (s *Server) addPeer(amount int) error {
	s.sem.Lock()
	if s.net.peers+amount > s.net.maxPeers {
		s.sem.Unlock()
		return fmt.Errorf("Max number of peers reached (%d)", s.net.maxPeers)
	}

	s.net.peers += amount
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

	common := make([]byte, len(vars.CommonSecret))
	copy(common, vars.CommonSecret)
	common = append(common, s.net.pass...)
	s.scryptRatelimit <- struct{}{}
	crypter, err := s.net.proto.HandshakeServer(common, c)
	if err != nil {
		<-s.scryptRatelimit
		s.connErr(err)
		return
	}
	<-s.scryptRatelimit

	s.addClient(1)
	defer s.addClient(-1)
	s.l.Println("New client", c.RemoteAddr())
	if err != nil {
		s.connErr(err)
		return
	}

	w := newCountWriter(c)
	var nbytes uint64

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

		w.Reset()
		if frame == s.frameCount {
			if _, err = w.Flush([]byte{0, 0, 0}); err != nil {
				s.connErr(err)
				return
			}

			time.Sleep(time.Millisecond * 50)
			continue
		}

		frame = s.frameCount
		err = crypter.Encrypt(bytes.NewBuffer(s.net.data), w)
		if err != nil {
			s.connErr(err)
			return
		}

		nbytes, err = w.Flush(nil)
		if err != nil {
			s.connErr(err)
			return
		}
		s.addBytes(nbytes)
	}
}

func (s *Server) Listen(output <-chan *bytes.Buffer) error {
	ln, err := net.Listen("tcp", s.net.addr)
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
			s.net.data = d.Bytes()
			s.frameCount++
			if s.frameCount > 1e16 {
				s.frameCount = 0
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
		s.cam.reinit = true
		for {
			if s.cam.reinit {
				s.cam.reinit = false
				s.initCam()
			}

			err := s.cam.cam.WaitForFrame(1)
			switch err.(type) {
			case nil:
			case *webcam.Timeout:
				continue
			default:
				s.l.Printf("Failed waiting for cam frame: %s", err)
				s.cam.reinit = true
				continue
			}

			if time.Since(last) < time.Second/time.Duration(s.fps) {
				s.cam.cam.ReadFrame()
				continue
			}

			_d, err := s.cam.cam.ReadFrame()
			if err != nil {
				s.l.Printf("Failed reading cam frame: %s", err)
				s.cam.reinit = true
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
