// Package adapter isolates all Wails framework imports. No other package may
// import Wails; engines and facades talk to the framework through Bridge.
package adapter

import "github.com/wailsapp/wails/v3/pkg/application"

// EventSink publishes backend events to the UI. Bridge implements it against
// the Wails v3 event API; tests inject fakes. Emit may be called concurrently
// from multiple goroutines (e.g. the fswatch forwarder); implementations must
// be safe for concurrent use.
type EventSink interface {
	Emit(name string, payload any)
}

// Bridge implements EventSink by emitting to all windows via the Wails v3
// global event manager (app.Event.Emit, beta.20 API).
type Bridge struct{}

func NewBridge() *Bridge { return &Bridge{} }

func (b *Bridge) Emit(name string, payload any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, payload)
	}
}
