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

func TestDisableOwnedPreservesOtherCopies(t *testing.T) {
	const exe = `C:\Installed\jonsbo.exe`
	ownedRun, _ := runCommand(exe)
	otherRun, _ := runCommand(`C:\Portable\jonsbo.exe`)
	ownedTask := taskXML(exe, "S-1-5-21-42")
	otherTask := taskXML(`C:\Portable\jonsbo.exe`, "S-1-5-21-42")
	for _, tc := range []struct {
		name      string
		old, want snapshot
	}{
		{"normal", snapshot{Run: ownedRun}, snapshot{}},
		{"elevated", snapshot{Task: ownedTask}, snapshot{}},
		{"both owned", snapshot{Run: ownedRun, Task: ownedTask}, snapshot{}},
		{"other normal", snapshot{Run: otherRun}, snapshot{Run: otherRun}},
		{"other elevated", snapshot{Task: otherTask}, snapshot{Task: otherTask}},
		{"different task arguments", snapshot{Task: strings.Replace(ownedTask, "<Arguments>tray</Arguments>", "<Arguments>other</Arguments>", 1)}, snapshot{Task: strings.Replace(ownedTask, "<Arguments>tray</Arguments>", "<Arguments>other</Arguments>", 1)}},
		{"owned run other task", snapshot{Run: ownedRun, Task: otherTask}, snapshot{Task: otherTask}},
		{"other run owned task", snapshot{Run: otherRun, Task: ownedTask}, snapshot{Run: otherRun}},
		{"case insensitive", snapshot{Run: strings.ToUpper(ownedRun), Task: taskXML(strings.ToUpper(exe), "S-1-5-21-42")}, snapshot{}},
		{"disabled", snapshot{}, snapshot{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeBackend{value: tc.old}
			if err := disableOwned(fake, exe); err != nil {
				t.Fatal(err)
			}
			if fake.value != tc.want {
				t.Fatalf("got %+v; want %+v", fake.value, tc.want)
			}
			for _, kind := range fake.writes {
				if kind == "task" && tc.old.Task == tc.want.Task {
					t.Fatal("rewrote an unchanged task")
				}
				if kind == "run" && tc.old.Run == tc.want.Run {
					t.Fatal("rewrote an unchanged Run entry")
				}
			}
		})
	}
}

func TestDisableOwnedRollsBackOnTaskFailure(t *testing.T) {
	const exe = `C:\Installed\jonsbo.exe`
	run, _ := runCommand(exe)
	old := snapshot{Run: run, Task: taskXML(exe, "S-1-5-21-42")}
	fake := &fakeBackend{value: old, fail: "task", writeErr: ErrElevationRequired}
	if err := disableOwned(fake, exe); !errors.Is(err, ErrElevationRequired) {
		t.Fatalf("expected elevation error for installer retry; got %v", err)
	}
	if fake.value != old {
		t.Fatalf("rollback lost startup: %+v", fake.value)
	}
}

func TestDisableOwnedAcceptsTaskSchedulerUnicodeDeclaration(t *testing.T) {
	const exe = `C:\Árvíztűrő\jonsbo.exe`
	// Task Scheduler returns a Unicode COM string. JSON has already decoded
	// that string to UTF-8, while the original XML declaration remains intact.
	task := strings.Replace(taskXML(exe, "S-1-5-21-42"), `<?xml version="1.0"?>`, `<?xml version="1.0" encoding="UTF-16"?>`, 1)
	fake := &fakeBackend{value: snapshot{Task: task}}
	if err := disableOwned(fake, exe); err != nil {
		t.Fatal(err)
	}
	if fake.value != (snapshot{}) {
		t.Fatalf("owned task was not removed: %+v", fake.value)
	}
}

func TestDisableOwnedRejectsUnknownTaskEncodingBeforeMutation(t *testing.T) {
	const exe = `C:\Installed\jonsbo.exe`
	task := strings.Replace(taskXML(exe, "S-1-5-21-42"), `<?xml version="1.0"?>`, `<?xml version="1.0" encoding="unknown-encoding"?>`, 1)
	old := snapshot{Run: `"C:\Installed\jonsbo.exe" tray`, Task: task}
	fake := &fakeBackend{value: old}
	if err := disableOwned(fake, exe); err == nil {
		t.Fatal("accepted unknown task encoding")
	}
	if fake.value != old || len(fake.writes) != 0 {
		t.Fatal("mutated startup with unknown task encoding")
	}
}

func TestDisableOwnedReadFailureDoesNotMutate(t *testing.T) {
	wantErr := errors.New("ownership check failed")
	fake := &fakeBackend{value: snapshot{Run: `"C:\Installed\jonsbo.exe" tray`}, readErr: wantErr}
	if err := disableOwned(fake, `C:\Installed\jonsbo.exe`); !errors.Is(err, wantErr) {
		t.Fatalf("got %v; want ownership error", err)
	}
	if len(fake.writes) != 0 {
		t.Fatal("mutated after failed ownership read")
	}
}

func TestDisableOwnedInvalidPathDoesNotMutate(t *testing.T) {
	fake := &fakeBackend{value: snapshot{Run: `"C:\Installed\jonsbo.exe" tray`}}
	if err := disableOwned(fake, "jonsbo.exe"); err == nil {
		t.Fatal("accepted relative path")
	}
	if len(fake.writes) != 0 {
		t.Fatal("mutated with invalid executable")
	}
}

func TestDisableOwnedRejectsInvalidTaskBeforeMutation(t *testing.T) {
	const exe = `C:\Installed\jonsbo.exe`
	run, _ := runCommand(exe)
	old := snapshot{Run: run, Task: "invalid XML"}
	fake := &fakeBackend{value: old}
	if err := disableOwned(fake, exe); err == nil {
		t.Fatal("accepted malformed task")
	}
	if fake.value != old || len(fake.writes) != 0 {
		t.Fatal("mutated before validating task")
	}
}

type fakeBackend struct {
	value    snapshot
	fail     string
	writes   []string
	readErr  error
	writeErr error
}

func (f *fakeBackend) read() (snapshot, error) { return f.value, f.readErr }
func (f *fakeBackend) write(kind, value string) error {
	f.writes = append(f.writes, kind)
	if kind == f.fail {
		f.fail = ""
		if f.writeErr != nil {
			return f.writeErr
		}
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
