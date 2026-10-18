package controllers

import "meerkat/clock"

// SetImportClock gives the Monica import sessions the application clock (set once at
// startup, before serving).
func SetImportClock(c clock.Clock) {
	monicaImportSessions.SetClock(c)
}
