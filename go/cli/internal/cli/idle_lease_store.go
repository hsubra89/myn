package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const stdioLeaseKind = "stdio"

type idleLeaseFileStore struct {
	dir string
}

func newIdleLeaseFileStore(env func(string) string) (idleLeaseFileStore, error) {
	leaseDir, err := resolveIdleLeaseDirectory(env)
	if err != nil {
		return idleLeaseFileStore{}, err
	}
	if err := ensureIdleLeaseDirectory(leaseDir); err != nil {
		return idleLeaseFileStore{}, err
	}
	return idleLeaseFileStore{dir: leaseDir}, nil
}

func (store idleLeaseFileStore) write(lease idleLease) error {
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return fmt.Errorf("encode idle lease %s: %w", lease.ID, err)
	}
	data = append(data, '\n')

	path := store.path(lease.ID)
	tmp, err := os.CreateTemp(store.dir, "."+lease.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create idle lease temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write idle lease temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set idle lease file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close idle lease temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish idle lease file: %w", err)
	}
	return nil
}

func (store idleLeaseFileStore) remove(id string) error {
	err := os.Remove(store.path(id))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove idle lease file: %w", err)
}

func (store idleLeaseFileStore) path(id string) string {
	return filepath.Join(store.dir, id+".json")
}

type stdioLeaseSession struct {
	store   idleLeaseFileStore
	lease   idleLease
	mu      sync.Mutex
	writeMu sync.Mutex
	stop    chan struct{}
	done    chan error
}

func newStdioLeaseSession(req stdioRunRequest) (*stdioLeaseSession, error) {
	store, err := newIdleLeaseFileStore(os.Getenv)
	if err != nil {
		return nil, err
	}
	id, err := generateIdleLeaseID()
	if err != nil {
		return nil, err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory for stdio lease: %w", err)
	}
	idleAfter := req.IdleAfter
	if idleAfter <= 0 {
		idleAfter = defaultStdioIdleAfter
	}

	return &stdioLeaseSession{
		store: store,
		lease: idleLease{
			Kind:             stdioLeaseKind,
			ID:               id,
			User:             currentIdleLeaseUser(),
			WorkingDirectory: workingDirectory,
			Command:          req.Command[0],
			Interactive:      true,
			IdleAfter:        idleLeaseDuration(idleAfter),
		},
	}, nil
}

func (session *stdioLeaseSession) start(rootPID int, processGroup int) error {
	now := time.Now().UTC()
	session.lease.RootPID = rootPID
	session.lease.ProcessGroup = processGroup
	session.lease.StartedAt = now
	if err := session.flush(now); err != nil {
		return err
	}

	session.stop = make(chan struct{})
	session.done = make(chan error, 1)
	go session.runHeartbeat(stdioLeaseHeartbeatInterval(time.Duration(session.lease.IdleAfter)))
	return nil
}

func (session *stdioLeaseSession) close() error {
	var heartbeatErr error
	if session.stop != nil {
		close(session.stop)
		heartbeatErr = <-session.done
	}

	removeErr := session.store.remove(session.lease.ID)
	if heartbeatErr != nil {
		return heartbeatErr
	}
	return removeErr
}

func (session *stdioLeaseSession) runHeartbeat(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := session.flush(time.Now().UTC()); err != nil {
				session.done <- err
				return
			}
		case <-session.stop:
			session.done <- nil
			return
		}
	}
}

func (session *stdioLeaseSession) recordInput(now time.Time) error {
	return session.recordTerminalActivity(now, true)
}

func (session *stdioLeaseSession) recordOutput(now time.Time) error {
	return session.recordTerminalActivity(now, false)
}

func (session *stdioLeaseSession) recordTerminalActivity(now time.Time, input bool) error {
	now = now.UTC()

	session.mu.Lock()
	previousActivity, hadPreviousActivity := latestTerminalActivity(session.lease)
	activityAt := now
	if input {
		session.lease.LastInputAt = &activityAt
	} else {
		session.lease.LastOutputAt = &activityAt
	}
	idleAfter := time.Duration(session.lease.IdleAfter)
	activityBecameActive := !hadPreviousActivity || previousActivity.Before(now.Add(-idleAfter))
	session.mu.Unlock()

	if activityBecameActive {
		return session.flush(time.Now().UTC())
	}
	return nil
}

func (session *stdioLeaseSession) flush(now time.Time) error {
	session.writeMu.Lock()
	defer session.writeMu.Unlock()

	session.mu.Lock()
	session.lease.UpdatedAt = now
	session.lease.ExpiresAt = stdioLeaseExpiresAt(now, time.Duration(session.lease.IdleAfter))
	lease := session.lease
	session.mu.Unlock()

	return session.store.write(lease)
}

func generateIdleLeaseID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate idle lease ID: %w", err)
	}
	return "stdio-" + hex.EncodeToString(random[:]), nil
}

func currentIdleLeaseUser() string {
	current, err := user.Current()
	if err == nil && current.Username != "" {
		return current.Username
	}
	if value := strings.TrimSpace(os.Getenv("USER")); value != "" {
		return value
	}
	return "unknown"
}

func stdioLeaseExpiresAt(now time.Time, idleAfter time.Duration) time.Time {
	return now.Add(idleHeartbeatStaleAfter(idleAfter) + time.Minute)
}

func stdioLeaseHeartbeatInterval(idleAfter time.Duration) time.Duration {
	if idleAfter <= 0 {
		return 15 * time.Second
	}
	interval := idleAfter / 3
	if interval < 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	if interval > 15*time.Second {
		return 15 * time.Second
	}
	return interval
}
