package client

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"

	"github.com/frizinak/homecam/crypto"
	"github.com/frizinak/homecam/server"
	"golang.org/x/crypto/scrypt"
)

type Client struct {
	l    *log.Logger
	addr string
	pass []byte

	w    []byte
	ping []byte
}

func New(l *log.Logger, addr string, pass []byte) *Client {
	return &Client{l, addr, pass, []byte{0}, []byte{10}}
}

func (c *Client) connErr(err error) {
	if err != io.EOF {
		c.l.Println(err)
	}
}

func (c *Client) Connect(data chan<- *bytes.Buffer) error {
	for {
		conn, err := net.Dial("tcp", c.addr)
		if err != nil {
			return err
		}

		handshake := make([]byte, server.HandshakeLen)
		if _, err := io.ReadFull(conn, handshake); err != nil {
			return err
		}

		handshakeHash, err := scrypt.Key(
			c.pass,
			handshake,
			1<<server.HandshakeCost,
			8,
			1,
			server.HandshakeHashLen,
		)
		if err != nil {
			return err
		}

		if _, err := conn.Write(handshakeHash); err != nil {
			return err
		}

		for {
			if _, err := conn.Write(c.w); err != nil {
				c.connErr(err)
				break
			}
			var ln uint64
			if err := binary.Read(conn, binary.LittleEndian, &ln); err != nil {
				c.connErr(err)
				break
			}

			d := make([]byte, ln)
			if _, err := io.ReadFull(conn, d); err != nil {
				c.connErr(err)
				break
			}

			if len(d) == 3 {
				continue
			}

			out := bytes.NewBuffer(make([]byte, 0, len(d)))
			if err := crypto.Decrypt(bytes.NewBuffer(d), out, c.pass); err != nil {
				c.connErr(err)
				break
			}

			data <- out
		}

		conn.Close()
		time.Sleep(time.Second)
	}
}
