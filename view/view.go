package view

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"time"

	"github.com/frizinak/inbetween-go-homecam/bound"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/event/touch"
	"golang.org/x/mobile/exp/gl/glutil"
	"golang.org/x/mobile/geom"
	"golang.org/x/mobile/gl"
)

const touchTypeNone = 100

type View struct {
	l      *log.Logger
	images *glutil.Images

	arrows map[byte]*glutil.Image

	frame  *glutil.Image
	sz     size.Event
	reinit bool

	auth struct {
		passChan    chan<- []byte
		passLen     int
		phase       int
		last        touch.Event
		tap         time.Time
		n           byte
		fingersDown bool
	}

	framePos struct {
		offsetX      float64
		offsetY      float64
		previousZoom float64
		zoom         float64
	}

	bounds       image.Rectangle
	frameCreated time.Time

	stopDecoder chan struct{}

	touch struct {
		tap            time.Time
		moving         bool
		pinching       bool
		pinchingIntent bool
		pinchDist      float64
		lastBegin      touch.Event
		lastBegin2     touch.Event
	}
}

func New(l *log.Logger, passChan chan<- []byte, passLen int) *View {
	v := &View{l: l, stopDecoder: make(chan struct{})}
	v.auth.passChan = passChan
	v.auth.passLen = passLen
	v.auth.last.Type = touchTypeNone
	v.touch.lastBegin.Type = touchTypeNone
	v.touch.lastBegin2.Type = touchTypeNone
	return v
}

func (v *View) initStage(glctx gl.Context, tick chan Reader) {
	v.images = glutil.NewImages(glctx)

	// todo error handling
	arrows := []byte{1, 2, 4, 8, 1 | 4, 1 | 8, 2 | 4, 2 | 8}
	v.arrows = make(map[byte]*glutil.Image, len(arrows))
	for i := range arrows {
		r := bytes.NewBuffer(bound.MustAsset(fmt.Sprintf("arrow-%d.png", arrows[i])))
		img, err := png.Decode(r)
		if err != nil {
			panic(err)
		}

		b := img.Bounds()
		ix := arrows[i]
		v.arrows[ix] = v.images.NewImage(b.Dx(), b.Dy())
		draw.Draw(v.arrows[ix].RGBA, b, img, image.Point{}, draw.Src)
		v.arrows[ix].Upload()
	}

	var origBounds image.Rectangle
	go func() {
		for {
			select {
			case <-v.stopDecoder:
				return
			case data := <-tick:
				i, err := jpeg.Decode(data)
				if err != nil {
					v.l.Println(err)
					continue
				}

				b := i.Bounds()
				v.bounds = b
				v.frameCreated = data.Created()
				owidth := float64(b.Dx())
				oheight := float64(b.Dy())
				if origBounds != b || v.frame == nil {
					origBounds = b
					if v.frame != nil {
						v.frame.Release()
					}
					v.frame = v.images.NewImage(int(owidth), int(oheight))
				}

				draw.Draw(
					v.frame.RGBA,
					b,
					i,
					image.Point{},
					draw.Src,
				)
				v.frame.Upload()
			}
		}

	}()
}

func (v *View) destroyStage(glctx gl.Context) {
	v.stopDecoder <- struct{}{}
	for i := range v.arrows {
		v.arrows[i].Release()
	}
	if v.frame != nil {
		v.frame.Release()
		v.frame = nil
	}
	v.images.Release()
}

