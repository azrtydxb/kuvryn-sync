// Package ui embeds the console's single-page application.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist all:placeholder
var files embed.FS

// Files returns the built SPA when dist/index.html exists, and the
// placeholder page explaining how to build it otherwise.
func Files() fs.FS {
	dir := "placeholder"
	if _, err := fs.Stat(files, "dist/index.html"); err == nil {
		dir = "dist"
	}
	sub, err := fs.Sub(files, dir)
	if err != nil {
		// fs.Sub fails only for an invalid path, and both are constants.
		panic(err)
	}
	return sub
}
