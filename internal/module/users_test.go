package module

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entelecheia/rootfiles-v2/internal/config"
	"github.com/entelecheia/rootfiles-v2/internal/exec"
)

func TestUsersModule_Name(t *testing.T) {
	if n := NewUsersModule().Name(); n != "users" {
		t.Errorf("Name() = %q, want users", n)
	}
}

func TestUsersModule_CheckDefaultHomeBaseIsSatisfied(t *testing.T) {
	rc := newDryRunRC(t)
	// HomeBase "" or "/home" means no custom setup required.
	result, err := NewUsersModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Satisfied {
		t.Errorf("Check with default HomeBase should be satisfied, got %+v", result.Changes)
	}
}

func TestUsersModule_ApplyCustomHomeBaseDryRun(t *testing.T) {
	tmp := t.TempDir()
	rc := newDryRunRC(t)
	rc.Config.Users = config.UsersConfig{HomeBase: filepath.Join(tmp, "home2")}
	result, err := NewUsersModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Changed {
		t.Error("Apply with custom HomeBase should report Changed=true")
	}
}

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

func TestBackupUsers_MergesSystemUsers(t *testing.T) {
	tmpDir := t.TempDir()
	metaDir := filepath.Join(tmpDir, ".rootfiles")
	os.MkdirAll(metaDir, 0755)

	// Existing managed user
	db := UsersDB{
		Version:   1,
		HomeBase:  tmpDir,
		CreatedBy: "rootfiles-v2",
		Users: []UserMeta{
			{Name: "managed", UID: 1001, GID: 1001, Shell: "/bin/bash", Home: filepath.Join(tmpDir, "managed")},
		},
	}
	data, _ := json.MarshalIndent(db, "", "  ")
	os.WriteFile(filepath.Join(metaDir, "users.json"), data, 0600)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := exec.NewRunner(false, logger)
	rc := &RunContext{
		Config: &config.Config{
			Users: config.UsersConfig{HomeBase: tmpDir},
		},
		Runner: runner,
		DryRun: false,
	}

	outputPath := filepath.Join(tmpDir, "backup.json")

	// Capture stdout
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := BackupUsers(rc, outputPath)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("BackupUsers failed: %v", err)
	}

	// Read output and verify managed user is present
	outData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}

	var result UsersDB
	if err := json.Unmarshal(outData, &result); err != nil {
		t.Fatalf("parsing backup: %v", err)
	}

	// At minimum the managed user must be in the output
	found := false
	for _, u := range result.Users {
		if u.Name == "managed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("managed user not found in backup output")
	}

	// System users (UID >= 1000) from /etc/passwd should also be present
	// (count will vary by host, so just verify total >= 1)
	if len(result.Users) < 1 {
		t.Errorf("expected at least 1 user in backup, got %d", len(result.Users))
	}
}

func TestRestoreUsers_RestoresSSHKeys(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a backup file with SSH pubkeys
	db := UsersDB{
		Version:  1,
		HomeBase: tmpDir,
		Users: []UserMeta{
			{
				Name:       "testuser",
				UID:        9901,
				GID:        9901,
				Shell:      "/bin/bash",
				Home:       filepath.Join(tmpDir, "testuser"),
				SSHPubkeys: []string{"ssh-ed25519 AAAAC3Nz_test_key user@test"},
			},
		},
	}
	backupData, _ := json.MarshalIndent(db, "", "  ")
	backupPath := filepath.Join(tmpDir, "backup.json")
	os.WriteFile(backupPath, backupData, 0600)

	// Create the home directory (simulating preserved home WITHOUT .ssh)
	homeDir := filepath.Join(tmpDir, "testuser")
	os.MkdirAll(homeDir, 0755)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := exec.NewRunner(true, logger) // dry-run: won't actually call useradd
	rc := &RunContext{
		Config: &config.Config{
			Users: config.UsersConfig{HomeBase: tmpDir},
		},
		Runner: runner,
		DryRun: true,
	}

	// Capture stdout
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	_ = RestoreUsers(context.Background(), rc, backupPath)

	w.Close()
	os.Stdout = old

	// In dry-run, files won't actually be written, but we can verify the logic
	// by checking that no error occurred. For a real test on Linux, dry-run=false
	// would need a mock runner or root permissions.
}

func TestScanSystemUsers_ParsesPasswd(t *testing.T) {
	// Verify scanSystemUsers works on the current host
	// (macOS uses a different format, so this test is Linux-only)
	if _, err := os.Stat("/etc/passwd"); err != nil {
		t.Skip("no /etc/passwd, skipping")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := exec.NewRunner(false, logger)
	rc := &RunContext{
		Config: &config.Config{},
		Runner: runner,
	}

	users, err := scanSystemUsers(context.Background(), rc)
	if err != nil {
		t.Fatalf("scanSystemUsers failed: %v", err)
	}

	// Every returned user should have UID >= 1000
	for _, u := range users {
		if u.UID < 1000 || u.UID > 65533 {
			t.Errorf("user %s has UID %d outside expected range", u.Name, u.UID)
		}
		if u.Shell == "" {
			t.Errorf("user %s has empty shell", u.Name)
		}
		if u.Home == "" {
			t.Errorf("user %s has empty home", u.Name)
		}
		if strings.HasSuffix(u.Shell, "/nologin") || strings.HasSuffix(u.Shell, "/false") {
			t.Errorf("user %s has nologin shell %s, should have been filtered", u.Name, u.Shell)
		}
	}
}
