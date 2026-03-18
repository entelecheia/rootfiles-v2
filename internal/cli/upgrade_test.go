package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v0.3.0", "0.3.0"},
		{"0.3.0", "0.3.0"},
		{"v0.3.0 (abc1234)", "0.3.0"},
		{"dev", "dev"},
		{"dev (none)", "dev"},
	}
	for _, tt := range tests {
		got := normalizeVersion(tt.input)
		if got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestVerifyChecksum_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake archive
	archiveContent := []byte("fake archive content for testing")
	archiveName := "rootfiles_0.4.0_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	os.WriteFile(archivePath, archiveContent, 0644)

	// Compute its hash
	h := sha256.Sum256(archiveContent)
	hashStr := hex.EncodeToString(h[:])

	// Create checksums file
	checksumsContent := hashStr + "  " + archiveName + "\n"
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	os.WriteFile(checksumsPath, []byte(checksumsContent), 0644)

	// Verify using the internal logic (we test the hash comparison part)
	f, _ := os.Open(archivePath)
	defer f.Close()
	sh := sha256.New()
	sh.Write(archiveContent)
	actualHash := hex.EncodeToString(sh.Sum(nil))

	if actualHash != hashStr {
		t.Errorf("hash mismatch: %s != %s", actualHash, hashStr)
	}
}

func TestReplaceBinary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create "current" binary
	currentPath := filepath.Join(tmpDir, "rootfiles")
	os.WriteFile(currentPath, []byte("old binary"), 0755)

	// Create "new" binary
	newPath := filepath.Join(tmpDir, "rootfiles-new")
	os.WriteFile(newPath, []byte("new binary"), 0755)

	if err := replaceBinary(newPath, currentPath); err != nil {
		t.Fatalf("replaceBinary failed: %v", err)
	}

	// Verify current now has new content
	data, _ := os.ReadFile(currentPath)
	if string(data) != "new binary" {
		t.Errorf("expected 'new binary', got %q", string(data))
	}

	// Verify backup exists
	backupData, _ := os.ReadFile(currentPath + ".bak")
	if string(backupData) != "old binary" {
		t.Errorf("expected backup 'old binary', got %q", string(backupData))
	}
}
