package device

import "testing"

func TestCyclopsIdentity(t *testing.T) {
	d, ok := ParseInterface(`\\?\hid#vid_1f3a&pid_b128#b&1d5ec18d&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`)
	if !ok || d.Kind != "cyclops" || d.Serial != "RJK-B_1D5EC18D_0_0000" {
		t.Fatalf("native HID Cyclops not recognized: %+v %v", d, ok)
	}
	for _, path := range []string{
		`\\?\usb#vid_1f3a&pid_b128#9&123abc&0&2#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`,
		`\\?\hid#vid_1f3a&pid_efe8#123#{guid}`,
		`\\?\hid#vid_0416&pid_5408#123#{guid}`,
	} {
		if _, ok := ParseInterface(path); ok {
			t.Fatal("unsupported transport or bootloader selected", path)
		}
	}
}

func TestParseInterfaceAllowsOnlySupportedHardware(t *testing.T) {
	for _, tc := range []struct {
		path, serial, kind string
		ok                 bool
	}{
		{`\\?\usb#vid_1cbe&pid_0035#0123456789abcdef#{88bae032-5a81-49f0-bc3d-a4ff138216d6}`, "0123456789ABCDEF", "pump", true},
		{`\\?\usb#vid_43a8&pid_0e61#aabbccddee#{12345678-1234-1344-1234-123456789abc}`, "AABBCCDDEE", "fan", true},
		{`\\?\usb#vid_1cbe&pid_0088#other#{guid}`, "", "", false},
		{"garbage", "", "", false},
	} {
		d, ok := ParseInterface(tc.path)
		if ok != tc.ok || d.Serial != tc.serial || d.Kind != tc.kind {
			t.Fatalf("ParseInterface(%q) = %+v, %v", tc.path, d, ok)
		}
	}
}

func TestSelectRequiresUniqueExplicitSerial(t *testing.T) {
	devices := []Device{{Serial: "AAA", Kind: "pump"}, {Serial: "BBB", Kind: "fan"}}
	for _, serial := range []string{"", "A", "missing"} {
		if _, err := Select(devices, serial); err == nil {
			t.Errorf("accepted invalid serial %q", serial)
		}
	}
	d, err := Select(devices, "bbb")
	if err != nil || d.Serial != "BBB" {
		t.Fatalf("case-insensitive selection: %+v %v", d, err)
	}
	if _, err := Select(append(devices, devices[1]), "BBB"); err == nil {
		t.Fatal("accepted ambiguous serial")
	}
}
