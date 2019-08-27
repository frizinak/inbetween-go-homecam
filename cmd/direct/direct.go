package main

import (
	"bytes"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/frizinak/homecam/mobile"
	"github.com/frizinak/homecam/server"
)

func main() {
	go http.ListenAndServe(":8080", nil)
	l := log.New(os.Stderr, "", 0)

	qual := server.QualityConfig{
		MinFPS: 8,
		MaxFPS: 16,

		MinJPEGQuality: 30,
		MaxJPEGQuality: 100,

		MinResolution: 0,
		MaxResolution: 64000 * 48000,

		DesiredTotalThroughput:  1e16,
		DesiredClientThroughput: 1e16,
	}

	s := server.New(l, "", []byte{}, "/dev/video0", qual)
	output, errs := s.Start()
	tick := make(chan *bytes.Buffer)
	var img *bytes.Buffer
	go func() {
		for {
			select {
			case err := <-errs:
				panic(err)
			case b := <-output:
				img = b
				select {
				case tick <- b:
				default:
				}
			}
		}

	}()

	v := mobile.New(l)
	v.Start(tick)
}
