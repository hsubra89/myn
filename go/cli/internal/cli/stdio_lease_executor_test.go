package cli

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestStoppableStdioActivityWriterStopWaitsForInFlightActivity(t *testing.T) {
	recordStarted := make(chan struct{})
	releaseRecord := make(chan struct{})

	var once sync.Once
	records := 0
	writer := newStoppableStdioActivityWriter(&bytes.Buffer{}, func(time.Time) error {
		once.Do(func() {
			close(recordStarted)
		})
		<-releaseRecord
		records++
		return nil
	}, func() time.Time {
		return time.Unix(0, 0).UTC()
	})

	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("input"))
		writeDone <- err
	}()

	<-recordStarted
	stopDone := make(chan struct{})
	go func() {
		writer.stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		t.Fatal("stop returned before in-flight activity finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseRecord)
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stop")
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("write input: %v", err)
	}

	if _, err := writer.Write([]byte("after-stop")); err == nil {
		t.Fatal("write after stop should fail")
	}
	if records != 1 {
		t.Fatalf("record count mismatch: want 1, got %d", records)
	}
}
