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
	// QuarterTurn reports key and strip-region images sent 90 degrees
	// counter-clockwise. Strip coordinates remain in display orientation.
	QuarterTurn bool

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

// The models below have no reference dump. Product ids marked "placeholder"
// do not belong to a real Elgato product; the others, key sizes, rotation and
// panel sizes come from open-source protocol descriptions and are unverified.

var XL = Model{
	ID:        "xl",
	Name:      "Stream Deck XL",
	ProductID: 0x006c,
	Serial:    "EMU00000000004",
	Rows:      4,
	Cols:      8,
	KeySize:   96,
	Rotated:   true,
	Features:  features(unitInfo(4, 8, 96, 1024, 600)),
}

// PlusXL has the Plus's dials and touch strip over a 9x4 key grid and six
// dials, each of which also presses.
var PlusXL = Model{
	ID:          "plus-xl",
	Name:        "Stream Deck + XL",
	ProductID:   0x00c6,
	Serial:      "EMU00000000005",
	Rows:        4,
	Cols:        9,
	KeySize:     120,
	Dials:       6,
	QuarterTurn: true,
	Strip:       Size{W: 1200, H: 100},
	Features:    features(unitInfo(4, 9, 120, 1200, 760, 0x04, 0x05, 0x01, 0x04, 0x04)),
}

// Module is the 15 key module, which behaves like the MK.2.
var Module = Model{
	ID:        "module",
	Name:      "Stream Deck Module",
	ProductID: 0x00b9,
	Serial:    "EMU00000000006",
	Rows:      3,
	Cols:      5,
	KeySize:   72,
	Rotated:   true,
	Features:  features(unitInfo(3, 5, 72, 480, 272)),
}

// ModuleVert is the module mounted upright. The product id is a placeholder.
var ModuleVert = Model{
	ID:        "module-vert",
	Name:      "Stream Deck Module Vertical",
	ProductID: 0xf001,
	Serial:    "EMU00000000007",
	Rows:      5,
	Cols:      3,
	KeySize:   72,
	Rotated:   true,
	Features:  features(unitInfo(5, 3, 72, 272, 480)),
}

var Studio = Model{
	ID:        "studio",
	Name:      "Stream Deck Studio",
	ProductID: 0x00aa,
	Serial:    "EMU00000000008",
	Rows:      2,
	Cols:      16,
	KeySize:   144,
	Features:  features(unitInfo(2, 16, 144, 2304, 288)),
}

// Mobile has no USB product; the product id is a placeholder.
var Mobile = Model{
	ID:        "mobile",
	Name:      "Stream Deck Mobile",
	ProductID: 0xf002,
	Serial:    "EMU00000000009",
	Rows:      2,
	Cols:      4,
	KeySize:   96,
	Features:  features(unitInfo(2, 4, 96, 480, 320)),
}

// CorsairKB has no Elgato USB product; the product id is a placeholder.
var CorsairKB = Model{
	ID:        "corsair-kb",
	Name:      "Stream Deck Corsair Keyboard",
	ProductID: 0xf003,
	Serial:    "EMU00000000010",
	Rows:      6,
	Cols:      1,
	KeySize:   72,
	Features:  features(unitInfo(6, 1, 72, 72, 432)),
}

// Pedal has three keys and no displays: images sent to it are ignored.
var Pedal = Model{
	ID:        "pedal",
	Name:      "Stream Deck Pedal",
	ProductID: 0x0086,
	Serial:    "EMU00000000011",
	Rows:      1,
	Cols:      3,
	Features:  features(unitInfo(1, 3, 0, 0, 0)),
}

// Models lists the emulated variants in bus order.
func Models() []Model {
	return []Model{Plus, Neo, MK2, XL, PlusXL, Module, ModuleVert, Studio, Mobile, CorsairKB, Pedal}
}

func ModelByID(id string) (Model, bool) {
	for _, m := range Models() {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}
