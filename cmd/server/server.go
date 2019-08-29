package main

import (
	"log"
	"os"
	"strconv"

	"github.com/frizinak/inbetween-go-homecam/config"
	"github.com/frizinak/inbetween-go-homecam/server"
	"github.com/frizinak/inbetween-go-homecam/vars"
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

	if len(conf.TouchPassword) != vars.TouchPasswordLen {
		l.Fatal("Invalid touch password (length)")
	}

	pass2 := make([]byte, vars.TouchPasswordLen)
	for i := range pass2 {
		n, err := strconv.Atoi(string(conf.TouchPassword[i]))
		if err != nil {
			l.Fatal(err)
		} else if n < 0 || n > 10 {
			l.Fatal("Invalid touch password (char)")
		}
		pass2[i] = byte(n)
	}

	s := server.New(
		l,
		conf.Address,
		[]byte(conf.Password),
		pass2,
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
