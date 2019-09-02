package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

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
	passChan := make(chan []byte)
	pass2Chan := make(chan []byte)

	statusChan := make(chan string, 1)
	v := view.New(l, pass2Chan, statusChan, touchPassLen)
	tickIn := make(chan view.Reader)
	tickOut := make(chan *client.Data)

	go func() {
		for {
			tickIn <- <-tickOut
		}
	}()

	c, info := client.New(l, conf.Address, passChan)

	if *genPass {
		go func() {
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

		}()
		v.Start(tickIn)
	}

	go func() {
		var str string
		var last string
		for msg := range info {
			switch msg {
			case client.InfoConnecting:
				str = "Connecting..."
			case client.InfoConnected:
				str = "Connected"
			case client.InfoReconnecting:
				str = "Reconnecting..."
			case client.InfoError:
				str = "Something went wrong!"
			case client.InfoHandshakeFail:
				str = "Wrong password"
				go func() {
					time.Sleep(time.Second * 1)
					v.ClearPass()
				}()
			}

			if str != "" && str != last {
				last = str
				statusChan <- str
			}
		}
	}()

	go func() {
		for p := range pass2Chan {
			passChan <- append([]byte(conf.Password), p...)
		}
	}()

	go func() {
		l.Fatal(c.Connect(tickOut))
	}()

	v.Start(tickIn)
}
