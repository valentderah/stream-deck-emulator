package deck

// Device identity shared by all models.
const (
	vendorID     = 0x0fd9
	bcdDevice    = 0x0100
	manufacturer = "Elgato"

	firmwareLD  = "1.00.003"
	firmwareAP2 = "3.01.005"
	firmwareAP1 = "1.01.000"
)

// Report sizes, report id included.
const (
	inputReportSize   = 512
	outputReportSize  = 1024
	featureReportSize = 32
)

// Report ids.
const (
	reportIDInput   = 0x01
	reportIDOutput  = 0x02
	reportIDCommand = 0x03 // feature report carrying a command from the host
)

// Feature reports the host reads. featureFirst through featureLast are
// declared in the report descriptor.
const (
	featureFirst       = reportIDCommand
	featureFirmwareLD  = 0x04
	featureFirmwareAP2 = 0x05
	featureSerial      = 0x06
	featureFirmwareAP1 = 0x07
	featureUnitInfo    = 0x08
	// The meaning of these is unknown; their payloads are replayed as is.
	featureUnknown09 = 0x09
	featureUnknown0a = 0x0a
	featureUnknown0b = 0x0b
	featureUnknown0c = 0x0c
	featureLast      = featureUnknown0c
)

// Layout of the feature reports the host reads.
const (
	offFeaturePayload  = 1
	offFirmwareLength  = 1
	offFirmwareVersion = 6
	firmwareLength     = 0x0c // the value real devices put before a version
	offSerialLength    = 1
	offSerial          = 2
)

// Commands in reportIDCommand: the command byte, then its arguments.
const (
	offCommand     = 1
	offCommandArgs = 2

	cmdReset         = 0x02 // no arguments
	cmdTouchKeyColor = 0x06 // key, red, green, blue
	cmdBrightness    = 0x08 // percent

	lenBrightnessArgs    = 1
	lenTouchKeyColorArgs = 4
	// Offset inside the arguments of cmdBrightness.
	offBrightnessPercent = 0
	// Offsets inside the arguments of cmdTouchKeyColor.
	offColorKey   = 0
	offColorRed   = 1
	offColorGreen = 2
	offColorBlue  = 3
)

// Output reports: reportIDOutput, a command byte, then a header and image
// data.
const (
	offOutputCommand = 1

	outKeyImage    = 0x07
	outScreenImage = 0x0b
	outStripImage  = 0x0c
)

// Header of a key image or info screen chunk. The key byte is unused for
// the info screen.
const (
	offChunkKey  = 2
	offChunkLast = 3
	offChunkSize = 4
	offChunkPart = 6
	chunkHeader  = 8
)

// Header of a touch strip chunk: the target rectangle, then the chunk.
const (
	offStripX    = 2
	offStripY    = 4
	offStripW    = 6
	offStripH    = 8
	offStripLast = 10
	offStripPart = 11
	offStripSize = 13
	stripHeader  = 16
)

// minOutputReport is the shortest output report the device looks at.
const minOutputReport = stripHeader

// Input reports: reportIDInput, a kind byte, the payload length, then the
// payload.
const (
	offInputKind   = 1
	offInputLength = 2
	inputHeader    = 4

	inputKeys  = 0x00 // one state byte per key, touch keys last
	inputTouch = 0x02
	inputDials = 0x03
)

// Payload of inputDials: an action byte, then one byte per dial.
const (
	offDialAction = inputHeader
	offDialValues = inputHeader + 1

	dialPress  = 0x00 // values are 0 or 1
	dialRotate = 0x01 // values are signed ticks
)

// Payload of inputTouch.
const (
	offTouchKind = inputHeader
	offTouchX    = inputHeader + 2
	offTouchY    = inputHeader + 4
	offTouchX2   = inputHeader + 6 // drag only
	offTouchY2   = inputHeader + 8 // drag only
	lenTouch     = 10
)

// Touch kinds as they appear in the input report.
const (
	TouchShort = 1
	TouchLong  = 2
	TouchDrag  = 3
)
