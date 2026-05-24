package cli

import (
	"reflect"
	"testing"
)

func TestPersonalServerSSHCommandArgsPassLoginAndHostSeparately(t *testing.T) {
	got := personalServerSSHCommandArgs(
		"/home/harish/.ssh/id_ed25519",
		" harish ",
		" 2001:db8::55 ",
		"-o", "BatchMode=yes",
	)
	want := []string{
		"ssh",
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-i", "/home/harish/.ssh/id_ed25519",
		"-l", "harish",
		"2001:db8::55",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SSH command args mismatch: want %#v, got %#v", want, got)
	}
}
