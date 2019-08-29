package main

import (
	"bytes"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/frizinak/inbetween-go-homecam/client"
	"github.com/frizinak/inbetween-go-homecam/server"
	"github.com/frizinak/inbetween-go-homecam/view"
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

	s := server.New(l, "", []byte{}, "/dev/video0", qual, 100)
	output, errs := s.Start()
	tick := make(chan *client.Data)
	var img *bytes.Buffer
	go func() {
		for {
			select {
			case err := <-errs:
				panic(err)
			case b := <-output:
				img = b
				select {
				case tick <- &client.Data{Buffer: b, Created: time.Now()}:
				default:
				}
			}
		}

	}()

	v := view.New(l)
	v.Start(tick)
}
