package module

import "testing"

func TestValidateDescriptor(t *testing.T) {
	d := Descriptor{ID: "hardware", Name: "Hardware", Version: "0.1.0", Views: []View{{ID: "cpu", Width: 640, Height: 180}}}
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Descriptor{{ID: "../bad", Name: "x", Version: "1"}, {ID: "ok", Name: "x", Version: "1", Views: []View{{ID: "x", Width: 0, Height: 1}}}, {ID: "ok", Name: "x", Version: "1", Views: []View{{ID: "x", Width: 1, Height: 1}, {ID: "x", Width: 1, Height: 1}}}} {
		if Validate(bad) == nil {
			t.Fatalf("accepted invalid descriptor %+v", bad)
		}
	}
}
