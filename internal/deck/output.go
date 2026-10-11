package deck

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/jpeg"
	"log"
)

// Out handles an output report: a chunk of a key image, of a touch strip region or of the info screen.
func (d *Device) Out(_ uint32, data []byte) {
	if len(data) < minOutputReport || data[0] != reportIDOutput {
		log.Printf("deck: %s: output report %s", d.model.ID, hexPrefix(data))
		return
	}
	switch cmd := data[offOutputCommand]; {
	case cmd == outKeyImage:
		d.keyChunk(data)
	case cmd == outStripImage && !d.model.Strip.Empty():
		d.stripChunk(data)
	case cmd == outScreenImage && !d.model.Screen.Empty():
		d.screenChunk(data)
	default:
		log.Printf("deck: %s: output report %s", d.model.ID, hexPrefix(data))
	}
}

// chunk is one piece of an image that the host sends in several reports.
type chunk struct {
	key  int
	part uint16
	last bool
	data []byte
}

// parseChunk reads the header shared by key image and info screen reports.
func parseChunk(report []byte) (chunk, bool) {
	le := binary.LittleEndian
	size := int(le.Uint16(report[offChunkSize:]))
	if chunkHeader+size > len(report) {
		return chunk{}, false
	}
	return chunk{
		key:  int(report[offChunkKey]),
		part: le.Uint16(report[offChunkPart:]),
		last: report[offChunkLast] != 0,
		data: report[chunkHeader : chunkHeader+size],
	}, true
}

// appendChunk adds a chunk to a partial image and returns the whole image
// once the last chunk has arrived. A nil buffer means no image is in
// progress: chunks other than part 0 are dropped, so an image that overflowed
// or was interrupted by a reset never publishes its tail.
func appendChunk(buf *[]byte, part uint16, last bool, data []byte) []byte {
	if part == 0 {
		*buf = make([]byte, 0, len(data))
	}
	if *buf == nil {
		return nil
	}
	if len(*buf)+len(data) > maxImageBytes {
		*buf = nil
		return nil
	}
	*buf = append(*buf, data...)
	if !last {
		return nil
	}
	img := *buf
	*buf = nil
	if len(img) == 0 {
		return nil
	}
	return img
}

func (d *Device) keyChunk(report []byte) {
	c, ok := parseChunk(report)
	if !ok || c.key >= d.model.Keys() {
		log.Printf("deck: %s: bad key image chunk %s", d.model.ID, hexPrefix(report))
		return
	}
	d.mu.Lock()
	img := appendChunk(&d.keyParts[c.key], c.part, c.last, c.data)
	if img != nil {
		d.keyImages[c.key] = img
	}
	d.mu.Unlock()
	if img != nil {
		d.publish(Event{Kind: EventKey, Index: c.key})
	}
}

func (d *Device) screenChunk(report []byte) {
	c, ok := parseChunk(report)
	if !ok {
		log.Printf("deck: %s: bad screen chunk %s", d.model.ID, hexPrefix(report))
		return
	}
	d.mu.Lock()
	img := appendChunk(&d.screenPart, c.part, c.last, c.data)
	if img != nil {
		d.screen = img
	}
	d.mu.Unlock()
	if img != nil {
		d.publish(Event{Kind: EventScreen})
	}
}

func (d *Device) stripChunk(report []byte) {
	le := binary.LittleEndian
	x, y := int(le.Uint16(report[offStripX:])), int(le.Uint16(report[offStripY:]))
	w, h := int(le.Uint16(report[offStripW:])), int(le.Uint16(report[offStripH:]))
	last, part := report[offStripLast] != 0, le.Uint16(report[offStripPart:])
	size := int(le.Uint16(report[offStripSize:]))
	if stripHeader+size > len(report) {
		log.Printf("deck: %s: bad strip chunk %s", d.model.ID, hexPrefix(report))
		return
	}
	d.mu.Lock()
	encoded := appendChunk(&d.stripPart, part, last, report[stripHeader:stripHeader+size])
	d.mu.Unlock()
	if encoded == nil {
		return
	}
	img, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil {
		log.Printf("deck: %s: decode strip region %dx%d at %d,%d: %v", d.model.ID, w, h, x, y, err)
		return
	}
	d.mu.Lock()
	draw.Draw(d.strip, image.Rect(x, y, x+w, y+h), img, img.Bounds().Min, draw.Src)
	d.mu.Unlock()
	d.publish(Event{Kind: EventStrip})
}
