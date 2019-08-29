package view

import (
	"image"
	"image/draw"
	"image/jpeg"
	"log"
	"time"

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

	frame    *glutil.Image
	sz       size.Event
	reinit   bool
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

func New(l *log.Logger) *View {
	v := &View{l: l, stopDecoder: make(chan struct{})}
	v.touch.lastBegin.Type = touchTypeNone
	v.touch.lastBegin2.Type = touchTypeNone
	return v
}

func (v *View) initStage(glctx gl.Context, tick chan Reader) {
	v.images = glutil.NewImages(glctx)

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
	v.frame.Release()
	v.frame = nil
	v.images.Release()
}

func (v *View) paint(glctx gl.Context, sz size.Event) {
	var r, g, b float32
	if time.Since(v.frameCreated) > time.Second {
		r, g, b = 0.6, 0.2, 0.2
	}

	glctx.ClearColor(r, g, b, 1)
	glctx.Clear(gl.COLOR_BUFFER_BIT)

	if v.frame == nil {
		return
	}

	owidth := float64(v.bounds.Dx())
	oheight := float64(v.bounds.Dy())

	pppt := float64(sz.PixelsPerPt)
	szWidth := float64(sz.WidthPt)
	szHeight := float64(sz.HeightPt)

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

	x1 := float64(v.bounds.Min.X)*scale*v.framePos.zoom + offsetX
	x2 := float64(v.bounds.Max.X)*scale*v.framePos.zoom + offsetX
	y1 := float64(v.bounds.Min.Y)*scale*v.framePos.zoom + offsetY
	y2 := float64(v.bounds.Max.Y)*scale*v.framePos.zoom + offsetY

	v.frame.Draw(
		sz,
		geom.Point{geom.Pt(x1), geom.Pt(y1)},
		geom.Point{geom.Pt(x2), geom.Pt(y1)},
		geom.Point{geom.Pt(x1), geom.Pt(y2)},
		v.frame.RGBA.Bounds(),
	)
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
}

func (v *View) loop(w window, events <-chan interface{}, f filter, tick chan Reader) {
	var glctx gl.Context
	var sz size.Event
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
			v.handleTouch(e, sz)
		case size.Event:
			sz = e
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
