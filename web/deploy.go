package web

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ccdavis/sfwr/models"
)

func (ws *WebServer) deployToGitHub() (string, error) {
	// Check if we're in a git repository
	cmd := exec.Command("git", "status")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not in a git repository: %v", err)
	}

	// Stage database file
	cmd = exec.Command("git", "add", "sfwr_database.db")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to stage database: %v", err)
	}

	// Stage cover images directory
	if _, err := os.Stat("saved_cover_images"); err == nil {
		cmd = exec.Command("git", "add", "saved_cover_images")
		if err := cmd.Run(); err != nil {
			// Non-fatal: images might already be committed
			fmt.Printf("Warning: could not stage images: %v\n", err)
		}
	}

	// Check if there are actual changes to commit
	cmd = exec.Command("git", "diff", "--cached", "--exit-code")
	hasChanges := cmd.Run() != nil

	if hasChanges {
		// Create deployment checkpoint commit
		bookCount := ws.getBookCount()
		authorCount := ws.getAuthorCount()
		commitMsg := fmt.Sprintf("[DEPLOY] %d books, %d authors - %s", bookCount, authorCount, getTimestamp())
		cmd = exec.Command("git", "commit", "-m", commitMsg)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to commit: %v\n%s", err, output)
		}
	}

	// Push to remote
	cmd = exec.Command("git", "push", "origin", "main")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to push to GitHub: %v\n%s", err, output)
	}

	if hasChanges {
		return "Successfully deployed! New checkpoint created. GitHub Actions will now build and publish your site.", nil
	}
	return "No changes since last deployment. Pushed any pending commits. GitHub Actions will build your site.", nil
}

func (ws *WebServer) buildStatic() (string, error) {
	// Run the sfwr build command
	cmd := exec.Command("./sfwr", "-build")

	// Explicitly capture stdout and stderr to prevent any leakage
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		combinedOutput := stdout.String() + stderr.String()
		return "", fmt.Errorf("failed to build static site: %v\n%s", err, combinedOutput)
	}

	// Note: ./sfwr -build already copies cover images to output/public/images/cover_images
	return "Static site built successfully in output/public", nil
}

func copyDir(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	cmd := exec.Command("cp", "-r", src+"/.", dst)
	return cmd.Run()
}

func getTimestamp() string {
	cmd := exec.Command("date", "+%Y-%m-%d %H:%M:%S")
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output))
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
	Hash    string
	Message string
	Date    string
	BookCount int
}

// GetRecentCommits returns recent deployment commits from git history
func (ws *WebServer) GetRecentCommits() ([]GitCommit, error) {
	// Only get commits with [DEPLOY] tag
	cmd := exec.Command("git", "log", "--grep=[DEPLOY]", "--oneline", "-n", "20", "--", "sfwr_database.db")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to all commits if no deploy commits found
		cmd = exec.Command("git", "log", "--oneline", "-n", "20", "--", "sfwr_database.db")
		output, err = cmd.Output()
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
		cmd = exec.Command("git", "show", "--format=%H|%s|%ai", "-s", parts[0])
		fullInfo, err := cmd.Output()
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
	// Check for uncommitted changes
	cmd := exec.Command("git", "diff", "--exit-code", "sfwr_database.db")
	if err := cmd.Run(); err != nil {
		// There are uncommitted changes - warn the user
		return fmt.Errorf("you have unsaved changes. Please deploy first to save your current state, then rollback")
	}

	// Checkout the database file from the specified commit
	cmd = exec.Command("git", "checkout", commitHash, "--", "sfwr_database.db")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to rollback database: %v\n%s", err, output)
	}

	// Also try to checkout cover images from that commit
	cmd = exec.Command("git", "checkout", commitHash, "--", "saved_cover_images")
	cmd.Run() // Ignore errors as images directory might not exist in that commit

	return nil
}