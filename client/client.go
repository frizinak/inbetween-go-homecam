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

type Info int

const (
	InfoConnecting Info = iota
	InfoReconnecting
	InfoConnected
	InfoHandshakeFail
	InfoError
)

type Client struct {
	l    *log.Logger
	addr string
	pass chan []byte

	w    []byte
	ping []byte

	proto *protocol.Protocol
	info  chan Info
}

func New(l *log.Logger, addr string, pass chan []byte) (*Client, <-chan Info) {
	info := make(chan Info, 1)
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
		info,
	}, info
}

func (c *Client) connErr(err error) error {
	switch {
	case err == io.EOF:
	case err == protocol.ErrDenied:
		c.info <- InfoHandshakeFail
	default:
		c.info <- InfoError
		c.l.Println(err)
	}

	return nil
}

type Data struct {
	*bytes.Buffer
	created time.Time
}

func (d *Data) Created() time.Time { return d.created }

func (c *Client) Connect(data chan<- *Data) error {
	var conn net.Conn
	var connErr error
	pass := <-c.pass

	for {
		if conn != nil {
			conn.Close()
			if connErr != nil {
				return connErr
			}

			time.Sleep(time.Second)
			c.info <- InfoReconnecting
			time.Sleep(time.Second)
		}

		var err error
		c.info <- InfoConnecting
		conn, err = net.Dial("tcp", c.addr)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		common := make([]byte, len(vars.CommonSecret))
		copy(common, vars.CommonSecret)
		common = append(common, pass...)

		crypter, err := c.proto.HandshakeClient(common, conn)
		if err != nil {
			connErr = c.connErr(err)
			if err == protocol.ErrDenied {
				conn.Close()
				conn = nil
				pass = <-c.pass
			}

			continue
		}

		c.info <- InfoConnected
		for {
			if _, err = conn.Write(c.w); err != nil {
				connErr = c.connErr(err)
				break
			}
			var ln uint64
			if err = binary.Read(conn, binary.LittleEndian, &ln); err != nil {
				connErr = c.connErr(err)
				break
			}

			d := make([]byte, ln)
			if _, err = io.ReadFull(conn, d); err != nil {
				connErr = c.connErr(err)
				break
			}

			if len(d) == 3 {
				continue
			}

			out := bytes.NewBuffer(make([]byte, 0, len(d)))
			if err = crypter.Decrypt(bytes.NewBuffer(d), out); err != nil {
				connErr = c.connErr(err)
				break
			}

			data <- &Data{Buffer: out, created: time.Now()}
		}
	}
}
