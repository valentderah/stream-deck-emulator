package deck

import (
	"encoding/binary"
	"log"
)

// SetKey presses or releases a key. Touch keys follow the regular keys.
func (d *Device) SetKey(key int, down bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if key < 0 || key >= len(d.keyDown) {
		return ErrInput
	}
	d.keyDown[key] = down
	r := newInputReport(inputKeys, len(d.keyDown))
	for i, v := range d.keyDown {
		r[inputHeader+i] = boolByte(v)
	}
	d.enqueue(r)
	return nil
}

// PressDial presses or releases a dial.
func (d *Device) PressDial(dial int, down bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if dial < 0 || dial >= len(d.dialDown) {
		return ErrInput
	}
	d.dialDown[dial] = down
	r := d.newDialReport(dialPress)
	for i, v := range d.dialDown {
		r[offDialValues+i] = boolByte(v)
	}
	d.enqueue(r)
	return nil
}

func (d *Device) RotateDial(dial int, ticks int8) error {
	if dial < 0 || dial >= d.model.Dials {
		return ErrInput
	}
	r := d.newDialReport(dialRotate)
	r[offDialValues+dial] = byte(ticks)
	d.send(r)
	return nil
}

// Touch reports a tap, a long press or a drag from (x, y) to (x2, y2) on the
// touch strip.
func (d *Device) Touch(kind uint8, x, y, x2, y2 uint16) error {
	strip := d.model.Strip
	inside := func(x, y uint16) bool { return int(x) < strip.W && int(y) < strip.H }
	if kind < TouchShort || kind > TouchDrag || !inside(x, y) || (kind == TouchDrag && !inside(x2, y2)) {
		return ErrInput
	}
	le := binary.LittleEndian
	r := newInputReport(inputTouch, lenTouch)
	r[offTouchKind] = kind
	le.PutUint16(r[offTouchX:], x)
	le.PutUint16(r[offTouchY:], y)
	if kind == TouchDrag {
		le.PutUint16(r[offTouchX2:], x2)
		le.PutUint16(r[offTouchY2:], y2)
	}
	d.send(r)
	return nil
}

func newInputReport(kind byte, payload int) []byte {
	r := make([]byte, inputReportSize)
	r[0] = reportIDInput
	r[offInputKind] = kind
	binary.LittleEndian.PutUint16(r[offInputLength:], uint16(payload))
	return r
}

// newDialReport returns a dial report with the action set and all values
// zero.
func (d *Device) newDialReport(action byte) []byte {
	r := newInputReport(inputDials, 1+d.model.Dials)
	r[offDialAction] = action
	return r
}

func (d *Device) send(report []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.enqueue(report)
}

// enqueue queues a report for the host without blocking. d.mu must be held,
// so a state change and its report stay in order.
func (d *Device) enqueue(report []byte) {
	if !d.attached {
		return
	}
	select {
	case d.input <- report:
	default:
		log.Printf("deck: %s: input queue full, report dropped", d.model.ID)
	}
}
