package web

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/site"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// reopenDatabase drops the current connection and opens the database file
// again. A git rollback replaces the file underneath us, so the existing
// handle would keep serving the pre-rollback contents.
func (ws *WebServer) reopenDatabase() error {
	if sqlDB, err := ws.db.DB(); err == nil {
		sqlDB.Close()
	}

	db, err := gorm.Open(sqlite.Open(ws.config.DatabasePath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to reopen %s: %w", ws.config.DatabasePath, err)
	}
	ws.db = db
	return nil
}

// git runs a git command inside the configured working tree. Every git call
// goes through here so none of them depend on the process's directory,
// which is arbitrary when the server runs as a service.
func (ws *WebServer) git(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = ws.config.RepoDir
	return cmd
}

// repoRelative expresses a configured path relative to the git working
// tree, which is how git wants to be given pathspecs.
func (ws *WebServer) repoRelative(target string) (string, error) {
	repo, err := filepath.Abs(ws.config.RepoDir)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(repo, absolute)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%s is outside the git working tree at %s", target, repo)
	}
	return rel, nil
}

func (ws *WebServer) deployToGitHub() (string, error) {
	if !ws.config.DeployEnabled() {
		return "", fmt.Errorf("no git working tree configured; set 'repo' in %s", ws.configFileLabel())
	}

	// Check if we're in a git repository
	if err := ws.git("status").Run(); err != nil {
		return "", fmt.Errorf("%s is not a git repository: %v", ws.config.RepoDir, err)
	}

	dbPath, err := ws.repoRelative(ws.config.DatabasePath)
	if err != nil {
		return "", fmt.Errorf("cannot version the database: %w", err)
	}
	if err := ws.git("add", dbPath).Run(); err != nil {
		return "", fmt.Errorf("failed to stage database: %v", err)
	}

	// Stage cover images when they live inside the repository.
	if coverPath, err := ws.repoRelative(ws.config.CoverImagesDir); err == nil {
		if _, statErr := os.Stat(ws.config.CoverImagesDir); statErr == nil {
			if output, addErr := ws.git("add", coverPath).CombinedOutput(); addErr != nil {
				// Non-fatal: images might already be committed
				log.Printf("Warning: could not stage cover images: %v\n%s", addErr, output)
			}
		}
	}

	// Check if there are actual changes to commit
	hasChanges := ws.git("diff", "--cached", "--exit-code").Run() != nil

	if hasChanges {
		// Create deployment checkpoint commit
		bookCount := ws.getBookCount()
		authorCount := ws.getAuthorCount()
		commitMsg := fmt.Sprintf("[DEPLOY] %d books, %d authors - %s", bookCount, authorCount, getTimestamp())
		if output, err := ws.git("commit", "-m", commitMsg).CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to commit: %v\n%s", err, output)
		}
	}

	// Push current branch to remote
	branchOutput, err := ws.git("rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to determine current branch: %v", err)
	}
	branch := strings.TrimSpace(string(branchOutput))

	if output, err := ws.git("push", "origin", branch).CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to push to GitHub: %v\n%s", err, output)
	}

	if hasChanges {
		return "Successfully deployed! New checkpoint created and pushed to GitHub.", nil
	}
	return "No changes since last deployment. Pushed any pending commits.", nil
}

// buildStatic regenerates the public site in this process. It used to run
// "./sfwr -build", which only worked when the server's working directory
// happened to contain the executable.
func (ws *WebServer) buildStatic() (string, error) {
	return site.Generate(ws.db, site.Options{
		Templates:      ws.config.Templates,
		OutputDir:      ws.config.OutputDir,
		CoverImagesDir: ws.config.CoverImagesDir,
	})
}

// configFileLabel names the settings file for error messages.
func (ws *WebServer) configFileLabel() string {
	if ws.config.ConfigFile == "" {
		return "the configuration"
	}
	return ws.config.ConfigFile
}

func getTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (ws *WebServer) getBookCount() int {
	var count int64
	ws.db.Model(&models.Book{}).Count(&count)
	return int(count)
}

func (ws *WebServer) getAuthorCount() int {
	var count int64
	ws.db.Model(&models.Author{}).Count(&count)
	return int(count)
}

type GitCommit struct {
	Hash      string
	Message   string
	Date      string
	BookCount int
}

// GetRecentCommits returns recent deployment commits from git history
func (ws *WebServer) GetRecentCommits() ([]GitCommit, error) {
	if !ws.config.DeployEnabled() {
		return nil, fmt.Errorf("no git working tree configured; set 'repo' in %s", ws.configFileLabel())
	}

	dbPath, err := ws.repoRelative(ws.config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read database history: %w", err)
	}

	// Only get commits with the [DEPLOY] tag. --fixed-strings matters: as a
	// regex, [DEPLOY] is a character class that matches any commit message
	// containing one of those six letters.
	output, err := ws.git("log", "--grep=[DEPLOY]", "--fixed-strings", "--oneline", "-n", "20", "--", dbPath).Output()
	if err != nil {
		// Fallback to all commits if no deploy commits found
		output, err = ws.git("log", "--oneline", "-n", "20", "--", dbPath).Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get git history: %v", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	commits := make([]GitCommit, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		// Get full commit info
		fullInfo, err := ws.git("show", "--format=%H|%s|%ai", "-s", parts[0]).Output()
		if err != nil {
			continue
		}

		infoParts := strings.Split(strings.TrimSpace(string(fullInfo)), "|")
		if len(infoParts) >= 3 {
			commit := GitCommit{
				Hash:    infoParts[0],
				Message: infoParts[1],
				Date:    infoParts[2],
			}

			// Extract book count from message if present
			if strings.Contains(commit.Message, "books") {
				// Try to extract number
				for _, word := range strings.Fields(commit.Message) {
					if num, err := strconv.Atoi(strings.TrimSuffix(word, ",")); err == nil {
						commit.BookCount = num
						break
					}
				}
			}

			commits = append(commits, commit)
		}
	}

	return commits, nil
}

// RollbackToCommit rolls back the database to a specific commit
func (ws *WebServer) RollbackToCommit(commitHash string) error {
	if !ws.config.DeployEnabled() {
		return fmt.Errorf("no git working tree configured; set 'repo' in %s", ws.configFileLabel())
	}

	dbPath, err := ws.repoRelative(ws.config.DatabasePath)
	if err != nil {
		return fmt.Errorf("cannot roll back the database: %w", err)
	}

	// Check for uncommitted changes
	if err := ws.git("diff", "--exit-code", "--", dbPath).Run(); err != nil {
		// There are uncommitted changes - warn the user
		return fmt.Errorf("you have unsaved changes. Please deploy first to save your current state, then rollback")
	}

	// Checkout the database file from the specified commit
	if output, err := ws.git("checkout", commitHash, "--", dbPath).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to rollback database: %v\n%s", err, output)
	}

	// Also try to checkout cover images from that commit. Non-fatal: the
	// images directory may not exist in older commits.
	if coverPath, err := ws.repoRelative(ws.config.CoverImagesDir); err == nil {
		if output, err := ws.git("checkout", commitHash, "--", coverPath).CombinedOutput(); err != nil {
			log.Printf("Note: could not restore cover images for %s: %v\n%s", commitHash, err, output)
		}
	}

	return nil
}
