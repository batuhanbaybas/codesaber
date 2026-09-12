package terminal

import (
	"os"
	"strings"
	"testing"
	"time"
)

func testOpts(t *testing.T) SessionOpts {
	t.Helper()
	dir := t.TempDir()
	return SessionOpts{
		ID:    "test-session",
		Cwd:   dir,
		Shell: "/bin/sh",
	}
}

func hasPty(t *testing.T) {
	if _, err := os.Stat("/dev/ptmx"); err != nil {
		t.Skipf("pty not available: %v", err)
	}
}

// readUntil accumulates Data events until substr appears or deadline hits.
func readUntil(t *testing.T, data <-chan Event, substr string, deadline time.Duration) string {
	t.Helper()
	var acc []byte
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-data:
			if !ok {
				t.Fatalf("stream closed before marker %q", substr)
			}
			if ev.Exit {
				t.Fatalf("exited before marker %q", substr)
			}
			acc = append(acc, ev.Data...)
			if strings.Contains(string(acc), substr) {
				return string(acc)
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q; got %q", substr, string(acc))
		}
	}
}

func TestSessionEchoRoundtrip(t *testing.T) {
	hasPty(t)
	s, err := New(testOpts(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	data := s.Data()
	marker := "aide-pty-9"
	if err := s.Input([]byte("echo " + marker + "\r")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, data, marker, 5*time.Second)
}

func TestCloseIdempotentAndExit(t *testing.T) {
	hasPty(t)
	s, err := New(testOpts(t))
	if err != nil {
		t.Fatal(err)
	}
	data := s.Data()
	// Drain output in the background so forwards never block Close.
	go func() {
		for range data {
		}
	}()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Second close must be safe (no panic, no race).
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestResizeNoPanic(t *testing.T) {
	hasPty(t)
	s, err := New(testOpts(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Resize(40, 120); err != nil {
		t.Fatal(err)
	}
	if err := s.Resize(0, 0); err == nil {
		t.Fatal("expected error for invalid size")
	}
}