func (v *View) paint(glctx gl.Context, sz size.Event) {
	var r, g, b float32
	if time.Since(v.frameCreated) > time.Second {
		r, g, b = 0.6, 0.2, 0.2
	}

	if v.auth.phase == 0 {
		r, g, b = 0.2, 0.2, 0.2
		if v.auth.fingersDown {
			g = 0.6
		}
	}

	glctx.ClearColor(r, g, b, 1)
	glctx.Clear(gl.COLOR_BUFFER_BIT)

	pppt := float64(sz.PixelsPerPt)
	szWidth := float64(sz.WidthPt)
	szHeight := float64(sz.HeightPt)

	if v.auth.phase == 0 {
		if v.auth.fingersDown && v.auth.n > 0 {
			a := v.arrows[v.auth.n]
			b := a.RGBA.Bounds()
			owidth := float64(b.Dx())
			oheight := float64(b.Dy())
			scale := szWidth / owidth
			scale2 := szHeight / oheight
			if scale2 < scale {
				scale = scale2
			}
			scale = 2 * scale / 3

			width := owidth * scale
			height := oheight * scale

			offsetX := szWidth/2 - width/2
			offsetY := szHeight/2 - height/2
			x1 := geom.Pt(float64(b.Min.X)*scale + offsetX)
			x2 := geom.Pt(float64(b.Max.X)*scale + offsetX)
			y1 := geom.Pt(float64(b.Min.Y)*scale + offsetY)
			y2 := geom.Pt(float64(b.Max.Y)*scale + offsetY)

			a.Draw(
				sz,
				geom.Point{x1, y1},
				geom.Point{x2, y1},
				geom.Point{x1, y2},
				b,
			)
		}
		return
	}

	if v.frame == nil {
		return
	}

	owidth := float64(v.bounds.Dx())
	oheight := float64(v.bounds.Dy())

	scale := szWidth / owidth
	scale2 := szHeight / oheight
	if scale2 < scale {
		scale = scale2
	}
	width := owidth * scale
	height := oheight * scale

	if sz != v.sz || v.reinit {
		v.sz = sz
		v.reinit = false
		v.framePos.offsetX = 0
		v.framePos.offsetY = 0
		v.framePos.previousZoom = 1
		v.framePos.zoom = 1
	}

	zoomDiff := v.framePos.zoom - v.framePos.previousZoom
	if zoomDiff != 0 {
		v.framePos.offsetX -= pppt * (width*v.framePos.zoom - width*v.framePos.previousZoom) / 2
		v.framePos.offsetY -= pppt * (height*v.framePos.zoom - height*v.framePos.previousZoom) / 2
	}
	v.framePos.previousZoom = v.framePos.zoom

	offsetX := szWidth/2 - width/2 + v.framePos.offsetX/pppt
	offsetY := szHeight/2 - height/2 + v.framePos.offsetY/pppt

	x1 := geom.Pt(float64(v.bounds.Min.X)*scale*v.framePos.zoom + offsetX)
	x2 := geom.Pt(float64(v.bounds.Max.X)*scale*v.framePos.zoom + offsetX)
	y1 := geom.Pt(float64(v.bounds.Min.Y)*scale*v.framePos.zoom + offsetY)
	y2 := geom.Pt(float64(v.bounds.Max.Y)*scale*v.framePos.zoom + offsetY)

	v.frame.Draw(
		sz,
		geom.Point{x1, y1},
		geom.Point{x2, y1},
		geom.Point{x1, y2},
		v.frame.RGBA.Bounds(),
	)
}

func (v *View) handlePassword(e touch.Event, sz size.Event, pass []byte) []byte {
	s := int(e.Sequence)
	if s > 0 {
		return pass
	}

	switch e.Type {
	case touch.TypeBegin:
		if time.Since(v.auth.tap) < time.Millisecond*200 {
			v.auth.last.Type = touchTypeNone
			return pass[0:0]
		}
		v.auth.last = e
		v.auth.tap = time.Now()
		v.auth.fingersDown = false
	case touch.TypeEnd:
		v.auth.n = 0
		v.auth.last.Type = touchTypeNone
		v.auth.fingersDown = false
	case touch.TypeMove:
		if v.auth.last.Type == touchTypeNone {
			return pass
		}

		thresh := float64(sz.WidthPx) / 5
		if TouchDistance(e, v.auth.last) < thresh {
			return pass
		}

		var max float32 = 2.5
		var n, u, d, r, l byte = 0, 8, 4, 2, 1
		m := (e.X - v.auth.last.X) / (e.Y - v.auth.last.Y)
		if m < 0 {
			m = -m
		}

		if m < max {
			n |= d
			if e.Y < v.auth.last.Y {
				n = (n | u) &^ d
			}
		}

		if m > 1/max {
			n |= r
			if e.X < v.auth.last.X {
				n = (n | l) &^ r
			}
		}
		if n == 0 || n == v.auth.n {
			return pass
		}

		v.auth.n = n
		pass = append(pass, n)
		v.auth.fingersDown = true
	}

	return pass
}

