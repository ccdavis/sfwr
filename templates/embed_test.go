package templates

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// Everything the program renders has to be in the binary, or a deployment
// that relies on the embedded copy fails at the first page instead of at
// startup.
func TestEmbeddedSetIsComplete(t *testing.T) {
	site := []string{
		"base.html", "child_dir_base.html", "index.html", "book.html",
		"book_list.html", "book_boxes.html", "author.html", "author_index.html",
		"decade.html", "decades_index.html", "recent_books.html",
	}
	admin := []string{
		"base.html", "home.html", "book_list.html", "book_form.html",
		"author_list.html", "author_form.html", "author_edit.html", "error.html",
		"decades.html", "decade.html", "backups.html", "login.html", "preview.html",
	}

	for _, name := range site {
		if _, err := fs.Stat(Embedded(), name); err != nil {
			t.Errorf("site template %s is not embedded: %v", name, err)
		}
	}
	for _, name := range admin {
		if _, err := fs.Stat(Embedded(), "web/"+name); err != nil {
			t.Errorf("admin template web/%s is not embedded: %v", name, err)
		}
	}
}

// The embedded copy must match the working tree, or a build silently ships
// stale markup.
func TestEmbeddedMatchesTheDirectory(t *testing.T) {
	entries, err := fs.Glob(Embedded(), "*.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no templates embedded")
	}

	for _, name := range entries {
		embedded, err := fs.ReadFile(Embedded(), name)
		if err != nil {
			t.Fatal(err)
		}
		onDisk, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s from the working tree: %v", name, err)
		}
		if string(embedded) != string(onDisk) {
			t.Errorf("%s differs between the binary and the working tree", name)
		}
	}
}

func TestFSPrefersAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.html"), []byte("override"), 0644); err != nil {
		t.Fatal(err)
	}

	body, err := fs.ReadFile(FS(dir), "base.html")
	if err != nil {
		t.Fatalf("reading from the override directory: %v", err)
	}
	if string(body) != "override" {
		t.Errorf("got %q, want the directory's copy", body)
	}
}

// A binary dropped on a server with no template directory has to work, so
// an absent or unset path falls back rather than failing.
func TestFSFallsBackToTheEmbeddedCopy(t *testing.T) {
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "missing")} {
		if _, err := fs.Stat(FS(dir), "base.html"); err != nil {
			t.Errorf("FS(%q) does not provide base.html: %v", dir, err)
		}
		if Source(dir) != "built into the binary" {
			t.Errorf("Source(%q) = %q, want the embedded description", dir, Source(dir))
		}
	}
}

func TestCheckRejectsAnIncompleteSet(t *testing.T) {
	if err := Check(Embedded()); err != nil {
		t.Errorf("the embedded set should pass Check: %v", err)
	}
	if err := Check(os.DirFS(t.TempDir())); err == nil {
		t.Error("an empty directory should fail Check")
	}
}
