package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testPersonalServerHostKey() personalServerHostKey {
	return personalServerHostKey{
		PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nhost-private-key-material\n-----END OPENSSH PRIVATE KEY-----\n",
		PublicKey:  "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHostKey myn-personal-server-host",
	}
}

func testGeneratePersonalServerHostKey() (personalServerHostKey, error) {
	return testPersonalServerHostKey(), nil
}

func TestGeneratePersonalServerHostKeyProducesEd25519Keypair(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen is not available")
	}

	hostKey, err := generatePersonalServerHostKey()
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	if !strings.Contains(hostKey.PrivateKey, "OPENSSH PRIVATE KEY") {
		t.Fatalf("private key should be in OpenSSH format, got %q", hostKey.PrivateKey)
	}
	publicKey, err := parseSSHPublicKey(hostKey.PublicKey)
	if err != nil {
		t.Fatalf("parse generated host public key: %v", err)
	}
	if publicKey.KeyType != sshKeyTypeEd25519 {
		t.Fatalf("host key type mismatch: want %q, got %q", sshKeyTypeEd25519, publicKey.KeyType)
	}
	if publicKey.Comment != personalServerHostKeyComment {
		t.Fatalf("host key comment mismatch: want %q, got %q", personalServerHostKeyComment, publicKey.Comment)
	}
}

func TestPersonalServerKnownHostsPathSitsBesideConfig(t *testing.T) {
	got := personalServerKnownHostsPath(filepath.Join("/", "home", "harish", ".config", "myn", "config.json"))
	want := filepath.Join("/", "home", "harish", ".config", "myn", "known_hosts")
	if got != want {
		t.Fatalf("known_hosts path mismatch: want %q, got %q", want, got)
	}
}

func TestWritePersonalServerKnownHostsPinsEveryAddress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myn", "known_hosts")
	hostKey := testPersonalServerHostKey()

	if err := writePersonalServerKnownHosts(path, hostKey.PublicKey, " 203.0.113.55 ", "", "2001:db8::55"); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	want := "203.0.113.55 " + hostKey.PublicKey + "\n2001:db8::55 " + hostKey.PublicKey + "\n"
	if got := string(data); got != want {
		t.Fatalf("known_hosts content mismatch: want %q, got %q", want, got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("known_hosts permissions mismatch: want %v, got %v", want, got)
	}
}

func TestWritePersonalServerKnownHostsReplacesPreviousServerEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	hostKey := testPersonalServerHostKey()

	if err := writePersonalServerKnownHosts(path, hostKey.PublicKey, "203.0.113.55"); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	if err := writePersonalServerKnownHosts(path, hostKey.PublicKey, "198.51.100.7"); err != nil {
		t.Fatalf("rewrite known_hosts: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if got, want := string(data), "198.51.100.7 "+hostKey.PublicKey+"\n"; got != want {
		t.Fatalf("known_hosts should only contain the current server: want %q, got %q", want, got)
	}
}

func TestWritePersonalServerKnownHostsRequiresAnAddress(t *testing.T) {
	err := writePersonalServerKnownHosts(filepath.Join(t.TempDir(), "known_hosts"), testPersonalServerHostKey().PublicKey, " ", "")
	if err == nil {
		t.Fatal("expected error when no addresses are available to pin")
	}
}

func TestPersonalServerProvisioningSSHArgsPinKnownHosts(t *testing.T) {
	got := personalServerProvisioningSSHArgs(
		filepath.Join("/", "home", "harish", ".config", "myn", "known_hosts"),
		"/home/harish/.ssh/id_ed25519",
		"harish",
		"203.0.113.55",
	)
	want := []string{
		"ssh",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", `UserKnownHostsFile="` + filepath.Join("/", "home", "harish", ".config", "myn", "known_hosts") + `"`,
		"-o", "ConnectTimeout=10",
		"-o", "IdentitiesOnly=yes",
		"-i", "/home/harish/.ssh/id_ed25519",
		"-l", "harish",
		"203.0.113.55",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provisioning SSH args mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestSSHUserKnownHostsOptionQuotesPathsWithSpaces(t *testing.T) {
	got := sshUserKnownHostsOption("/Users/harish/Library/Application Support/myn/known_hosts")
	want := `UserKnownHostsFile="/Users/harish/Library/Application Support/myn/known_hosts"`
	if got != want {
		t.Fatalf("known hosts option mismatch: want %q, got %q", want, got)
	}
}
