package client

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"

	"github.com/frizinak/inbetween-go-homecam/protocol"
	"github.com/frizinak/inbetween-go-homecam/vars"
)

type Client struct {
	l    *log.Logger
	addr string
	pass []byte

	w    []byte
	ping []byte

	proto *protocol.Protocol
}

func New(l *log.Logger, addr string, pass []byte) *Client {
	return &Client{
		l,
		addr,
		pass,
		[]byte{0},
		[]byte{10},
		protocol.New(
			vars.HandshakeCost,
			vars.EncryptCost,
			vars.HandshakeLen,
			vars.HandshakeHashLen,
		),
	}
}

func (c *Client) connErr(err error) {
	if err != io.EOF {
		c.l.Println(err)
	}
}

type Data struct {
	*bytes.Buffer
	created time.Time
}

func (d *Data) Created() time.Time { return d.created }

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
			time.Sleep(time.Second)
			continue
		}

		common := make([]byte, len(vars.CommonSecret))
		copy(common, vars.CommonSecret)
		common = append(common, c.pass...)

		crypter, err := c.proto.HandshakeClient(common, conn)
		if err != nil {
			connErr = err
			continue
		}

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

			data <- &Data{Buffer: out, created: time.Now()}
		}
	}
}
