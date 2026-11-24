package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenKeepsAtMostOneIdleConn(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	if n := s.db.Stats().MaxOpenConnections; n != 4 {
		t.Fatalf("MaxOpenConnections = %d", n)
	}
}
