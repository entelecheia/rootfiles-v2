package exec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupPreserveAndRestore(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()
	existing := filepath.Join(work, "sshd.conf")
	created := filepath.Join(work, "new.conf")
	link := filepath.Join(work, "data")
	os.WriteFile(existing, []byte("original\n"), 0640)
	os.Symlink("/old/target", link)

	r := quietRunner(false)
	r.Backup = NewBackup(root, "test")
	if r.Backup.Used() {
		t.Fatal("fresh backup should be unused")
	}

	// Mutate: overwrite twice (only the first version is kept), create a
	// file, re-point a symlink.
	r.WriteFile(existing, []byte("changed-1\n"), 0644)
	r.WriteFile(existing, []byte("changed-2\n"), 0644)
	r.WriteFile(created, []byte("brand new\n"), 0644)
	r.Remove(link)
	r.Symlink("/new/target", link)

	if !r.Backup.Used() {
		t.Fatal("backup should be used")
	}
	list, err := ListBackups(root)
	if err != nil || len(list) != 1 || len(list[0].Entries) != 3 {
		t.Fatalf("ListBackups = %+v, %v", list, err)
	}

	preview, err := RestoreBackup(root, r.Backup.ID, true)
	if err != nil || len(preview) != 3 {
		t.Fatalf("preview = %v, %v", preview, err)
	}
	if data, _ := os.ReadFile(existing); string(data) != "changed-2\n" {
		t.Fatal("dry-run restore must not change files")
	}

	if _, err := RestoreBackup(root, r.Backup.ID, false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(existing); string(data) != "original\n" {
		t.Errorf("existing file = %q, want original", data)
	}
	if fi, _ := os.Stat(existing); fi.Mode().Perm() != 0640 {
		t.Errorf("mode = %v, want 0640", fi.Mode().Perm())
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Error("file created by the run should be removed")
	}
	if tgt, _ := os.Readlink(link); tgt != "/old/target" {
		t.Errorf("symlink target = %q, want /old/target", tgt)
	}
}

func TestRestoreBackupKeepsCreatedDirectories(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "home.rootfiles-bak")
	r := quietRunner(false)
	r.Backup = NewBackup(root, "test")
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0644)
	if err := r.Rename(src, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreBackup(root, r.Backup.ID, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f")); err != nil {
		t.Error("rollback must never delete a directory with data")
	}
}

func TestLoadBackupRejectsTraversal(t *testing.T) {
	if _, err := LoadBackup(t.TempDir(), "../etc"); err == nil {
		t.Error("path traversal in backup id must be rejected")
	}
}
