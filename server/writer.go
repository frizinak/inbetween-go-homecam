package server

import (
	"bytes"
	"encoding/binary"
	"io"
)

type countWriter struct {
	buf *bytes.Buffer
	w   io.Writer
}

func newCountWriter(w io.Writer) *countWriter {
	buf := bytes.NewBuffer(nil)
	return &countWriter{w: w, buf: buf}
}

func (c *countWriter) Reset() {
	c.buf.Reset()
}

func (c *countWriter) Write(b []byte) (int, error) {
	return c.buf.Write(b)
}

func (c *countWriter) Flush(b []byte) (uint64, error) {
	var err error
	_, err = c.Write(b)
	if err != nil {
		return 0, err
	}

	l := uint64(c.buf.Len())
	err = binary.Write(c.w, binary.LittleEndian, l)
	if err != nil {
		return l, err
	}

	_, err = c.buf.WriteTo(c.w)
	return l, err
}
