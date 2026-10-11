package deck

import "log"

func (d *Device) featureReport(id uint8) ([]byte, bool) {
	r := make([]byte, featureReportSize)
	r[0] = id
	firmware := func(version string) {
		r[offFirmwareLength] = firmwareLength
		copy(r[offFirmwareVersion:], version)
	}
	switch id {
	case reportIDCommand:
	case featureFirmwareLD:
		firmware(firmwareLD)
	case featureFirmwareAP2:
		firmware(firmwareAP2)
	case featureSerial:
		r[offSerialLength] = byte(len(d.model.Serial))
		copy(r[offSerial:], d.model.Serial)
	case featureFirmwareAP1:
		firmware(firmwareAP1)
	default:
		payload, ok := d.model.Features[id]
		if !ok {
			return nil, false
		}
		copy(r[offFeaturePayload:], payload)
	}
	return r, true
}

// setFeature handles a feature report written by the host: a command.
func (d *Device) setFeature(data []byte) {
	if len(data) > offCommand && data[0] == reportIDCommand {
		cmd, args := data[offCommand], data[offCommandArgs:]
		switch {
		case cmd == cmdBrightness && len(args) >= lenBrightnessArgs:
			percent := min(int(args[offBrightnessPercent]), maxBrightness)
			d.mu.Lock()
			d.brightness = percent
			d.mu.Unlock()
			d.publish(Event{Kind: EventBrightness, Value: percent})
			return
		case cmd == cmdReset:
			d.mu.Lock()
			d.clearDisplay()
			d.mu.Unlock()
			d.publish(Event{Kind: EventReset})
			return
		case cmd == cmdTouchKeyColor && len(args) >= lenTouchKeyColorArgs:
			// The host addresses touch keys by their key index.
			i := int(args[offColorKey]) - d.model.Keys()
			if i < 0 || i >= d.model.TouchKeys {
				break
			}
			rgb := packRGB(args[offColorRed], args[offColorGreen], args[offColorBlue])
			d.mu.Lock()
			d.touchColors[i] = rgb
			d.mu.Unlock()
			d.publish(Event{Kind: EventTouchKey, Index: i, Value: rgb})
			return
		}
	}
	log.Printf("deck: %s: set feature %s", d.model.ID, hexPrefix(data))
}

// packRGB returns a colour as 0xRRGGBB.
func packRGB(r, g, b byte) int {
	return int(r)<<16 | int(g)<<8 | int(b)
}
