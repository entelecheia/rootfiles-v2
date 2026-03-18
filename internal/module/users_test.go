package module

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
)

func TestListUserNames(t *testing.T) {
	// Create temp dir with a users.json
	tmpDir := t.TempDir()
	metaDir := filepath.Join(tmpDir, ".rootfiles")
	os.MkdirAll(metaDir, 0755)

	db := UsersDB{
		Version:  1,
		HomeBase: tmpDir,
		Users: []UserMeta{
			{Name: "alice", UID: 1001, Home: filepath.Join(tmpDir, "alice")},
			{Name: "bob", UID: 1002, Home: filepath.Join(tmpDir, "bob")},
			{Name: "charlie", UID: 1003, Home: filepath.Join(tmpDir, "charlie")},
		},
	}
	data, _ := json.MarshalIndent(db, "", "  ")
	os.WriteFile(filepath.Join(metaDir, "users.json"), data, 0600)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := exec.NewRunner(true, logger)
	rc := &RunContext{
		Config: &config.Config{
			Users: config.UsersConfig{HomeBase: tmpDir},
		},
		Runner: runner,
		DryRun: true,
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := ListUserNames(rc)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "alice\nbob\ncharlie\n" {
		t.Errorf("unexpected output:\n%s\nwant: alice\\nbob\\ncharlie\\n", output)
	}
}

func TestListUserNames_NoDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := exec.NewRunner(true, logger)
	rc := &RunContext{
		Config: &config.Config{
			Users: config.UsersConfig{HomeBase: tmpDir},
		},
		Runner: runner,
		DryRun: true,
	}

	err := ListUserNames(rc)
	if err != nil {
		t.Fatalf("should return nil when no database exists, got: %v", err)
	}
}
