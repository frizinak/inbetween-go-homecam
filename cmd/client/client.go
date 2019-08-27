package main

import (
	"bytes"
	"log"
	"os"

	"github.com/frizinak/homecam/client"
	"github.com/frizinak/homecam/config"
	"github.com/frizinak/homecam/mobile"
)

var (
	address  string
	password string
)

func main() {
	conf := config.Config{
		Address:  address,
		Password: password,
	}

	if conf.Password == "" {
		file, err := config.DefaultConfigFile()
		if err == nil {
			c, err := config.LoadConfig(file)
			if err == nil {
				conf = c
			}
		}
	}

	l := log.New(os.Stderr, "", 0)
	v := mobile.New(l)
	c := client.New(l, conf.Address, []byte(conf.Password))
	tick := make(chan *bytes.Buffer)
	go func() {
		panic(c.Connect(tick))
	}()
	v.Start(tick)
}
