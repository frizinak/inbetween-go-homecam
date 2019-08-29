package client

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/frizinak/inbetween-go-homecam/crypto"
	"github.com/frizinak/inbetween-go-homecam/server"
	"golang.org/x/crypto/scrypt"
)

type Client struct {
	l    *log.Logger
	addr string
	pass []byte

	w []byte

	bytesRead int
	since     time.Time

	maxThroughput int
}

func New(l *log.Logger, addr string, pass []byte, maxThroughput int) *Client {
	return &Client{
		l:             l,
		addr:          addr,
		pass:          pass,
		w:             []byte{0},
		maxThroughput: maxThroughput,
	}
}

func (c *Client) connErr(err error) {
	if err != io.EOF {
		c.l.Println(err)
	}
}

type Data struct {
	*bytes.Buffer
	Created time.Time
}

func (c *Client) Connect(data chan<- *Data) error {
	var conn net.Conn
	var connErr error
	for {
		if conn != nil {
			conn.Close()
			if connErr != nil {
				return connErr
			}

			time.Sleep(time.Second)
		}

		var err error
		conn, err = net.Dial("tcp", c.addr)
		if err != nil {
			continue
		}

		handshake := make([]byte, server.HandshakeLen)
		if _, connErr = io.ReadFull(conn, handshake); connErr != nil {
			continue
		}

		var handshakeHash []byte
		handshakeHash, connErr = scrypt.Key(
			server.CommonSecret,
			handshake,
			1<<server.HandshakeCost,
			8,
			1,
			server.HandshakeHashLen,
		)
		if connErr != nil {
			continue
		}

		if _, connErr = conn.Write(handshakeHash); connErr != nil {
			continue
		}

		pass := append(handshakeHash, c.pass...)
		crypter := crypto.NewImmutableKeyDecrypter(pass)

		c.since = time.Now()
		sleepTime := time.Duration(0)
		for {
			since := time.Since(c.since).Seconds()
			if since > 3 {
				throughput := int(float64(c.bytesRead) / since)
				c.since = time.Now()
				c.bytesRead = 0

				if throughput > c.maxThroughput {
					sleepTime += time.Millisecond * 20
				} else if throughput < 4*c.maxThroughput/5 {
					sleepTime -= time.Millisecond * 50
					if sleepTime < 0 {
						sleepTime = 0
					}
				}
				fmt.Println(throughput/1024, sleepTime)
			}

			if sleepTime != 0 {
				time.Sleep(sleepTime)
			}

			if _, err = conn.Write(c.w); err != nil {
				c.connErr(err)
				break
			}
			var ln uint64
			if err = binary.Read(conn, binary.LittleEndian, &ln); err != nil {
				c.connErr(err)
				break
			}

			d := make([]byte, ln)
			if _, err = io.ReadFull(conn, d); err != nil {
				c.connErr(err)
				break
			}

			if len(d) == 3 {
				continue
			}

			out := bytes.NewBuffer(make([]byte, 0, len(d)))
			if err = crypter.Decrypt(bytes.NewBuffer(d), out); err != nil {
				c.connErr(err)
				break
			}

			c.bytesRead += out.Len()

			data <- &Data{Buffer: out, Created: time.Now()}
		}

	}
}
