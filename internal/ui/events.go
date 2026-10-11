package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"stream-deck-plus-emulator/internal/deck"
	"stream-deck-plus-emulator/internal/link"
)

const kindLink = "link"

// message is one server-sent event: a display change of a device or, with
// Kind "link", a change of its connection state.
type message struct {
	Device string       `json:"device"`
	Kind   string       `json:"kind"`
	Index  int          `json:"index"`
	Value  int          `json:"value"`
	Link   *link.Status `json:"link,omitempty"`
}

func linkMessage(s link.Status) message {
	return message{Device: s.ID, Kind: kindLink, Link: &s}
}

// events streams display and connection changes of all devices. A new
// stream starts with the current brightness, touch key colours and
// connection states.
func events(devices []*deck.Device, lk Linker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			fail(w, http.StatusInternalServerError, codeBadRequest)
			return
		}
		ctx := r.Context()
		merged := make(chan message, 256)
		for _, d := range devices {
			ch, cancel := d.Subscribe()
			defer cancel()
			go forward(ctx, d.Model().ID, ch, merged)
		}
		statuses, cancel := lk.Subscribe()
		defer cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		write := func(m message) {
			data, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		for _, d := range devices {
			id := d.Model().ID
			write(message{Device: id, Kind: string(deck.EventBrightness), Value: d.Brightness()})
			for i := range d.Model().TouchKeys {
				write(message{Device: id, Kind: string(deck.EventTouchKey), Index: i, Value: d.TouchColor(i)})
			}
		}
		for _, s := range lk.Statuses() {
			write(linkMessage(s))
		}
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-merged:
				write(m)
			case s := <-statuses:
				write(linkMessage(s))
			}
		}
	}
}

func forward(ctx context.Context, id string, in <-chan deck.Event, out chan<- message) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-in:
			select {
			case out <- message{Device: id, Kind: string(e.Kind), Index: e.Index, Value: e.Value}:
			case <-ctx.Done():
				return
			}
		}
	}
}
