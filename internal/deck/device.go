// Package deck emulates the HID side of Stream Deck devices: keys, dials,
// touch strip, info screen and touch keys, as described by a Model.
package deck

import (
	"encoding/hex"
	"errors"
	"image"
	"log"
	"sync"

	"stream-deck-plus-emulator/internal/usbip"
)

const (
	inputQueueSize    = 256
	eventQueueSize    = 64
	logPrefixBytes    = 32
	defaultBrightness = 100
	maxBrightness     = 100
	// maxImageBytes bounds a partially received image.
	maxImageBytes = 1 << 20
)

// ErrInput reports an input the model does not have.
var ErrInput = errors.New("deck: input not available on this model")

type EventKind string

const (
	EventKey        EventKind = "key"
	EventStrip      EventKind = "strip"
	EventScreen     EventKind = "screen"
	EventTouchKey   EventKind = "touchkey"
	EventBrightness EventKind = "brightness"
	EventReset      EventKind = "reset"
)

// Event tells subscribers which part of the device's display state changed.
type Event struct {
	Kind  EventKind `json:"kind"`
	Index int       `json:"index"`
	Value int       `json:"value"`
}

type Device struct {
	model Model
	input chan []byte

	mu          sync.Mutex
	attached    bool
	keyImages   [][]byte
	keyParts    [][]byte
	strip       *image.RGBA
	stripPart   []byte
	screen      []byte
	screenPart  []byte
	touchColors []int
	brightness  int
	keyDown     []bool
	dialDown    []bool
	subs        map[chan Event]struct{}
}

func New(m Model) *Device {
	d := &Device{
		model:      m,
		input:      make(chan []byte, inputQueueSize),
		brightness: defaultBrightness,
		subs:       map[chan Event]struct{}{},
	}
	d.clearDisplay()
	d.releaseInputs()
	return d
}

func (d *Device) Model() Model { return d.model }

func (d *Device) Info() usbip.Info {
	return usbip.Info{
		Speed:              usbip.SpeedHigh,
		VendorID:           vendorID,
		ProductID:          d.model.ProductID,
		BCDDevice:          bcdDevice,
		ConfigurationValue: configValue,
		NumConfig:          numConfigurations,
		Interfaces:         []usbip.Interface{{Class: classHID}},
	}
}

func (d *Device) In() <-chan []byte { return d.input }

// SetAttached tells the device whether a host session exists. Input is
// dropped while there is none.
func (d *Device) SetAttached(attached bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.attached = attached
	if !attached {
		d.drain()
	}
}

// Reset puts the device back to its power-on state, as if it had been
// unplugged. Subscribers get a reset event and the default brightness.
func (d *Device) Reset() {
	d.mu.Lock()
	d.clearDisplay()
	d.releaseInputs()
	d.brightness = defaultBrightness
	d.drain()
	d.mu.Unlock()
	d.publish(Event{Kind: EventReset})
	d.publish(Event{Kind: EventBrightness, Value: defaultBrightness})
}

// clearDisplay blanks everything the host can draw. d.mu must be held.
func (d *Device) clearDisplay() {
	m := d.model
	d.keyImages = make([][]byte, m.Keys())
	d.keyParts = make([][]byte, m.Keys())
	d.strip, d.stripPart = nil, nil
	if !m.Strip.Empty() {
		d.strip = image.NewRGBA(image.Rect(0, 0, m.Strip.W, m.Strip.H))
	}
	d.screen, d.screenPart = nil, nil
	d.touchColors = make([]int, m.TouchKeys)
}

// releaseInputs forgets held keys and dials. d.mu must be held.
func (d *Device) releaseInputs() {
	d.keyDown = make([]bool, d.model.Keys()+d.model.TouchKeys)
	d.dialDown = make([]bool, d.model.Dials)
}

// drain empties the input queue. d.mu must be held.
func (d *Device) drain() {
	for {
		select {
		case <-d.input:
		default:
			return
		}
	}
}

func (d *Device) Control(s usbip.Setup, out []byte) ([]byte, error) {
	// wValue carries a type in the high byte and an index or id in the low
	// byte for both descriptor and report requests.
	kind, index := uint8(s.Value>>8), uint8(s.Value)
	is := func(requestType, request uint8) bool {
		return s.RequestType == requestType && s.Request == request
	}
	switch {
	case is(reqTypeDeviceIn, reqGetDescriptor):
		switch kind {
		case descDevice:
			return d.model.deviceDescriptor(), nil
		case descConfiguration:
			return configDescriptor(), nil
		case descString:
			if desc, ok := d.model.stringDescriptor(index); ok {
				return desc, nil
			}
		}
	case is(reqTypeInterfaceIn, reqGetDescriptor):
		switch kind {
		case descHID:
			return hidDescriptor(), nil
		case descReport:
			return reportDescriptor, nil
		}
	case is(reqTypeDeviceIn, reqGetStatus):
		return make([]byte, lenStatus), nil
	case is(reqTypeDeviceIn, reqGetConfiguration):
		return []byte{configValue}, nil
	case is(reqTypeDeviceOut, reqSetConfiguration),
		is(reqTypeInterfaceOut, reqSetInterface),
		is(reqTypeEndpointOut, reqClearFeature),
		is(reqTypeHIDOut, hidSetIdle),
		is(reqTypeHIDOut, hidSetProtocol):
		return nil, nil
	case is(reqTypeHIDIn, hidGetReport):
		if report, ok := d.featureReport(index); ok && kind == reportTypeFeature {
			log.Printf("deck: %s: get feature %#02x -> %s", d.model.ID, index, hex.EncodeToString(report))
			return report, nil
		}
	case is(reqTypeHIDOut, hidSetReport):
		if kind == reportTypeOutput {
			d.Out(0, out)
		} else {
			d.setFeature(out)
		}
		return nil, nil
	}
	log.Printf("deck: %s: stalled control request %+v", d.model.ID, s)
	return nil, usbip.ErrStall
}

// KeyImage returns the JPEG last sent for a key, or nil.
func (d *Device) KeyImage(key int) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if key < 0 || key >= len(d.keyImages) {
		return nil
	}
	return d.keyImages[key]
}

// Strip returns a copy of the touch strip contents, or nil if the model has
// no strip.
func (d *Device) Strip() *image.RGBA {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.strip == nil {
		return nil
	}
	c := image.NewRGBA(d.strip.Bounds())
	copy(c.Pix, d.strip.Pix)
	return c
}

// Screen returns the JPEG last sent to the info screen, or nil.
func (d *Device) Screen() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.screen
}

// TouchColor returns the colour of a touch key as 0xRRGGBB.
func (d *Device) TouchColor(i int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if i < 0 || i >= len(d.touchColors) {
		return 0
	}
	return d.touchColors[i]
}

func (d *Device) Brightness() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.brightness
}

func (d *Device) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, eventQueueSize)
	d.mu.Lock()
	d.subs[ch] = struct{}{}
	d.mu.Unlock()
	return ch, func() {
		d.mu.Lock()
		delete(d.subs, ch)
		d.mu.Unlock()
	}
}

func (d *Device) publish(e Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for ch := range d.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func hexPrefix(data []byte) string {
	if len(data) > logPrefixBytes {
		return hex.EncodeToString(data[:logPrefixBytes]) + "..."
	}
	return hex.EncodeToString(data)
}
