package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
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
	resp, err := http.Get(versionURL)
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
	// Determine binary name based on platform
	osName := runtime.GOOS
	archName := runtime.GOARCH
	binaryName := fmt.Sprintf("openexec-%s-%s", osName, archName)
	if osName == "windows" {
		binaryName += ".exe"
	}

	downloadURL := fmt.Sprintf("%s/%s", downloadBaseURL, binaryName)
	fmt.Printf("📥 Downloading v%s from %s...\n", targetVersion, downloadURL)

	// Create temp file for download
	tmpFile, err := os.CreateTemp("", "openexec-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Download
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d during download", resp.StatusCode)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save download: %w", err)
	}
	tmpFile.Close()

	// Set executable permissions
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

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
		if err := os.Rename(tmpFile.Name(), selfPath); err != nil {
			// Try to restore
			_ = os.Rename(oldPath, selfPath)
			return fmt.Errorf("failed to install new binary on Windows: %w", err)
		}
		// Try to delete old path (might fail if still in use, that's okay)
		_ = os.Remove(oldPath)
	} else {
		if err := os.Rename(tmpFile.Name(), selfPath); err != nil {
			// If rename fails (e.g. cross-device), try copy
			if err := copyFile(tmpFile.Name(), selfPath); err != nil {
				return fmt.Errorf("failed to install new binary: %w", err)
			}
		}
	}

	fmt.Printf("✅ OpenExec updated to v%s successfully!\n", targetVersion)
	return nil
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
