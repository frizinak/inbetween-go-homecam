package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/frizinak/inbetween-go-homecam/client"
	"github.com/frizinak/inbetween-go-homecam/config"
	"github.com/frizinak/inbetween-go-homecam/view"
)

func main() {
	genPass := flag.Bool("p", false, "Generate touch password")
	flag.Parse()

	conf := config.Config{
		Address:  address,
		Password: password,
	}

	l := log.New(os.Stderr, "", 0)
	pass2Chan := make(chan []byte)
	pass := []byte(conf.Password)
	v := view.New(l, pass2Chan, touchPassLen)
	tickIn := make(chan view.Reader)
	tickOut := make(chan *client.Data)

	go func() {
		for {
			tickIn <- <-tickOut
		}
	}()

	go func() {
		if *genPass {
			pass := <-pass2Chan
			ints := make([]int, len(pass))
			for i := range pass {
				ints[i] = int(pass[i])
			}
			enc := json.NewEncoder(os.Stdout)
			if err := enc.Encode(ints); err != nil {
				l.Fatal(err)
			}
			os.Exit(0)
		}

		pass = append(pass, <-pass2Chan...)
		c := client.New(l, conf.Address, pass)
		l.Fatal(c.Connect(tickOut))
	}()

	v.Start(tickIn)
}
