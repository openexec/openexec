//go:build !ui_dist

package openexec

import (
	"embed"
	"io/fs"
)

// The default module build embeds a small tracked fallback so downstream Go
// modules remain buildable without generated ui/dist artifacts.

//go:embed all:assets/ui-fallback
var uiAssets embed.FS

func GetUIFS() fs.FS {
	f, _ := fs.Sub(uiAssets, "assets/ui-fallback")
	return f
}
