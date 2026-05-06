package server

import (
	"embed"
	"testing"
)

func TestNewGuardianServer(t *testing.T) {
	// Create an empty embedded FS for testing
	var dummyFS embed.FS

	// Use an in-memory database for testing
	srv, err := NewGuardianServer(
		"8.8.8.8:53",
		"localhost:50051",
		"dummy_blocklist.txt",
		":memory:", // in-memory SQLite DB
		LogLevelDebug,
		dummyFS,
		"test-version",
	)

	if err != nil {
		t.Fatalf("Failed to create GuardianServer: %v", err)
	}

	if srv == nil {
		t.Fatal("Expected GuardianServer instance, got nil")
	}

	// Clean up
	if srv.db != nil {
		srv.db.Close()
	}
}
