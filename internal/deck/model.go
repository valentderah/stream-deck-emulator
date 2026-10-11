package deck

import "encoding/binary"

// Size is a display size in pixels. The zero Size means the display is absent.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

func (s Size) Empty() bool { return s.W == 0 || s.H == 0 }

// Model describes one Stream Deck variant. Everything a Device does that
// differs between variants is derived from it.
type Model struct {
	ID        string
	Name      string // USB product string
	ProductID uint16
	Serial    string

	Rows, Cols int
	KeySize    int // key image edge in pixels
	// Rotated reports that the host sends key and screen images rotated by
	// 180 degrees because the panel is mounted upside down.
	Rotated bool

	Dials     int
	Strip     Size // touch strip
	Screen    Size // info screen
	TouchKeys int  // sensor keys, reported after the regular keys

	// Features holds the payloads of the static feature reports by report id.
	Features map[byte][]byte
}

func (m Model) Keys() int { return m.Rows * m.Cols }

// unitInfo builds the payload of featureUnitInfo: rows, columns, key width,
// key height, panel width, panel height, then a tail of unknown meaning.
func unitInfo(rows, cols, keySize, panelW, panelH int, tail ...byte) []byte {
	info := []byte{byte(rows), byte(cols)}
	for _, v := range []int{keySize, keySize, panelW, panelH} {
		info = binary.LittleEndian.AppendUint16(info, uint16(v))
	}
	return append(info, tail...)
}

// features returns the static feature reports of a model. Only the unit
// info differs between models; the rest is replayed from a real device.
func features(unit []byte) map[byte][]byte {
	return map[byte][]byte{
		featureUnitInfo:  unit,
		featureUnknown09: {0x01, 0x00, 0x01, 0x00, 0xd3, 0x29},
		featureUnknown0a: {0x04},
		featureUnknown0b: {0x02, byte(bcdDevice & 0xff), byte(bcdDevice >> 8)},
		featureUnknown0c: {0x08},
	}
}

var Plus = Model{
	ID:        "plus",
	Name:      "Stream Deck +",
	ProductID: 0x0084,
	Serial:    "EMU00000000001",
	Rows:      2,
	Cols:      4,
	KeySize:   120,
	Dials:     4,
	Strip:     Size{W: 800, H: 100},
	Features:  features(unitInfo(2, 4, 120, 800, 480, 0x04, 0x05, 0x01, 0x04, 0x04)),
}

// Neo has no reference dump: the panel size in its unit info is a guess.
var Neo = Model{
	ID:        "neo",
	Name:      "Stream Deck Neo",
	ProductID: 0x009a,
	Serial:    "EMU00000000002",
	Rows:      2,
	Cols:      4,
	KeySize:   96,
	Rotated:   true,
	Screen:    Size{W: 248, H: 58},
	TouchKeys: 2,
	Features:  features(unitInfo(2, 4, 96, 480, 320)),
}

var MK2 = Model{
	ID:        "mk2",
	Name:      "Stream Deck MK.2",
	ProductID: 0x0080,
	Serial:    "EMU00000000003",
	Rows:      3,
	Cols:      5,
	KeySize:   72,
	Rotated:   true,
	Features:  features(unitInfo(3, 5, 72, 480, 272)),
}

// Models lists the emulated variants in bus order.
func Models() []Model { return []Model{Plus, Neo, MK2} }

func ModelByID(id string) (Model, bool) {
	for _, m := range Models() {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}
