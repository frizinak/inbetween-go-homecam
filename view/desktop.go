// +build !mobile

package view

import (
	"log"

	"golang.org/x/exp/shiny/driver/gldriver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/touch"
)

type desktopWindow struct {
	screen.Window
}

func (d *desktopWindow) Publish() {
	d.Window.Publish()
}

func (v *View) Start(tick chan Reader) {
	gldriver.Main(func(s screen.Screen) {
		w, err := s.NewWindow(nil)
		if err != nil {
			log.Fatal(err)
		}
		defer w.Release()

		events := make(chan interface{})
		go func() {
			for {
				events <- w.NextEvent()
			}
		}()

		v.loop(&desktopWindow{w}, events, convert, tick)
	})
}

// copy pasta from golang.org/x/mobile/app/shiny.go
func convert(e interface{}) interface{} {
	switch e := e.(type) {
	case mouse.Event:
		te := touch.Event{
			X: e.X,
			Y: e.Y,
		}
		switch e.Direction {
		case mouse.DirNone:
			te.Type = touch.TypeMove
		case mouse.DirPress:
			te.Type = touch.TypeBegin
		case mouse.DirRelease:
			te.Type = touch.TypeEnd
		}

		return te
	}
	return e
}
