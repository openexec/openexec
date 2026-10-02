package gates

import (
	"fmt"
	"path"
	"strings"
)

func validRepairPath(p string) bool {
	return p != "" && p != "." && !strings.Contains(p, "\\") && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

// ValidateRepairScope requires exact repository-relative files or explicit
// directory prefixes ending in /. Missing scope never means whole repository.
func ValidateRepairScope(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("repair requires persisted allowed_paths")
	}
	for _, p := range paths {
		if !validRepairPath(strings.TrimSuffix(p, "/")) {
			return fmt.Errorf("invalid repair scope %q", p)
		}
	}
	return nil
}
