package view

import (
	"io"
	"time"
)

type Reader interface {
	io.Reader
	Created() time.Time
}
