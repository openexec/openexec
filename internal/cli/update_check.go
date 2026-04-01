package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/openexec/openexec/pkg/version"
)

const (
	updateCheckURL     = "https://openexec.io/version.txt"
	updateCheckTimeout = 3 * time.Second
	updateCheckFile    = ".last_update_check"
	updateCheckInterval = 24 * time.Hour
)

// checkForUpdate checks if a newer version is available.
// Runs at most once per 24 hours, non-blocking on network errors.
func checkForUpdate() {
	// Skip in CI or when explicitly disabled
	if os.Getenv("CI") != "" || os.Getenv("OPENEXEC_NO_UPDATE_CHECK") == "1" {
		return
	}

	// Rate limit: check at most once per day
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	checkFile := filepath.Join(homeDir, ".openexec", updateCheckFile)
	if info, err := os.Stat(checkFile); err == nil {
		if time.Since(info.ModTime()) < updateCheckInterval {
			return // Checked recently
		}
	}

	// Fetch latest version (with short timeout)
	client := &http.Client{Timeout: updateCheckTimeout}
	resp, err := client.Get(updateCheckURL)
	if err != nil {
		return // Network error — silently skip
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	latest := strings.TrimSpace(string(body))
	current := version.Version

	// Touch the check file to record we checked
	_ = os.MkdirAll(filepath.Dir(checkFile), 0755)
	_ = os.WriteFile(checkFile, []byte(latest), 0644)

	// Compare versions
	if latest != "" && latest != current && isNewer(latest, current) {
		fmt.Fprintf(os.Stderr, "%s %s → %s\n",
			color.YellowString("Update available:"),
			current, color.GreenString(latest))
		fmt.Fprintf(os.Stderr, "  Run: %s\n\n",
			color.CyanString("curl -sSfL https://openexec.io/install.sh | sh"))
	}
}

// isNewer returns true if latest is a newer semver than current.
// Simple comparison: split by "." and compare numerically.
func isNewer(latest, current string) bool {
	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	for i := 0; i < len(latestParts) && i < len(currentParts); i++ {
		var l, c int
		fmt.Sscanf(latestParts[i], "%d", &l)
		fmt.Sscanf(currentParts[i], "%d", &c)
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return len(latestParts) > len(currentParts)
}
