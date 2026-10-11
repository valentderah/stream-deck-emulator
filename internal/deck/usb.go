package deck

// bmRequestType values of the control requests the device answers.
const (
	reqTypeDeviceIn     = 0x80
	reqTypeDeviceOut    = 0x00
	reqTypeInterfaceIn  = 0x81
	reqTypeInterfaceOut = 0x01
	reqTypeEndpointOut  = 0x02
	reqTypeHIDIn        = 0xa1
	reqTypeHIDOut       = 0x21
)

// Standard requests.
const (
	reqGetStatus        = 0x00
	reqClearFeature     = 0x01
	reqGetDescriptor    = 0x06
	reqGetConfiguration = 0x08
	reqSetConfiguration = 0x09
	reqSetInterface     = 0x0b
)

// HID class requests.
const (
	hidGetReport   = 0x01
	hidSetReport   = 0x09
	hidSetIdle     = 0x0a
	hidSetProtocol = 0x0b
)

// HID report types: the high byte of wValue in GET_REPORT and SET_REPORT.
const (
	reportTypeInput   = 0x01
	reportTypeOutput  = 0x02
	reportTypeFeature = 0x03
)

// Descriptor types.
const (
	descDevice        = 0x01
	descConfiguration = 0x02
	descString        = 0x03
	descInterface     = 0x04
	descEndpoint      = 0x05
	descHID           = 0x21
	descReport        = 0x22
)

// Descriptor lengths.
const (
	lenDeviceDesc    = 18
	lenConfigDesc    = 9
	lenInterfaceDesc = 9
	lenHIDDesc       = 9
	lenEndpointDesc  = 7
	lenLangIDDesc    = 4
	lenStringHeader  = 2
	lenStatus        = 2 // GET_STATUS reply

	offConfigTotalLength = 2
)

// Descriptor field values.
const (
	bcdUSB            = 0x0200
	bcdHID            = 0x0111
	maxPacketSize0    = 64
	classHID          = 0x03
	countryNone       = 0x00
	configValue       = 1
	numConfigurations = 1
	numInterfaces     = 1
	numEndpoints      = 2
	numClassDescs     = 1
	attrBusPowered    = 0x80
	maxPower500mA     = 250 // in units of 2 mA
	endpointIn        = 0x81
	endpointOut       = 0x02
	transferInterrupt = 0x03
	pollIntervalMs    = 1
	langEnglishUS     = 0x0409
)

// String descriptor indexes.
const (
	stringLangIDs = iota
	stringManufacturer
	stringProduct
	stringSerial
)

// HID report descriptor item prefixes. The 16 suffix marks a two-byte value.
const (
	itemUsagePage     = 0x05
	itemUsage         = 0x09
	itemUsage16       = 0x0a
	itemCollection    = 0xa1
	itemEndCollection = 0xc0
	itemLogicalMin    = 0x15
	itemLogicalMax16  = 0x26
	itemReportSize    = 0x75
	itemReportCount16 = 0x96
	itemReportID      = 0x85
	itemInput         = 0x81
	itemOutput        = 0x91
	itemFeature       = 0xb1
)

// HID report descriptor item values.
const (
	usagePageConsumer     = 0x0c
	usageConsumerControl  = 0x01
	usageVendor           = 0xff00
	collectionApplication = 0x01
	mainDataVarAbs        = 0x02 // Data, Variable, Absolute
	mainDataArrayRel      = 0x04 // Data, Array, Relative
	fieldBits             = 8
	fieldMin              = 0x00
	fieldMax              = 0xff
)
