package main

import (
	"log"
	"os"

	"github.com/frizinak/inbetween-go-homecam/config"
	"github.com/frizinak/inbetween-go-homecam/server"
)

func main() {
	l := log.New(os.Stderr, "", log.Ldate|log.Ltime)
	file, err := config.DefaultConfigFile()
	if err != nil {
		l.Fatal(err)
	}

	conf, err := config.LoadConfig(file)
	if err != nil {
		if !os.IsNotExist(err) {
			l.Fatal(err)
		}

		if err := config.EnsureConfig(file); err != nil {
			l.Fatal(err)
		}

		l.Printf("Created example config file in %s", file)
		return
	}

	s := server.New(
		l,
		conf.Address,
		[]byte(conf.Password),
		conf.Device,
		conf.Quality,
		conf.MaxPeers,
	)
	output, errs := s.Start()
	go func() {
		if err := s.Listen(output); err != nil {
			l.Fatal(err)
		}
	}()

	if err := <-errs; err != nil {
		l.Fatal(err)
	}
}
