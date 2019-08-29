// +build mobile

package view

import (
	"golang.org/x/mobile/app"
)

type mobileWindow struct {
	app.App
}

func (m *mobileWindow) Publish()                     { m.App.Publish() }
func (m *mobileWindow) RequiresViewportUpdate() bool { return false }

func (v *View) Start(tick chan Reader) {
	app.Main(func(mobile app.App) {
		v.loop(&mobileWindow{mobile}, mobile.Events(), mobile.Filter, tick)
	})
}
