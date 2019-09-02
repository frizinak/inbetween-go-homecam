package view

import (
	"image"
	"image/color"
	"image/draw"
	"io"
	"io/ioutil"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/exp/gl/glutil"
	"golang.org/x/mobile/geom"
)

type TextWriter struct {
	ctx    *freetype.Context
	tt     *truetype.Font
	bounds fixed.Rectangle26_6
	// size, dpi float64
}

func NewTextWriter() *TextWriter {
	return &TextWriter{ctx: freetype.NewContext()}
}

func (t *TextWriter) SetColor(c color.Color) { t.ctx.SetSrc(image.NewUniform(c)) }

func (t *TextWriter) SetFont(tt *truetype.Font) *TextWriter {
	t.tt = tt
	t.ctx.SetFont(tt)
	t.SetFontSize(12, 72)

	return t
}

func (t *TextWriter) SetReadFont(r io.Reader) error {
	f, err := ReadFont(r)
	if err != nil {
		return err
	}

	t.SetFont(f)
	return nil
}

func (t *TextWriter) SetFontSize(size float64, dpi float64) {
	t.ctx.SetFontSize(size)
	t.ctx.SetDPI(dpi)
	// t.size = size
	// t.dpi = dpi
	t.bounds = t.tt.Bounds(fixed.Int26_6(0.5 + (size * dpi * 64 / 72)))
}

func (t *TextWriter) Write(img draw.Image, text string, pt image.Point) (image.Point, error) {
	t.ctx.SetDst(img)
	var b image.Rectangle
	if img != nil {
		b = img.Bounds()
	}

	t.ctx.SetClip(b)
	f := fixed.P(pt.X, pt.Y)
	min := -t.bounds.Max.Y
	max := -t.bounds.Min.Y - 63

	f.Y -= min
	p, err := t.ctx.DrawString(text, f)
	return image.Pt(int(p.X)>>6, int(p.Y+max-min)>>6), err
}

func ReadFont(r io.Reader) (*truetype.Font, error) {
	rawFont, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return freetype.ParseFont(rawFont)
}

type GlText struct {
	imgs   *glutil.Images
	frame  *glutil.Image
	writer *TextWriter
	text   string
	draw   bool
}

func NewGlText(imgs *glutil.Images) *GlText {
	return &GlText{imgs: imgs, writer: NewTextWriter()}
}

func (g *GlText) SetFont(tt *truetype.Font) {
	g.draw = true
	g.writer.SetFont(tt)
}

func (g *GlText) SetReadFont(r io.Reader) error {
	g.draw = true
	return g.writer.SetReadFont(r)
}

func (g *GlText) SetFontSize(pt float64, dpi float64) {
	g.draw = true
	g.writer.SetFontSize(pt, dpi)
}

func (g *GlText) SetColor(c color.Color) {
	g.draw = true
	g.writer.SetColor(c)
}

func (g *GlText) Write(text string) {
	if g.text == text {
		return
	}
	g.draw = true
	g.text = text

}

func (g *GlText) Clear() {
	g.text = ""
	if g.frame != nil {
		g.frame.Release()
		g.frame = nil
	}
}

func (g *GlText) Release() {
	g.Clear()
}

func (g *GlText) Draw(sz size.Event, pt image.Point) error {
	if g.text == "" {
		return nil
	}

	if g.draw {
		if g.frame != nil {
			g.frame.Release()
		}
		zp := image.Point{}
		p, err := g.writer.Write(nil, g.text, zp)
		if err != nil {
			return err
		}

		g.frame = g.imgs.NewImage(p.X, p.Y)
		_, err = g.writer.Write(g.frame.RGBA, g.text, zp)
		if err != nil {
			g.frame.Release()
			g.frame = nil
			return err
		}
		g.draw = false
	}

	if g.frame == nil {
		return nil
	}

	b := g.frame.RGBA.Bounds()
	pppt := float64(sz.PixelsPerPt)
	owidth := float64(b.Dx()) / pppt
	oheight := float64(b.Dy()) / pppt

	x, y := float64(pt.X)/pppt, float64(pt.Y)/pppt
	x1, y1 := geom.Pt(x), geom.Pt(y)
	x2, y2 := geom.Pt(x+owidth), geom.Pt(y+oheight)
	g.frame.Upload()
	g.frame.Draw(
		sz,
		geom.Point{x1, y1},
		geom.Point{x2, y1},
		geom.Point{x1, y2},
		b,
	)

	return nil
}
