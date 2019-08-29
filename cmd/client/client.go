package main

import (
	"log"
	"os"

	"github.com/frizinak/inbetween-go-homecam/client"
	"github.com/frizinak/inbetween-go-homecam/config"
	"github.com/frizinak/inbetween-go-homecam/view"
)

func main() {
	conf := config.Config{
		Address:  address,
		Password: password,
	}

	l := log.New(os.Stderr, "", 0)
	v := view.New(l)
	c := client.New(l, conf.Address, []byte(conf.Password))
	tickIn := make(chan view.Reader)
	tickOut := make(chan *client.Data)

	go func() {
		for {
			tickIn <- <-tickOut
		}
	}()

	go func() {
		l.Fatal(c.Connect(tickOut))
	}()
	v.Start(tickIn)
}
