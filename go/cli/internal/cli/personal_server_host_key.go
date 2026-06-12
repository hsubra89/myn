package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const personalServerHostKeyComment = "myn-personal-server-host"

type personalServerHostKey struct {
	PrivateKey string
	PublicKey  string
}

// generatePersonalServerHostKey creates the server's Ed25519 SSH host keypair
// locally so the host key is known before the first connection. The private
// key is delivered to the server through cloud-init and the public key is
// pinned in the myn known_hosts file.
func generatePersonalServerHostKey() (personalServerHostKey, error) {
	dir, err := os.MkdirTemp("", "myn-host-key-")
	if err != nil {
		return personalServerHostKey{}, fmt.Errorf("create temporary SSH host key directory: %w", err)
	}
	defer os.RemoveAll(dir)

	keyPath := filepath.Join(dir, "ssh_host_ed25519_key")
	output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", personalServerHostKeyComment, "-f", keyPath).CombinedOutput()
	if err != nil {
		return personalServerHostKey{}, commandOutputError("ssh-keygen", output, err)
	}

	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return personalServerHostKey{}, fmt.Errorf("read generated SSH host key: %w", err)
	}
	publicLine, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return personalServerHostKey{}, fmt.Errorf("read generated SSH host public key: %w", err)
	}
	publicKey, err := parseSSHPublicKey(string(publicLine))
	if err != nil {
		return personalServerHostKey{}, fmt.Errorf("parse generated SSH host public key: %w", err)
	}

	return personalServerHostKey{
		PrivateKey: string(privateKey),
		PublicKey:  publicKey.Line(),
	}, nil
}

func personalServerKnownHostsPath(appConfigPath string) string {
	return filepath.Join(filepath.Dir(appConfigPath), "known_hosts")
}

// writePersonalServerKnownHosts replaces the myn-managed known_hosts file with
// entries for the given Personal Server addresses. The config supports a
// single Personal Server, so entries for any previous server are dropped.
func writePersonalServerKnownHosts(path string, hostPublicKey string, hosts ...string) error {
	var b strings.Builder
	publicKey := strings.TrimSpace(hostPublicKey)
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", host, publicKey)
	}
	if b.Len() == 0 {
		return fmt.Errorf("Personal Server has no addresses to pin in known_hosts")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create known_hosts directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write known_hosts: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure known_hosts: %w", err)
	}
	return nil
}
