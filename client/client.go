package client

import (
	"bytes"
	"encoding/binary"
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
		for {
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

			data <- &Data{Buffer: out, Created: time.Now()}
		}

	}
}