func (v *View) handleTouch(e touch.Event, sz size.Event) {
	if e.Sequence > 1 {
		return
	}

	switch e.Type {

	case touch.TypeBegin:
		switch e.Sequence {
		case 0:
			v.touch.lastBegin = e
			if time.Since(v.touch.tap) < time.Millisecond*200 {
				v.reinit = true
			}
			v.touch.tap = time.Now()
		case 1:
			v.touch.lastBegin2 = e
			v.touch.moving = false
		}

	case touch.TypeEnd:
		v.touch.pinching = false
		v.touch.pinchingIntent = false
		switch e.Sequence {
		case 0:
			v.touch.lastBegin.Type = touchTypeNone
			v.touch.moving = false
		case 1:
			v.touch.lastBegin2.Type = touchTypeNone
		}

	case touch.TypeMove:
		if v.touch.lastBegin.Type == touchTypeNone {
			return
		}

		if v.touch.lastBegin2.Type == touchTypeNone {
			if v.touch.moving || TouchDistance(e, v.touch.lastBegin) > 50 {
				v.touch.moving = true
				v.framePos.offsetX += float64(e.X - v.touch.lastBegin.X)
				v.framePos.offsetY += float64(e.Y - v.touch.lastBegin.Y)
				v.touch.lastBegin.X = e.X
				v.touch.lastBegin.Y = e.Y
			}
			return
		}

		switch e.Sequence {
		case 0:
			v.touch.lastBegin.X = e.X
			v.touch.lastBegin.Y = e.Y
		case 1:
			v.touch.lastBegin2.X = e.X
			v.touch.lastBegin2.Y = e.Y
		}

		dist := TouchDistance(v.touch.lastBegin, v.touch.lastBegin2)
		if !v.touch.pinchingIntent {
			v.touch.pinchingIntent = true
			v.touch.pinchDist = dist
		}

		diff := dist - v.touch.pinchDist
		if diff > 50 || diff < -50 || v.touch.pinching {
			v.touch.pinching = true
			v.touch.pinchDist = dist
			v.framePos.zoom += diff / float64(sz.WidthPx) * v.framePos.zoom
		}
	}
}

type filter func(interface{}) interface{}
type window interface {
	Send(event interface{})
	Publish()
	RequiresViewportUpdate() bool
}

func (v *View) loop(w window, events <-chan interface{}, f filter, tick chan Reader) {
	var glctx gl.Context
	var sz size.Event
	vpUpdate := w.RequiresViewportUpdate()
	pass := make([]byte, 0, v.auth.passLen)
	for e := range events {
		switch e := f(e).(type) {
		case lifecycle.Event:
			switch e.Crosses(lifecycle.StageVisible) {
			case lifecycle.CrossOn:
				glctx, _ = e.DrawContext.(gl.Context)
				v.initStage(glctx, tick)
				w.Send(paint.Event{})
			case lifecycle.CrossOff:
				v.destroyStage(glctx)
				glctx = nil
			}
		case touch.Event:
			if v.auth.phase == 0 {
				pass = v.handlePassword(e, sz, pass)
				if len(pass) >= v.auth.passLen {
					v.auth.passChan <- pass[:v.auth.passLen]
					v.auth.phase = 1
				}

				continue
			}
			v.handleTouch(e, sz)
		case size.Event:
			sz = e
			if vpUpdate && glctx != nil {
				glctx.Viewport(0, 0, sz.WidthPx, sz.HeightPx)
			}
		case paint.Event:
			if glctx == nil || e.External {
				continue
			}
			v.paint(glctx, sz)
			w.Publish()
			w.Send(paint.Event{})
		}
	}
}
