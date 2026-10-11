package deck

import (
	"encoding/binary"
	"unicode/utf16"
)

// reportDescriptor mirrors the layout of a Stream Deck MK.2 as seen through
// HidP_GetCaps: one vendor usage on the Consumer page, one input report, one
// output report and the feature reports. All models share it.
var reportDescriptor = buildReportDescriptor()

func buildReportDescriptor() []byte {
	le := binary.LittleEndian
	d := []byte{
		itemUsagePage, usagePageConsumer,
		itemUsage, usageConsumerControl,
		itemCollection, collectionApplication,
	}
	// report declares a report of size-1 opaque bytes after the report id.
	report := func(id byte, size uint16, mainTag, mainData byte) {
		d = append(d, itemUsage16)
		d = le.AppendUint16(d, usageVendor)
		d = append(d, itemLogicalMin, fieldMin, itemLogicalMax16)
		d = le.AppendUint16(d, fieldMax)
		d = append(d, itemReportSize, fieldBits, itemReportCount16)
		d = le.AppendUint16(d, size-1)
		d = append(d, itemReportID, id, mainTag, mainData)
	}
	report(reportIDInput, inputReportSize, itemInput, mainDataVarAbs)
	report(reportIDOutput, outputReportSize, itemOutput, mainDataVarAbs)
	for id := byte(featureFirst); id <= featureLast; id++ {
		report(id, featureReportSize, itemFeature, mainDataArrayRel)
	}
	return append(d, itemEndCollection)
}

func (m Model) deviceDescriptor() []byte {
	le := binary.LittleEndian
	d := []byte{lenDeviceDesc, descDevice}
	d = le.AppendUint16(d, bcdUSB)
	d = append(d, 0, 0, 0, maxPacketSize0) // class, subclass, protocol: set per interface
	d = le.AppendUint16(d, vendorID)
	d = le.AppendUint16(d, m.ProductID)
	d = le.AppendUint16(d, bcdDevice)
	return append(d, stringManufacturer, stringProduct, stringSerial, numConfigurations)
}

func hidDescriptor() []byte {
	le := binary.LittleEndian
	d := []byte{lenHIDDesc, descHID}
	d = le.AppendUint16(d, bcdHID)
	d = append(d, countryNone, numClassDescs, descReport)
	return le.AppendUint16(d, uint16(len(reportDescriptor)))
}

func configDescriptor() []byte {
	le := binary.LittleEndian
	d := []byte{
		lenConfigDesc, descConfiguration,
		0, 0, // wTotalLength, patched below
		numInterfaces, configValue,
		0, // iConfiguration
		attrBusPowered, maxPower500mA,

		lenInterfaceDesc, descInterface,
		0, 0, // bInterfaceNumber, bAlternateSetting
		numEndpoints,
		classHID, 0, 0, // no subclass, no protocol
		0, // iInterface
	}
	d = append(d, hidDescriptor()...)
	endpoint := func(address byte, packetSize uint16) {
		d = append(d, lenEndpointDesc, descEndpoint, address, transferInterrupt)
		d = le.AppendUint16(d, packetSize)
		d = append(d, pollIntervalMs)
	}
	endpoint(endpointIn, inputReportSize)
	endpoint(endpointOut, outputReportSize)
	le.PutUint16(d[offConfigTotalLength:], uint16(len(d)))
	return d
}

func (m Model) stringDescriptor(index uint8) ([]byte, bool) {
	le := binary.LittleEndian
	var s string
	switch index {
	case stringLangIDs:
		return le.AppendUint16([]byte{lenLangIDDesc, descString}, langEnglishUS), true
	case stringManufacturer:
		s = manufacturer
	case stringProduct:
		s = m.Name
	case stringSerial:
		s = m.Serial
	default:
		return nil, false
	}
	units := utf16.Encode([]rune(s))
	d := []byte{byte(lenStringHeader + 2*len(units)), descString}
	for _, u := range units {
		d = le.AppendUint16(d, u)
	}
	return d, true
}
