package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	repoOwner = "entelecheia"
	repoName  = "rootfiles-v2"
	githubAPI = "https://api.github.com"
)

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

func newUpgradeCmd(currentVersion string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Self-upgrade rootfiles to the latest (or specified) version",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			targetVersion, _ := cmd.Flags().GetString("version")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			// Resolve target version
			if targetVersion == "" {
				fmt.Print("Checking latest version... ")
				latest, err := fetchLatestVersion(ctx)
				if err != nil {
					return fmt.Errorf("fetching latest version: %w", err)
				}
				targetVersion = latest
				fmt.Println(targetVersion)
			}

			current := normalizeVersion(currentVersion)
			target := normalizeVersion(targetVersion)

			if current == target {
				fmt.Printf("Already at version %s, nothing to do.\n", targetVersion)
				return nil
			}
			fmt.Printf("Current: %s → Target: %s\n", currentVersion, targetVersion)

			if checkOnly {
				fmt.Println("Upgrade available. Run `rootfiles upgrade` to install.")
				return nil
			}

			osName := runtime.GOOS
			arch := runtime.GOARCH
			if osName != "linux" {
				return fmt.Errorf("self-upgrade only supported on linux (current: %s)", osName)
			}

			// Download
			tmpDir, err := os.MkdirTemp("", "rootfiles-upgrade-*")
			if err != nil {
				return fmt.Errorf("creating temp dir: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			fmt.Print("Downloading... ")
			archivePath, err := downloadRelease(ctx, targetVersion, osName, arch, tmpDir)
			if err != nil {
				return fmt.Errorf("download failed: %w", err)
			}
			fmt.Println("OK")

			// Verify checksum
			fmt.Print("Verifying checksum... ")
			if err := verifyChecksum(ctx, archivePath, targetVersion); err != nil {
				return fmt.Errorf("checksum verification failed: %w", err)
			}
			fmt.Println("OK")

			// Extract
			fmt.Print("Extracting... ")
			newBinary, err := extractBinary(archivePath, tmpDir)
			if err != nil {
				return fmt.Errorf("extraction failed: %w", err)
			}
			fmt.Println("OK")

			if dryRun {
				fmt.Printf("Dry-run: would replace binary with %s\n", newBinary)
				return nil
			}

			// Replace
			currentBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolving current binary: %w", err)
			}
			currentBinary, err = filepath.EvalSymlinks(currentBinary)
			if err != nil {
				return fmt.Errorf("resolving symlinks: %w", err)
			}

			fmt.Printf("Replacing %s... ", currentBinary)
			if err := replaceBinary(newBinary, currentBinary); err != nil {
				return fmt.Errorf("replace failed: %w", err)
			}
			fmt.Println("OK")

			fmt.Printf("Upgraded to %s\n", targetVersion)
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "Only check if an upgrade is available")
	cmd.Flags().String("version", "", "Upgrade to a specific version (e.g., v0.3.0)")
	return cmd
}

// fetchLatestVersion queries GitHub API for the latest release tag.
func fetchLatestVersion(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPI, repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var info releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if info.TagName == "" {
		return "", fmt.Errorf("no tag_name in release response")
	}
	return info.TagName, nil
}

// downloadRelease downloads the release archive to destDir.
func downloadRelease(ctx context.Context, version, osName, arch, destDir string) (string, error) {
	ver := strings.TrimPrefix(version, "v")
	filename := fmt.Sprintf("rootfiles_%s_%s_%s.tar.gz", ver, osName, arch)
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		repoOwner, repoName, version, filename)

	outPath := filepath.Join(destDir, filename)
	return outPath, downloadFile(ctx, url, outPath)
}

// verifyChecksum downloads checksums.txt and verifies the archive's SHA256.
func verifyChecksum(ctx context.Context, archivePath, version string) error {
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/checksums.txt",
		repoOwner, repoName, version)

	checksumData, err := fetchBody(ctx, url)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	// Compute actual hash
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Find expected hash
	archiveName := filepath.Base(archivePath)
	for _, line := range strings.Split(string(checksumData), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == archiveName {
			if parts[0] == actualHash {
				return nil
			}
			return fmt.Errorf("hash mismatch: expected %s, got %s", parts[0], actualHash)
		}
	}
	return fmt.Errorf("no checksum found for %s", archiveName)
}

// extractBinary extracts the rootfiles binary from tar.gz.
func extractBinary(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == "rootfiles" {
			outPath := filepath.Join(destDir, "rootfiles")
			out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return outPath, nil
		}
	}
	return "", fmt.Errorf("rootfiles binary not found in archive")
}

// replaceBinary backs up the current binary and replaces it with the new one.
func replaceBinary(newPath, currentPath string) error {
	backupPath := currentPath + ".bak"

	// Remove old backup if exists
	os.Remove(backupPath)

	// Backup current
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Copy new binary (cross-device rename safe)
	src, err := os.Open(newPath)
	if err != nil {
		// Restore backup on failure
		os.Rename(backupPath, currentPath)
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		os.Rename(backupPath, currentPath)
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Rename(backupPath, currentPath)
		return err
	}

	return nil
}

// normalizeVersion strips "v" prefix and any parenthetical suffix for comparison.
func normalizeVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, " "); idx != -1 {
		v = v[:idx]
	}
	return v
}

// downloadFile downloads a URL to a local path.
func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// fetchBody downloads a URL and returns the body as bytes.
func fetchBody(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
