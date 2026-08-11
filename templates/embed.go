// Package templates carries the site and admin templates. They are compiled
// into the binary so a deployment is a single file copy: the templates can
// never be a version behind the code that renders them, which is exactly
// what happens when they travel separately.
//
// A directory can still override them. Point `templates` in sfwr.conf at a
// checkout to edit the markup without rebuilding, which is how the site's
// own styling gets worked on.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
)

// The site templates live at the root of this package and the admin UI's in
// web/. Both are needed at runtime.
//
//go:embed *.html web/*.html
var embedded embed.FS

// Embedded is the copy compiled into the binary.
func Embedded() fs.FS { return embedded }

// FS returns the templates to render from. An empty dir, or one that does
// not exist, falls back to the embedded copy, so a binary dropped onto a
// server with no template directory still works.
func FS(dir string) fs.FS {
	if dir == "" {
		return embedded
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return embedded
	}
	return os.DirFS(dir)
}

// Source describes where the templates came from, for the startup banner.
func Source(dir string) string {
	if fsys := FS(dir); fsys == fs.FS(embedded) {
		return "built into the binary"
	}
	return dir
}

// Check reports whether a template set has the files the program needs. It
// catches a mistyped `templates` path at startup rather than on the first
// page render.
func Check(fsys fs.FS) error {
	for _, name := range []string{"base.html", "child_dir_base.html", "web/base.html"} {
		if _, err := fs.Stat(fsys, name); err != nil {
			return fmt.Errorf("template %s is missing: %w", name, err)
		}
	}
	return nil
}
