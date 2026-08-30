package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gkmz/opspilot/internal/llm"
)

func TestStoreSavesAndLoadsRecord(t *testing.T) {
	directory := t.TempDir()
	store := NewStore(directory)
	record := Record{
		Version:   currentRecordVersion,
		ID:        "session-001",
		CreatedAt: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 30, 12, 1, 0, 0, time.UTC),
		Messages:  []llm.Message{{Role: "user", Content: "问题"}},
	}

	if err := store.Save(record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.ID != record.ID || len(loaded.Messages) != 1 || loaded.Messages[0].Content != "问题" {
		t.Fatalf("loaded record = %+v, want %+v", loaded, record)
	}
}

func TestStoreUsesRestrictedFilePermissions(t *testing.T) {
	directory := t.TempDir()
	store := NewStore(directory)
	if err := store.Save(Record{Version: currentRecordVersion, ID: "session-001"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(directory, "session-001.json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permissions = %o, want 600", got)
	}
}

func TestStoreRejectsUnsafeSessionID(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, id := range []string{"", "../escape", "a/b"} {
		if err := store.Save(Record{ID: id}); err == nil {
			t.Fatalf("Save(%q) expected an error", id)
		}
	}
}
