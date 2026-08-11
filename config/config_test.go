package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sfwr.conf")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingDefaultFileFallsBackToDefaults(t *testing.T) {
	t.Chdir(t.TempDir())

	settings, err := Load("")
	if err != nil {
		t.Fatalf("a missing default config should not be an error: %v", err)
	}
	if settings.Database != "sfwr_database.db" {
		t.Errorf("Database = %q, want the default", settings.Database)
	}
	if settings.SourceFile != "" {
		t.Errorf("SourceFile = %q, want empty when no file was read", settings.SourceFile)
	}
}

func TestMissingNamedFileIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.conf")); err == nil {
		t.Error("an explicitly named config file that does not exist should be an error")
	}
}

func TestLoadReadsSettings(t *testing.T) {
	path := writeConfig(t, `
# A comment, then a blank line

database     = /srv/books/books.db
cover_images = /srv/books/covers
output       = /home/user/example.com
templates    = /srv/books/templates
repo         = /srv/books
site_name    = Example Books
bind         = 0.0.0.0
port         = 9000
behind_proxy = true
`)

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	checks := map[string]string{
		settings.Database:    "/srv/books/books.db",
		settings.CoverImages: "/srv/books/covers",
		settings.Output:      "/home/user/example.com",
		settings.Templates:   "/srv/books/templates",
		settings.Repo:        "/srv/books",
		settings.SiteName:    "Example Books",
		settings.Bind:        "0.0.0.0",
		settings.Port:        "9000",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if !settings.BehindProxy {
		t.Error("behind_proxy = true was not applied")
	}
}

// Relative paths are anchored at the config file so the program can be
// started from any working directory, which is how a service runs it.
func TestRelativePathsResolveAgainstTheConfigFile(t *testing.T) {
	path := writeConfig(t, "database = data/books.db\noutput = public\n")
	dir := filepath.Dir(path)

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if want := filepath.Join(dir, "data/books.db"); settings.Database != want {
		t.Errorf("Database = %q, want %q", settings.Database, want)
	}
	if want := filepath.Join(dir, "public"); settings.Output != want {
		t.Errorf("Output = %q, want %q", settings.Output, want)
	}
}

func TestAbsolutePathsAreLeftAlone(t *testing.T) {
	path := writeConfig(t, "output = /var/www/example.com\n")

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.Output != "/var/www/example.com" {
		t.Errorf("Output = %q, want the absolute path unchanged", settings.Output)
	}
}

func TestUnsetKeysKeepTheirDefaults(t *testing.T) {
	path := writeConfig(t, "port = 9000\n")

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.Bind != "127.0.0.1" {
		t.Errorf("Bind = %q, want the loopback default", settings.Bind)
	}
	if !strings.HasSuffix(settings.Database, "sfwr_database.db") {
		t.Errorf("Database = %q, want the default file name", settings.Database)
	}
}

func TestUnknownSettingIsRejected(t *testing.T) {
	path := writeConfig(t, "colour = blue\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("an unknown setting should be reported, not ignored")
	}
	if !strings.Contains(err.Error(), "colour") {
		t.Errorf("the error should name the bad setting, got: %v", err)
	}
}

func TestMalformedLineIsRejected(t *testing.T) {
	path := writeConfig(t, "database\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("a line without '=' should be reported")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("the error should give the line number, got: %v", err)
	}
}

func TestBadBooleanIsRejected(t *testing.T) {
	path := writeConfig(t, "behind_proxy = yes-please\n")

	if _, err := Load(path); err == nil {
		t.Error("a non-boolean behind_proxy should be reported")
	}
}

func TestQuotedValuesAreUnwrapped(t *testing.T) {
	path := writeConfig(t, `site_name = "Science Fiction Worth Reading"`+"\n")

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.SiteName != "Science Fiction Worth Reading" {
		t.Errorf("SiteName = %q, want the quotes stripped", settings.SiteName)
	}
}

// Templates live in the binary, so a missing directory is not an error:
// it just means the embedded copy is used.
func TestCheckIgnoresTheTemplatesDirectory(t *testing.T) {
	settings := Defaults()
	settings.Templates = filepath.Join(t.TempDir(), "not-there")
	settings.Output = "out"

	if err := settings.Check(); err != nil {
		t.Errorf("a missing templates directory should be fine, got: %v", err)
	}
}

func TestCheckRequiresAnOutputDirectory(t *testing.T) {
	settings := Defaults()
	settings.Output = ""

	if err := settings.Check(); err == nil {
		t.Error("Check should fail with no output directory")
	}
}

func TestDeployEnabled(t *testing.T) {
	settings := Defaults()
	if !settings.DeployEnabled() {
		t.Error("the default repo should enable deploy")
	}

	settings.Repo = ""
	if settings.DeployEnabled() {
		t.Error("an empty repo should disable deploy")
	}
}

func TestBasePathIsNormalized(t *testing.T) {
	cases := map[string]string{
		"/admin":  "/admin",
		"admin":   "/admin",
		"/admin/": "/admin",
		"admin/":  "/admin",
		"/":       "",
		"":        "",
		"  ":      "",
		"a/b":     "/a/b",
	}
	for input, want := range cases {
		if got := NormalizeBasePath(input); got != want {
			t.Errorf("NormalizeBasePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBasePathReadFromFile(t *testing.T) {
	path := writeConfig(t, "base_path = /admin/\n")

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.BasePath != "/admin" {
		t.Errorf("BasePath = %q, want /admin", settings.BasePath)
	}
}

func TestBasePathDefaultsToRoot(t *testing.T) {
	if Defaults().BasePath != "" {
		t.Error("the default should serve from the root")
	}
}

func TestRemoteSettings(t *testing.T) {
	path := writeConfig(t, "remote_host = user@example.com\nremote_dir = /home/user/sfwr\n")

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.RemoteHost != "user@example.com" || settings.RemoteDir != "/home/user/sfwr" {
		t.Errorf("remote = %q %q, want the configured values", settings.RemoteHost, settings.RemoteDir)
	}
	if !settings.SyncEnabled() {
		t.Error("both remote settings present should enable syncing")
	}
}

// remote_dir is a path on the other machine, so it must not be rewritten
// to sit under this machine's config directory.
func TestRemoteDirIsNotResolvedLocally(t *testing.T) {
	path := writeConfig(t, "remote_dir = sfwr\nremote_host = user@example.com\n")

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.RemoteDir != "sfwr" {
		t.Errorf("RemoteDir = %q, want it left exactly as written", settings.RemoteDir)
	}
}

func TestSyncDisabledWithoutBothSettings(t *testing.T) {
	for _, body := range []string{"", "remote_host = user@example.com\n", "remote_dir = /srv/sfwr\n"} {
		settings, err := Load(writeConfig(t, body))
		if err != nil {
			t.Fatal(err)
		}
		if settings.SyncEnabled() {
			t.Errorf("sync should be disabled for config %q", body)
		}
	}
}
