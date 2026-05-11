package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdleLeaseFileStorePublishesOnlyFinalJSONFile(t *testing.T) {
	leaseDir := t.TempDir()
	store, err := newIdleLeaseFileStore(func(key string) string {
		if key == "ME_LEASE_DIR" {
			return leaseDir
		}
		return ""
	})
	if err != nil {
		t.Fatalf("create lease store: %v", err)
	}

	now := time.Now().UTC()
	if err := store.write(idleLease{
		Kind:             stdioLeaseKind,
		ID:               "stdio-test",
		RootPID:          os.Getpid(),
		ProcessGroup:     os.Getpid(),
		User:             "harish",
		WorkingDirectory: "/home/harish/projects/me",
		Command:          "codex",
		Interactive:      true,
		StartedAt:        now,
		UpdatedAt:        now,
		IdleAfter:        idleLeaseDuration(30 * time.Minute),
		ExpiresAt:        now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("write lease: %v", err)
	}

	entries, err := os.ReadDir(leaseDir)
	if err != nil {
		t.Fatalf("read lease directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "stdio-test.json" {
		t.Fatalf("lease write should publish one final JSON file, got %#v", entries)
	}

	leasePath := filepath.Join(leaseDir, "stdio-test.json")
	info, err := os.Stat(leasePath)
	if err != nil {
		t.Fatalf("stat lease file: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Fatalf("lease file permissions mismatch: want %s, got %s", want, got)
	}

	data, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("read lease file: %v", err)
	}
	var lease idleLease
	if err := json.Unmarshal(data, &lease); err != nil {
		t.Fatalf("decode lease JSON %s: %v", data, err)
	}
	if lease.ID != "stdio-test" {
		t.Fatalf("lease ID mismatch: %q", lease.ID)
	}
}
