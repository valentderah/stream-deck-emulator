// Package ui serves the browser front end: an API for the emulated devices,
// an API for connecting them, and the page that uses both.
package ui

import (
	"embed"
	"encoding/json"
	"errors"
	"image/png"
	"mime"
	"net/http"
	"slices"
	"strconv"

	"stream-deck-plus-emulator/internal/deck"
	"stream-deck-plus-emulator/internal/link"
)

//go:embed static locales
var assets embed.FS

// Codes sent in error bodies. The browser catalog has an entry for each.
const (
	codeUnknownDevice    = "unknown_device"
	codeUnsupportedInput = "unsupported_input"
	codeBadRequest       = "bad_request"
	codeForbidden        = "forbidden"
)

// Codes lists the error codes the API itself sends.
func Codes() []string {
	return []string{codeUnknownDevice, codeUnsupportedInput, codeBadRequest, codeForbidden}
}

const maxInputBytes = 1024

func init() {
	// The Windows registry can map these extensions to the wrong type.
	mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	mime.AddExtensionType(".css", "text/css; charset=utf-8")
	mime.AddExtensionType(".json", "application/json")
}

// Linker is the connection side of the API.
type Linker interface {
	Connect(id string) error
	Disconnect(id string) error
	Statuses() []link.Status
	Subscribe() (<-chan link.Status, func())
}

type deviceInfo struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Rows      int         `json:"rows"`
	Cols      int         `json:"cols"`
	KeySize   int         `json:"keySize"`
	Rotated   bool        `json:"rotated"`
	Dials     int         `json:"dials"`
	Strip     *deck.Size  `json:"strip"`
	Screen    *deck.Size  `json:"screen"`
	TouchKeys int         `json:"touchKeys"`
	Link      link.Status `json:"link"`
}

func describe(m deck.Model, status link.Status) deviceInfo {
	info := deviceInfo{
		ID: m.ID, Name: m.Name, Rows: m.Rows, Cols: m.Cols, KeySize: m.KeySize,
		Rotated: m.Rotated, Dials: m.Dials, TouchKeys: m.TouchKeys, Link: status,
	}
	if !m.Strip.Empty() {
		info.Strip = &m.Strip
	}
	if !m.Screen.Empty() {
		info.Screen = &m.Screen
	}
	return info
}

type input struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Down  bool   `json:"down"`
	Ticks int8   `json:"ticks"`
	Kind  uint8  `json:"kind"`
	X     uint16 `json:"x"`
	Y     uint16 `json:"y"`
	X2    uint16 `json:"x2"`
	Y2    uint16 `json:"y2"`
}

// Handler serves the UI for devices. hosts lists the Host header values the
// server answers to; nil accepts any.
func Handler(devices []*deck.Device, lk Linker, hosts []string) http.Handler {
	byID := map[string]*deck.Device{}
	for _, d := range devices {
		byID[d.Model().ID] = d
	}
	// device resolves the {id} path value and answers 404 itself if there
	// is no such device.
	device := func(w http.ResponseWriter, r *http.Request) *deck.Device {
		d := byID[r.PathValue("id")]
		if d == nil {
			fail(w, http.StatusNotFound, codeUnknownDevice)
		}
		return d
	}
	image := func(w http.ResponseWriter, contentType string, data []byte) {
		if data == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assets, "static/index.html")
	})
	mux.Handle("GET /static/", http.FileServerFS(assets))
	mux.Handle("GET /locales/", http.FileServerFS(assets))

	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, _ *http.Request) {
		statuses := map[string]link.Status{}
		for _, s := range lk.Statuses() {
			statuses[s.ID] = s
		}
		list := make([]deviceInfo, 0, len(devices))
		for _, d := range devices {
			list = append(list, describe(d.Model(), statuses[d.Model().ID]))
		}
		writeJSON(w, http.StatusOK, list)
	})

	mux.HandleFunc("GET /api/devices/{id}/key/{index}", func(w http.ResponseWriter, r *http.Request) {
		d := device(w, r)
		if d == nil {
			return
		}
		i, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || i < 0 || i >= d.Model().Keys() {
			http.NotFound(w, r)
			return
		}
		image(w, "image/jpeg", d.KeyImage(i))
	})

	mux.HandleFunc("GET /api/devices/{id}/strip", func(w http.ResponseWriter, r *http.Request) {
		d := device(w, r)
		if d == nil {
			return
		}
		strip := d.Strip()
		if strip == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		png.Encode(w, strip)
	})

	mux.HandleFunc("GET /api/devices/{id}/screen", func(w http.ResponseWriter, r *http.Request) {
		d := device(w, r)
		if d == nil {
			return
		}
		if d.Model().Screen.Empty() {
			http.NotFound(w, r)
			return
		}
		image(w, "image/jpeg", d.Screen())
	})

	mux.HandleFunc("POST /api/devices/{id}/input", func(w http.ResponseWriter, r *http.Request) {
		d := device(w, r)
		if d == nil {
			return
		}
		var in input
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInputBytes)).Decode(&in); err != nil {
			fail(w, http.StatusBadRequest, codeBadRequest)
			return
		}
		var err error
		switch in.Type {
		case "key":
			err = d.SetKey(in.Index, in.Down)
		case "dialPress":
			err = d.PressDial(in.Index, in.Down)
		case "dialRotate":
			err = d.RotateDial(in.Index, in.Ticks)
		case "touch":
			err = d.Touch(in.Kind, in.X, in.Y, in.X2, in.Y2)
		default:
			fail(w, http.StatusBadRequest, codeBadRequest)
			return
		}
		if errors.Is(err, deck.ErrInput) {
			fail(w, http.StatusBadRequest, codeUnsupportedInput)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	action := func(do func(id string) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			d := device(w, r)
			if d == nil {
				return
			}
			if err := do(d.Model().ID); err != nil {
				fail(w, http.StatusNotFound, codeUnknownDevice)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("POST /api/devices/{id}/connect", action(lk.Connect))
	mux.HandleFunc("POST /api/devices/{id}/disconnect", action(lk.Disconnect))

	mux.HandleFunc("GET /api/events", events(devices, lk))

	return guard(hosts, mux)
}

// guard rejects requests that a foreign web page could have caused: a Host
// the server does not answer to (DNS rebinding) or an Origin other than the
// page's own.
func guard(hosts []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hosts != nil && !slices.Contains(hosts, r.Host) {
			fail(w, http.StatusForbidden, codeForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			fail(w, http.StatusForbidden, codeForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
