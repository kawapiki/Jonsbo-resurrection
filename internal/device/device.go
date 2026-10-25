// Package device identifies only the display controllers verified on this cooler.
package device

import (
	"fmt"
	"strings"
)

type Device struct {
	Serial string `json:"serial"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
}

func ParseInterface(path string) (Device, bool) {
	parts := strings.Split(strings.ToLower(path), "#")
	if len(parts) != 4 || (parts[0] != `\\?\usb` && parts[0] != `\\?\hid`) || parts[2] == "" {
		return Device{}, false
	}
	kind := ""
	switch parts[1] {
	case "vid_1cbe&pid_0035":
		kind = "pump"
	case "vid_43a8&pid_0e61":
		kind = "fan"
	case "vid_1f3a&pid_b128":
		kind = "cyclops"
	default:
		return Device{}, false
	}
	if (parts[0] == `\\?\hid`) != (kind == "cyclops") {
		return Device{}, false
	}
	serial := strings.ToUpper(parts[2])
	// Windows supplies a port identity when a controller has no USB serial.
	// Prefix it to avoid collisions with the other display families.
	if kind == "cyclops" {
		serial = "RJK-" + strings.ReplaceAll(serial, "&", "_")
	}
	return Device{Serial: serial, Kind: kind, Path: path}, true
}

func Select(devices []Device, serial string) (Device, error) {
	if serial == "" {
		return Device{}, fmt.Errorf("an explicit device serial is required; run list")
	}
	var found []Device
	for _, d := range devices {
		if strings.EqualFold(d.Serial, serial) {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		return Device{}, fmt.Errorf("serial %q matched %d devices; expected exactly one", serial, len(found))
	}
	return found[0], nil
}
