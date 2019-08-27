package server

import (
	"bytes"
	"encoding/binary"
	"io"
)

type countWriter struct {
	n   int
	buf bytes.Buffer
	w   io.Writer
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.buf.Write(b)
	c.n += n
	return n, err
}

func (c *countWriter) Flush() error {
	err := binary.Write(c.w, binary.LittleEndian, uint64(c.n))
	if err != nil {
		return err
	}

	_, err = c.buf.WriteTo(c.w)
	return err
}
