package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/openexec/openexec/pkg/version"
	"github.com/spf13/cobra"
)

const (
	versionURL      = "https://openexec.io/version.txt"
	downloadBaseURL = "https://openexec.io/downloads"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update OpenExec to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("🔍 Checking for updates...\n")

		latestVersion, err := fetchLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		latestVersion = strings.TrimSpace(latestVersion)
		currentVersion := version.Version

		if latestVersion == currentVersion {
			fmt.Printf("✅ OpenExec is already up to date (v%s)\n", currentVersion)
			return nil
		}

		fmt.Printf("✨ A new version is available: v%s (current: v%s)\n", latestVersion, currentVersion)

		// Confirm update
		fmt.Print("Do you want to update? [Y/n]: ")
		var confirm string
		fmt.Scanln(&confirm)
		confirm = strings.ToLower(strings.TrimSpace(confirm))

		if confirm != "" && confirm != "y" && confirm != "yes" {
			fmt.Println("Update cancelled.")
			return nil
		}

		return performUpdate(latestVersion)
	},
}

func fetchLatestVersion() (string, error) {
	// Send our version so the server can gate old clients (which cannot
	// decompress archives) to a raw stepping-stone release. Static hosts
	// ignore the query param and serve version.txt unchanged.
	resp, err := http.Get(versionURL + "?from=" + url.QueryEscape(version.Version))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func performUpdate(targetVersion string) error {
	tmpPath, err := downloadBinary(targetVersion)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	// Get current executable path
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find current executable: %w", err)
	}

	// Replace binary
	fmt.Printf("📦 Replacing binary at %s...\n", selfPath)

	// On Unix we can rename over the running binary.
	// On Windows we might need to use a trick, but we'll try rename first.
	if runtime.GOOS == "windows" {
		// Windows specific: rename old to .old and move new to current
		oldPath := selfPath + ".old"
		_ = os.Remove(oldPath) // remove previous .old if exists
		if err := os.Rename(selfPath, oldPath); err != nil {
			return fmt.Errorf("failed to move current binary on Windows: %w", err)
		}
		if err := os.Rename(tmpPath, selfPath); err != nil {
			// Try to restore
			_ = os.Rename(oldPath, selfPath)
			return fmt.Errorf("failed to install new binary on Windows: %w", err)
		}
		// Try to delete old path (might fail if still in use, that's okay)
		_ = os.Remove(oldPath)
	} else {
		if err := os.Rename(tmpPath, selfPath); err != nil {
			// If rename fails (e.g. cross-device), try copy
			if err := copyFile(tmpPath, selfPath); err != nil {
				return fmt.Errorf("failed to install new binary: %w", err)
			}
		}
	}

	fmt.Printf("✅ OpenExec updated to v%s successfully!\n", targetVersion)
	return nil
}

// downloadBinary fetches the platform asset and returns the path to an
// extracted, executable temp file. It prefers the compressed archive
// (.tar.gz on unix, .zip on windows) and falls back to the legacy raw binary
// when the server only serves raw (404) — so a v0.11.0+ client keeps working
// against both archive-only and raw-only download hosts.
func downloadBinary(targetVersion string) (string, error) {
	osName := runtime.GOOS
	archName := runtime.GOARCH
	base := fmt.Sprintf("openexec-%s-%s", osName, archName)

	archiveName := base + ".tar.gz"
	if osName == "windows" {
		archiveName = base + ".zip"
	}
	archiveURL := fmt.Sprintf("%s/%s", downloadBaseURL, archiveName)
	fmt.Printf("📥 Downloading v%s from %s...\n", targetVersion, archiveURL)
	data, status, err := httpGetBytes(archiveURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	if status == http.StatusOK {
		return extractArchive(archiveName, data)
	}
	if status != http.StatusNotFound {
		return "", fmt.Errorf("server returned %d during download", status)
	}

	// Legacy raw fallback (host has no archive for this platform).
	rawName := base
	if osName == "windows" {
		rawName += ".exe"
	}
	rawURL := fmt.Sprintf("%s/%s", downloadBaseURL, rawName)
	fmt.Printf("📥 Archive not found; falling back to raw binary %s...\n", rawURL)
	data, status, err = httpGetBytes(rawURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("server returned %d during download", status)
	}
	return writeTempExecutable(data)
}

func httpGetBytes(u string) ([]byte, int, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// extractArchive pulls the single openexec binary out of a .tar.gz or .zip and
// writes it to an executable temp file.
func extractArchive(name string, data []byte) (string, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", fmt.Errorf("open zip: %w", err)
		}
		for _, f := range zr.File {
			if isOpenexecBinary(f.Name) {
				rc, err := f.Open()
				if err != nil {
					return "", fmt.Errorf("read zip entry: %w", err)
				}
				b, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return "", fmt.Errorf("read zip entry: %w", err)
				}
				return writeTempExecutable(b)
			}
		}
		return "", fmt.Errorf("no openexec binary in archive %s", name)
	}

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && isOpenexecBinary(hdr.Name) {
			b, err := io.ReadAll(tr)
			if err != nil {
				return "", fmt.Errorf("read tar entry: %w", err)
			}
			return writeTempExecutable(b)
		}
	}
	return "", fmt.Errorf("no openexec binary in archive %s", name)
}

// isOpenexecBinary matches the binary regardless of any directory prefix in the
// archive (openexec or openexec.exe).
func isOpenexecBinary(entry string) bool {
	b := path.Base(entry)
	return b == "openexec" || b == "openexec.exe"
}

func writeTempExecutable(data []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "openexec-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to save download: %w", err)
	}
	tmpFile.Close()
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to set permissions: %w", err)
	}
	return tmpFile.Name(), nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
