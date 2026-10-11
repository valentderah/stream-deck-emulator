package deck

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func screenChunk(part uint16, last bool, payload []byte) []byte {
	r := make([]byte, outputReportSize)
	r[0], r[1], r[3] = 0x02, 0x0b, boolByte(last)
	binary.LittleEndian.PutUint16(r[4:], uint16(len(payload)))
	binary.LittleEndian.PutUint16(r[6:], part)
	copy(r[chunkHeader:], payload)
	return r
}

func TestModels(t *testing.T) {
	want := []struct {
		id      string
		pid     uint16
		keys    int
		keySize int
		rotated bool
	}{
		{"plus", 0x0084, 8, 120, false},
		{"neo", 0x009a, 8, 96, true},
		{"mk2", 0x0080, 15, 72, true},
	}
	models := Models()
	if len(models) != len(want) {
		t.Fatalf("got %d models", len(models))
	}
	for i, w := range want {
		m := models[i]
		if m.ID != w.id || m.ProductID != w.pid || m.Keys() != w.keys || m.KeySize != w.keySize || m.Rotated != w.rotated {
			t.Errorf("model %d: %+v", i, m)
		}
		desc := m.deviceDescriptor()
		if got := binary.LittleEndian.Uint16(desc[10:]); got != w.pid {
			t.Errorf("%s: descriptor product id %#04x", w.id, got)
		}
		if info := m.Features[0x08]; int(info[0])*int(info[1]) != w.keys || int(info[2]) != w.keySize {
			t.Errorf("%s: unit info %v", w.id, info)
		}
		if _, ok := ModelByID(w.id); !ok {
			t.Errorf("ModelByID(%q) failed", w.id)
		}
	}
	if _, ok := ModelByID("xl"); ok {
		t.Error("ModelByID accepted an unknown id")
	}
}

func TestSerialsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, m := range Models() {
		if other, ok := seen[m.Serial]; ok {
			t.Errorf("%s and %s share serial %q", m.ID, other, m.Serial)
		}
		seen[m.Serial] = m.ID
	}
}

func TestNeoTouchKeys(t *testing.T) {
	d := attached(Neo)
	if err := d.SetKey(9, true); err != nil {
		t.Fatal(err)
	}
	r := <-d.In()
	if !bytes.Equal(r[:4], []byte{0x01, 0x00, 0x0a, 0x00}) {
		t.Errorf("header %x", r[:4])
	}
	if r[4+9] != 1 || r[4+8] != 0 {
		t.Errorf("states %x", r[4:14])
	}
	if err := d.SetKey(10, true); !errors.Is(err, ErrInput) {
		t.Errorf("key 10: %v", err)
	}
}

func TestNeoHasNoDialsOrStrip(t *testing.T) {
	d := attached(Neo)
	if err := d.PressDial(0, true); !errors.Is(err, ErrInput) {
		t.Errorf("dial: %v", err)
	}
	if err := d.Touch(TouchShort, 0, 0, 0, 0); !errors.Is(err, ErrInput) {
		t.Errorf("touch: %v", err)
	}
	if d.Strip() != nil {
		t.Error("Neo has a strip")
	}
}

func TestNeoScreen(t *testing.T) {
	d := attached(Neo)
	events, cancel := d.Subscribe()
	defer cancel()
	d.Out(0, screenChunk(0, false, []byte{0xff, 0xd8}))
	if d.Screen() != nil {
		t.Fatal("screen published before the last chunk")
	}
	d.Out(0, screenChunk(1, true, []byte{0xff, 0xd9}))
	if got := d.Screen(); !bytes.Equal(got, []byte{0xff, 0xd8, 0xff, 0xd9}) {
		t.Errorf("got %x", got)
	}
	if e := <-events; e.Kind != EventScreen {
		t.Errorf("got event %+v", e)
	}
}

func TestScreenReportIgnoredWithoutScreen(t *testing.T) {
	d := attached(Plus)
	d.Out(0, screenChunk(0, true, []byte{1}))
	if d.Screen() != nil {
		t.Error("the + accepted an info screen image")
	}
}

func TestNeoTouchKeyColour(t *testing.T) {
	d := attached(Neo)
	events, cancel := d.Subscribe()
	defer cancel()
	d.setFeature([]byte{0x03, 0x06, 9, 0x11, 0x22, 0x33})
	if got := d.TouchColor(1); got != 0x112233 {
		t.Errorf("colour %#06x", got)
	}
	if e := <-events; e.Kind != EventTouchKey || e.Index != 1 || e.Value != 0x112233 {
		t.Errorf("got event %+v", e)
	}
	d.setFeature([]byte{0x03, 0x06, 3, 1, 2, 3})
	if d.TouchColor(0) != 0 || d.TouchColor(5) != 0 {
		t.Error("colour set for a key that is not a touch key")
	}
	d.Reset()
	if d.TouchColor(1) != 0 {
		t.Error("colour survived Reset")
	}
}

func TestMK2Keys(t *testing.T) {
	d := attached(MK2)
	if err := d.SetKey(14, true); err != nil {
		t.Fatal(err)
	}
	r := <-d.In()
	if !bytes.Equal(r[:4], []byte{0x01, 0x00, 0x0f, 0x00}) || r[4+14] != 1 {
		t.Errorf("report %x", r[:20])
	}
	d.Out(0, keyChunk(14, 0, true, []byte{7}))
	if !bytes.Equal(d.KeyImage(14), []byte{7}) {
		t.Error("key 14 image not stored")
	}
}

func TestSerialsAreSynthetic(t *testing.T) {
	for _, m := range Models() {
		if !strings.HasPrefix(m.Serial, "EMU") {
			t.Errorf("%s: serial %q does not look synthetic", m.ID, m.Serial)
		}
	}
}
