package deck

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"sync"
	"testing"
)

const chunkPayload = outputReportSize - stripHeader

func keyChunk(key int, part uint16, last bool, payload []byte) []byte {
	r := make([]byte, outputReportSize)
	r[0], r[1], r[2], r[3] = 0x02, 0x07, byte(key), boolByte(last)
	binary.LittleEndian.PutUint16(r[4:], uint16(len(payload)))
	binary.LittleEndian.PutUint16(r[6:], part)
	copy(r[chunkHeader:], payload)
	return r
}

func stripChunk(x, y, w, h int, part uint16, last bool, payload []byte) []byte {
	le := binary.LittleEndian
	r := make([]byte, outputReportSize)
	r[0], r[1] = 0x02, 0x0c
	le.PutUint16(r[2:], uint16(x))
	le.PutUint16(r[4:], uint16(y))
	le.PutUint16(r[6:], uint16(w))
	le.PutUint16(r[8:], uint16(h))
	r[10] = boolByte(last)
	le.PutUint16(r[11:], part)
	le.PutUint16(r[13:], uint16(len(payload)))
	copy(r[stripHeader:], payload)
	return r
}

func solidJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func attached(m Model) *Device {
	d := New(m)
	d.SetAttached(true)
	return d
}

func TestInputDroppedWhileDetached(t *testing.T) {
	d := New(Plus)
	if err := d.SetKey(0, true); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-d.In():
		t.Fatalf("got report %x while detached", r[:8])
	default:
	}
}

func TestDetachDrainsQueue(t *testing.T) {
	d := attached(Plus)
	d.SetKey(0, true)
	d.SetAttached(false)
	d.SetAttached(true)
	select {
	case <-d.In():
		t.Fatal("stale report survived a detach")
	default:
	}
}

func TestResetReleasesKeys(t *testing.T) {
	d := attached(Plus)
	d.SetKey(2, true)
	d.PressDial(1, true)
	d.Reset()
	d.SetAttached(true)
	d.SetKey(0, true)
	r := <-d.In()
	if r[4+2] != 0 {
		t.Errorf("key 2 still pressed after Reset: %x", r[4:12])
	}
	d.PressDial(0, true)
	r = <-d.In()
	if r[5+1] != 0 {
		t.Errorf("dial 1 still pressed after Reset: %x", r[4:9])
	}
}

func TestResetClearsDisplay(t *testing.T) {
	d := attached(Plus)
	d.Out(0, keyChunk(1, 0, true, []byte{1, 2, 3}))
	events, cancel := d.Subscribe()
	defer cancel()
	d.Reset()
	if d.KeyImage(1) != nil {
		t.Error("key image survived Reset")
	}
	if e := <-events; e.Kind != EventReset {
		t.Errorf("got event %q, want reset", e.Kind)
	}
}

func TestInputErrors(t *testing.T) {
	d := attached(Plus)
	for name, err := range map[string]error{
		"key out of range":    d.SetKey(8, true),
		"negative key":        d.SetKey(-1, true),
		"dial out of range":   d.PressDial(4, true),
		"rotate out of range": d.RotateDial(4, 1),
		"touch kind":          d.Touch(9, 0, 0, 0, 0),
		"touch x":             d.Touch(TouchShort, 800, 0, 0, 0),
		"touch y":             d.Touch(TouchShort, 0, 100, 0, 0),
	} {
		if !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v, want ErrInput", name, err)
		}
	}
}

func TestKeyImageChunks(t *testing.T) {
	d := attached(Plus)
	events, cancel := d.Subscribe()
	defer cancel()
	d.Out(0, keyChunk(5, 0, false, []byte{1, 2}))
	if d.KeyImage(5) != nil {
		t.Fatal("image published before the last chunk")
	}
	d.Out(0, keyChunk(5, 1, true, []byte{3}))
	if got := d.KeyImage(5); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("got %v", got)
	}
	if e := <-events; e.Kind != EventKey || e.Index != 5 {
		t.Errorf("got event %+v", e)
	}
}

func TestKeyImageOutOfRange(t *testing.T) {
	d := attached(Plus)
	d.Out(0, keyChunk(8, 0, true, []byte{1}))
	if d.KeyImage(8) != nil || d.KeyImage(-1) != nil {
		t.Error("out-of-range key returned an image")
	}
}

func TestChunkCap(t *testing.T) {
	d := attached(Plus)
	payload := make([]byte, outputReportSize-chunkHeader)
	for part := 0; part <= maxImageBytes/len(payload)+1; part++ {
		d.Out(0, keyChunk(0, uint16(part), false, payload))
	}
	d.mu.Lock()
	n := len(d.keyParts[0])
	d.mu.Unlock()
	if n > maxImageBytes {
		t.Errorf("partial image grew to %d bytes", n)
	}
}

