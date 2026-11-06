package deck

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"testing"
)

func TestRotateClockwise(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 13, 22))
	for y := 20; y < 22; y++ {
		for x := 10; x < 13; x++ {
			src.SetRGBA(x, y, color.RGBA{R: byte(x), G: byte(y), A: 255})
		}
	}
	got := rotateClockwise(src)
	if got.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatalf("bounds %v", got.Bounds())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			want := src.RGBAAt(10+y, 21-x)
			if c := color.RGBAModel.Convert(got.At(x, y)); c != want {
				t.Errorf("pixel %d,%d: got %v, want %v", x, y, c, want)
			}
		}
	}
}

func TestStripRegionOrientation(t *testing.T) {
	red := color.RGBA{R: 240, A: 255}
	green := color.RGBA{G: 240, A: 255}
	blue := color.RGBA{B: 240, A: 255}
	yellow := color.RGBA{R: 240, G: 240, A: 255}
	for _, m := range []Model{Plus, PlusXL} {
		t.Run(m.ID, func(t *testing.T) {
			const w, h = 80, 40
			size := image.Rect(0, 0, w, h)
			corners := []color.RGBA{red, green, blue, yellow}
			if m.ID == "plus-xl" {
				size = image.Rect(0, 0, h, w)
				corners = []color.RGBA{green, yellow, red, blue}
			}
			src := image.NewRGBA(size)
			halfW, halfH := size.Dx()/2, size.Dy()/2
			for i, c := range corners {
				x, y := (i%2)*halfW, (i/2)*halfH
				draw.Draw(src, image.Rect(x, y, x+halfW, y+halfH), image.NewUniform(c), image.Point{}, draw.Src)
			}
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, src, &jpeg.Options{Quality: 100}); err != nil {
				t.Fatal(err)
			}
			d := attached(m)
			events, cancel := d.Subscribe()
			defer cancel()
			data := encoded.Bytes()
			partSize := min(chunkPayload, len(data)/2)
			for part, off := uint16(0), 0; off < len(data); part++ {
				end := min(off+partSize, len(data))
				d.Out(0, stripChunk(250, 20, w, h, part, end == len(data), data[off:end]))
				if end < len(data) {
					if c := d.Strip().RGBAAt(270, 30); c.R != 0 || c.G != 0 || c.B != 0 {
						t.Errorf("incomplete region was displayed: %v", c)
					}
					select {
					case e := <-events:
						t.Errorf("event for incomplete region: %+v", e)
					default:
					}
				}
				off = end
			}
			select {
			case e := <-events:
				if e.Kind != EventStrip {
					t.Errorf("event %+v", e)
				}
			default:
				t.Fatal("no strip event")
			}
			strip := d.Strip()
			for i, want := range []color.RGBA{red, green, blue, yellow} {
				x, y := 250+20+(i%2)*40, 20+10+(i/2)*20
				got := strip.RGBAAt(x, y)
				for _, diff := range []int{int(got.R) - int(want.R), int(got.G) - int(want.G), int(got.B) - int(want.B)} {
					if diff < -3 || diff > 3 {
						t.Errorf("corner %d: got %v, want %v", i, got, want)
						break
					}
				}
			}
			for _, p := range []image.Point{{249, 20}, {330, 20}, {250, 19}, {250, 60}} {
				if c := strip.RGBAAt(p.X, p.Y); c.R != 0 || c.G != 0 || c.B != 0 {
					t.Errorf("outside region at %v: %v", p, c)
				}
			}
			// A later region must not rotate or overwrite the existing strip.
			data = solidJPEG(t, 16, 16, green)
			d.Out(0, stripChunk(500, 50, 16, 16, 0, true, data))
			strip = d.Strip()
			if c := strip.RGBAAt(270, 30); c.R < 230 || c.G > 3 {
				t.Errorf("first region changed: %v", c)
			}
			if c := strip.RGBAAt(508, 58); c.G < 230 || c.R > 3 {
				t.Errorf("second region missing: %v", c)
			}
		})
	}
}
