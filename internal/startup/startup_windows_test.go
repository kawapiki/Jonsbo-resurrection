package startup

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"
)

func TestTaskXMLScopesInteractiveUserAndEscapesPath(t *testing.T) {
	s := taskXML(`C:\A&B\jonsbo.exe`, "S-1-5-21-42")
	var v struct {
		Principals struct {
			Principal struct {
				UserID string `xml:"UserId"`
				Logon  string `xml:"LogonType"`
				Level  string `xml:"RunLevel"`
			}
		}
		Actions struct {
			Exec struct {
				Command   string
				Arguments string
			}
		}
		Triggers struct {
			LogonTrigger struct {
				UserID string `xml:"UserId"`
			}
		}
	}
	if err := xml.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	if v.Principals.Principal.UserID != "S-1-5-21-42" || v.Triggers.LogonTrigger.UserID != "S-1-5-21-42" || v.Principals.Principal.Logon != "InteractiveToken" || v.Principals.Principal.Level != "HighestAvailable" || v.Actions.Exec.Command != `C:\A&B\jonsbo.exe` || v.Actions.Exec.Arguments != "tray" {
		t.Fatalf("unsafe task: %+v", v)
	}
}

func TestRunCommandRejectsUnsafePaths(t *testing.T) {
	for _, p := range []string{"relative.exe", `C:\bad"name.exe`, "C:\\bad\nname.exe", `\\server\app.exe`} {
		if _, err := runCommand(p); err == nil {
			t.Errorf("accepted %q", p)
		}
	}
	got, err := runCommand(`C:\Program Files\O'Brien $stuff\jonsbo.exe`)
	if err != nil || got != `"C:\Program Files\O'Brien $stuff\jonsbo.exe" tray` {
		t.Fatalf("%q %v", got, err)
	}
}

func TestFailedRegistrationPreservesPreviousStartup(t *testing.T) {
	old := snapshot{Run: `"C:\old\jonsbo.exe" tray`}
	fake := &fakeBackend{value: old, fail: "task"}
	if err := change(fake, `C:\new\jonsbo.exe`, Elevated); err == nil {
		t.Fatal("expected error")
	}
	if fake.value != old {
		t.Fatalf("old startup changed: %+v", fake.value)
	}
}

func TestFailedCleanupRollsBackNewStartup(t *testing.T) {
	old := snapshot{Run: `"C:\old\jonsbo.exe" tray`}
	fake := &fakeBackend{value: old, fail: "run"}
	if err := change(fake, `C:\new\jonsbo.exe`, Elevated); err == nil {
		t.Fatal("expected error")
	}
	if fake.value != old {
		t.Fatalf("rollback failed: %+v", fake.value)
	}
}

func TestSwitchAndDisableDoNotLeaveDuplicateEntries(t *testing.T) {
	fake := &fakeBackend{value: snapshot{Task: "existing"}}
	if err := change(fake, `C:\new\jonsbo.exe`, Normal); err != nil {
		t.Fatal(err)
	}
	if fake.value.Task != "" || !strings.Contains(fake.value.Run, "new") {
		t.Fatal(fake.value)
	}
	if err := change(fake, "", Disabled); err != nil {
		t.Fatal(err)
	}
	if fake.value != (snapshot{}) {
		t.Fatal(fake.value)
	}
}

type fakeBackend struct {
	value snapshot
	fail  string
}

func (f *fakeBackend) read() (snapshot, error) { return f.value, nil }
func (f *fakeBackend) write(kind, value string) error {
	if kind == f.fail {
		f.fail = ""
		return errors.New("injected failure")
	}
	if kind == "task" {
		f.value.Task = value
	} else {
		f.value.Run = value
	}
	return nil
}
func (f *fakeBackend) sid() string { return "S-1-5-21-42" }
