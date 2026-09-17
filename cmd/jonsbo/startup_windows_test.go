package main

import (
	"reflect"
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
