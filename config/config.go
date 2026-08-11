// Package config loads the small settings file that tells sfwr where things
// live, so one build of the program can serve any static book site.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultFileName is the config file looked for next to the working
// directory when -config is not given.
const DefaultFileName = "sfwr.conf"

// Settings is the whole configuration. Every path may be relative (resolved
// against the config file's directory) or absolute.
type Settings struct {
	// Database is the SQLite file holding books and authors.
	Database string

	// CoverImages is the directory of downloaded cover art.
	CoverImages string

	// Output is where the generated static site is written. On a host that
	// serves files straight from a directory, point this at the document
	// root and a build publishes immediately.
	Output string

	// Templates is the directory holding the site templates. It must
	// contain the admin templates in a web/ subdirectory.
	Templates string

	// Repo is the git working tree used by deploy and rollback. Empty
	// disables both.
	Repo string

	// SiteName labels the collection in the admin UI.
	SiteName string

	// RemoteHost and RemoteDir locate the other installation for sync.sh:
	// an ssh destination ("user@example.com") and the directory there that
	// holds the database, cover art, and config. Empty disables syncing.
	RemoteHost string
	RemoteDir  string

	// Bind, Port, and BehindProxy configure the admin server.
	Bind        string
	Port        string
	BehindProxy bool

	// BasePath mounts the admin UI under a sub-path of a larger site, e.g.
	// "/admin" when a reverse proxy forwards example.com/admin here. Empty
	// serves from the root. Always normalized to a leading slash and no
	// trailing slash.
	BasePath string

	// Path of the file these settings came from, empty if defaults.
	SourceFile string
}

// Defaults returns the settings used when no config file is present. They
// match the layout of a freshly cloned repository, so running sfwr from the
// checkout keeps working with no config file at all.
func Defaults() Settings {
	return Settings{
		Database:    "sfwr_database.db",
		CoverImages: "saved_cover_images",
		Output:      "output/public",
		Templates:   "templates",
		Repo:        ".",
		SiteName:    "SFWR",
		Bind:        "127.0.0.1",
		Port:        "",
		BehindProxy: false,
	}
}

// Load reads a config file. A blank path looks for DefaultFileName in the
// working directory and falls back to Defaults() when it does not exist.
// An explicitly named file that is missing is an error.
func Load(path string) (Settings, error) {
	settings := Defaults()

	explicit := path != ""
	if !explicit {
		path = DefaultFileName
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return settings, nil
		}
		return settings, fmt.Errorf("could not read config %s: %w", path, err)
	}
	defer file.Close()

	if err := parse(file, &settings, path); err != nil {
		return settings, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return settings, fmt.Errorf("could not resolve %s: %w", path, err)
	}
	settings.SourceFile = absPath

	// Relative paths in the file are read as relative to the file itself,
	// so the program can be started from any directory.
	base := filepath.Dir(absPath)
	settings.Database = resolve(base, settings.Database)
	settings.CoverImages = resolve(base, settings.CoverImages)
	settings.Output = resolve(base, settings.Output)
	settings.Templates = resolve(base, settings.Templates)
	if settings.Repo != "" {
		settings.Repo = resolve(base, settings.Repo)
	}

	return settings, nil
}

// parse reads "key = value" lines, ignoring blanks and # comments.
func parse(file *os.File, settings *Settings, path string) error {
	scanner := bufio.NewScanner(file)
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("%s line %d: expected 'key = value', got %q", path, lineNo, line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)

		if err := assign(settings, key, value); err != nil {
			return fmt.Errorf("%s line %d: %w", path, lineNo, err)
		}
	}
	return scanner.Err()
}

func assign(settings *Settings, key, value string) error {
	switch key {
	case "database":
		settings.Database = value
	case "cover_images":
		settings.CoverImages = value
	case "output":
		settings.Output = value
	case "templates":
		settings.Templates = value
	case "repo":
		settings.Repo = value
	case "site_name":
		settings.SiteName = value
	case "remote_host":
		settings.RemoteHost = value
	case "remote_dir":
		settings.RemoteDir = value
	case "bind":
		settings.Bind = value
	case "port":
		settings.Port = value
	case "base_path":
		settings.BasePath = NormalizeBasePath(value)
	case "behind_proxy":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("behind_proxy must be true or false, got %q", value)
		}
		settings.BehindProxy = parsed
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

// NormalizeBasePath puts a mount point into the one form the rest of the
// code expects: empty, or a leading slash with no trailing slash. "/",
// "admin", "/admin/" and "admin/" all mean the same thing to a user.
func NormalizeBasePath(path string) string {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return ""
	}
	return "/" + trimmed
}

// resolve turns a relative path into one anchored at the config file.
func resolve(base, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(base, path))
}

// AdminTemplates is the directory holding the admin UI templates.
func (s Settings) AdminTemplates() string {
	return filepath.Join(s.Templates, "web")
}

// EnvFileName is the shell file holding the admin password hash. It is kept
// out of the config file so the secret and the settings have separate
// permissions.
const EnvFileName = "sfwr.env"

// EnvFilePath is where the password hash is stored: beside the config file
// that is in use, or the working directory when running on defaults.
func (s Settings) EnvFilePath() string {
	if s.SourceFile == "" {
		return EnvFileName
	}
	return filepath.Join(filepath.Dir(s.SourceFile), EnvFileName)
}

// SyncEnabled reports whether a remote installation is configured.
func (s Settings) SyncEnabled() bool {
	return s.RemoteHost != "" && s.RemoteDir != ""
}

// DeployEnabled reports whether a git working tree is configured.
func (s Settings) DeployEnabled() bool {
	return s.Repo != ""
}

// Check reports problems that would only surface later as confusing
// failures, such as a templates directory that does not exist.
func (s Settings) Check() error {
	if s.Templates == "" {
		return fmt.Errorf("no templates directory configured")
	}
	if info, err := os.Stat(s.AdminTemplates()); err != nil || !info.IsDir() {
		return fmt.Errorf("admin templates not found at %s (set 'templates' in the config file)", s.AdminTemplates())
	}
	if s.Output == "" {
		return fmt.Errorf("no output directory configured")
	}
	return nil
}
