package mobile

import (
	"math"

	"golang.org/x/mobile/event/touch"
)

func TouchDistance(e1, e2 touch.Event) float64 {
	x := e1.X - e2.X
	y := e1.Y - e2.Y
	return math.Sqrt(float64(x*x + y*y))
}

func TouchAvg(e1, e2 touch.Event) (x, y float64) {
	x = float64(e1.X+e2.X) / 2
	y = float64(e1.Y+e2.Y) / 2
	return
}
