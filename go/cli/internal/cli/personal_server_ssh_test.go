package cli

import (
	"reflect"
	"testing"
)

func TestPersonalServerSSHCommandArgsPassLoginAndHostSeparately(t *testing.T) {
	got := personalServerSSHCommandArgs(
		" harish ",
		" harish-personal-server ",
		"-o", "BatchMode=yes",
	)
	want := []string{
		"ssh",
		"-o", "BatchMode=yes",
		"-l", "harish",
		"harish-personal-server",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SSH command args mismatch: want %#v, got %#v", want, got)
	}
}
