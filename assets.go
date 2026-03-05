package openexec

import (
	"embed"
	"io/fs"
)

// UIAssets holds the built React frontend
//go:embed all:ui/dist
var uiAssets embed.FS

// GetUIFS returns the sub-filesystem for the UI assets
func GetUIFS() fs.FS {
	f, _ := fs.Sub(uiAssets, "ui/dist")
	return f
}
