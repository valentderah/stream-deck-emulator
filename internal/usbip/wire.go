package usbip

// Handshake: protocol version, operation codes and reply status.
const (
	protocolVersion = 0x0111

	opReqDevlist = 0x8005
	opRepDevlist = 0x0005
	opReqImport  = 0x8003
	opRepImport  = 0x0003

	opStatusOK    = 0
	opStatusError = 1
)

// Handshake layout.
const (
	opHeaderSize = 8
	offOpCode    = 2
	pathSize     = 256
	busIDSize    = 32
)

// Device records in handshake replies.
const (
	sysPathPrefix = "/sys/devices/pci0000:00/0000:00:01.2/usb1/"
	busNum        = 1
	firstDevNum   = 2
)

// URB commands, directions and endpoints.
const (
	cmdSubmit = 1
	cmdUnlink = 2
	retSubmit = 3
	retUnlink = 4

	dirOut = 0
	dirIn  = 1

	controlEndpoint = 0
)

// URB completion status: zero or a negated Linux errno.
const (
	statusOK        = 0
	statusStall     = -32  // -EPIPE
	statusConnReset = -104 // -ECONNRESET
)

// URB header layout.
const (
	urbHeaderSize = 48

	offURBCommand   = 0
	offURBSeqNum    = 4
	offURBDirection = 12
	offURBEndpoint  = 16
	offURBStatus    = 20 // replies
	offURBUnlinkSeq = 20 // CMD_UNLINK: the seqnum to cancel
	offURBLength    = 24 // transfer buffer length, or actual length in replies
	offURBSetup     = 40
	setupSize       = 8

	// maxTransferLength bounds transfer_buffer_length. The largest transfer
	// the emulated devices send or receive is a 1024-byte output report.
	maxTransferLength = 64 << 10
)

// Setup packet layout. Its fields are little-endian, unlike the rest.
const (
	offSetupRequestType = 0
	offSetupRequest     = 1
	offSetupValue       = 2
	offSetupIndex       = 4
	offSetupLength      = 6
)
