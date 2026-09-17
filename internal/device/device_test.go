package device

import "testing"

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
