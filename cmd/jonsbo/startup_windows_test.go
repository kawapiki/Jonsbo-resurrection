package main

import (
	"io"
	"os"
	"os/user"
	"reflect"
	"strings"
	"testing"
)

func TestStartupRejectsDifferentUACUserBeforeMutation(t *testing.T) {
	if _, err := startupCaller([]string{"enable", "--elevated", "--expected-user-sid", "S-1-5-21-123"}, "S-1-5-21-456"); err == nil {
		t.Fatal("accepted different admin identity")
	}
	want := []string{"enable", "--elevated"}
	got, err := startupCaller(append(append([]string{}, want...), "--expected-user-sid", "S-1-5-21-123"), "S-1-5-21-123")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestStartupIdentityPrintsCurrentSID(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	old := os.Stdout
	os.Stdout = w
	err = startup([]string{"identity"})
	os.Stdout = old
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != u.Uid {
		t.Fatalf("identity = %q; want %q", got, u.Uid)
	}
}

func TestInstallerStartupCommandsRejectUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{{"identity", "extra"}, {"disable-owned", "extra"}, {"disable-owned", "--expected-user-sid", "different-user"}, {"identity", "--expected-user-sid", "different-user"}} {
		if err := startup(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