func TestStripRegion(t *testing.T) {
	d := attached(Plus)
	img := solidJPEG(t, 200, 100, color.RGBA{R: 200, A: 255})
	for part, off := uint16(0), 0; off < len(img); part++ {
		end := min(off+chunkPayload, len(img))
		d.Out(0, stripChunk(400, 0, 200, 100, part, end == len(img), img[off:end]))
		off = end
	}
	strip := d.Strip()
	if c := strip.RGBAAt(500, 50); c.R < 150 || c.G > 60 {
		t.Errorf("inside the region: %+v", c)
	}
	if c := strip.RGBAAt(100, 50); c.R != 0 {
		t.Errorf("outside the region: %+v", c)
	}
}

func TestBrightnessAndFirmwareReset(t *testing.T) {
	d := attached(Plus)
	d.Out(0, keyChunk(0, 0, true, []byte{1}))
	d.setFeature([]byte{0x03, 0x08, 40})
	if d.Brightness() != 40 {
		t.Errorf("brightness %d", d.Brightness())
	}
	d.setFeature([]byte{0x03, 0x02})
	if d.KeyImage(0) != nil {
		t.Error("key image survived the reset feature")
	}
	if d.Brightness() != 40 {
		t.Error("the reset feature changed brightness")
	}
}

func TestOverflowedImageIsDropped(t *testing.T) {
	d := attached(Plus)
	payload := make([]byte, outputReportSize-chunkHeader)
	parts := maxImageBytes/len(payload) + 2
	for part := 0; part < parts; part++ {
		d.Out(0, keyChunk(0, uint16(part), part == parts-1, payload))
	}
	if d.KeyImage(0) != nil {
		t.Fatal("overflowed image was published")
	}
	d.Out(0, keyChunk(0, 0, false, []byte{1, 2}))
	d.Out(0, keyChunk(0, 1, true, []byte{3}))
	if got := d.KeyImage(0); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("next image: got %v", got)
	}
}

func TestInterruptedImageIsDropped(t *testing.T) {
	d := attached(Plus)
	d.Out(0, keyChunk(0, 0, false, []byte{1, 2}))
	d.setFeature([]byte{0x03, 0x02})
	d.Out(0, keyChunk(0, 1, true, []byte{3}))
	if d.KeyImage(0) != nil {
		t.Fatal("tail of an interrupted image was published")
	}
	d.Out(0, keyChunk(0, 0, true, []byte{4}))
	if got := d.KeyImage(0); !bytes.Equal(got, []byte{4}) {
		t.Errorf("next image: got %v", got)
	}
}

func TestInterruptedStripImageIsDropped(t *testing.T) {
	d := attached(Plus)
	img := solidJPEG(t, 200, 100, color.RGBA{R: 200, A: 255})
	half := len(img) / 2
	d.Out(0, stripChunk(400, 0, 200, 100, 0, false, img[:half]))
	d.Reset()
	d.Out(0, stripChunk(400, 0, 200, 100, 1, true, img[half:]))
	if c := d.Strip().RGBAAt(500, 50); c.R != 0 {
		t.Errorf("tail of an interrupted strip image was drawn: %+v", c)
	}
}

func TestConcurrentKeyReportsStayOrdered(t *testing.T) {
	d := attached(Plus)
	const workers, rounds = 8, 30 // fewer reports than the queue holds
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				d.SetKey(w, i%2 == 0) // every call flips this worker's key
			}
		}()
	}
	wg.Wait()
	if len(d.In()) != workers*rounds {
		t.Fatalf("%d reports queued, want %d", len(d.In()), workers*rounds)
	}
	// Read in order, each report must differ from the previous one by exactly
	// one key, as the state changes did.
	prev := make([]byte, workers)
	for n := 0; len(d.In()) > 0; n++ {
		r := <-d.In()
		flips := 0
		for k := 0; k < workers; k++ {
			if r[4+k] != prev[k] {
				flips++
			}
		}
		if flips != 1 {
			t.Fatalf("report %d changes %d keys: %x -> %x", n, flips, prev, r[4:4+workers])
		}
		copy(prev, r[4:4+workers])
	}
}

func TestBrightnessClamped(t *testing.T) {
	d := attached(Plus)
	events, cancel := d.Subscribe()
	defer cancel()
	d.setFeature([]byte{0x03, 0x08, 0xff})
	if d.Brightness() != 100 {
		t.Errorf("brightness %d, want 100", d.Brightness())
	}
	if e := <-events; e.Kind != EventBrightness || e.Value != 100 {
		t.Errorf("got event %+v", e)
	}
}

func TestResetRestoresBrightness(t *testing.T) {
	d := attached(Plus)
	d.setFeature([]byte{0x03, 0x08, 10})
	events, cancel := d.Subscribe()
	defer cancel()
	d.Reset()
	if d.Brightness() != 100 {
		t.Errorf("brightness %d", d.Brightness())
	}
	if e := <-events; e.Kind != EventReset {
		t.Fatalf("got event %+v", e)
	}
	if e := <-events; e.Kind != EventBrightness || e.Value != 100 {
		t.Errorf("got event %+v", e)
	}
}
