package web

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccdavis/sfwr/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestGitRepo creates a temporary git repository for testing
func setupTestGitRepo(t *testing.T) (string, *gorm.DB, func()) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Change to temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to init git repo:", err)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to config git email:", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to config git name:", err)
	}

	// Create test database
	testDBPath := filepath.Join(tmpDir, "sfwr_database.db")
	db, err := gorm.Open(sqlite.Open(testDBPath), &gorm.Config{})
	if err != nil {
		t.Fatal("Failed to create test database:", err)
	}

	// Migrate schema
	err = db.AutoMigrate(&models.Book{}, &models.Author{})
	if err != nil {
		t.Fatal("Failed to migrate test database:", err)
	}

	// Create initial commit
	cmd = exec.Command("git", "add", ".")
	cmd.Run() // Ignore error if no files

	cmd = exec.Command("git", "commit", "--allow-empty", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to create initial commit:", err)
	}

	// Cleanup function
	cleanup := func() {
		db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.Chdir(originalDir)
		os.RemoveAll(tmpDir)
	}

	return tmpDir, db, cleanup
}

// createTestDeployment creates a test deployment commit
func createTestDeployment(t *testing.T, db *gorm.DB, bookCount int) string {
	// Add test books
	for i := 0; i < bookCount; i++ {
		book := models.Book{
			MainTitle: fmt.Sprintf("Test Book %d", i),
			Rating:    "Very-Good",
		}
		db.Create(&book)
	}

	// Stage database
	cmd := exec.Command("git", "add", "sfwr_database.db")
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to stage database:", err)
	}

	// Create deployment commit
	commitMsg := fmt.Sprintf("[DEPLOY] %d books, 0 authors - test", bookCount)
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	if err := cmd.Run(); err != nil {
		t.Fatal("Failed to create deployment commit:", err)
	}

	// Get commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("Failed to get commit hash:", err)
	}

	return strings.TrimSpace(string(output))
}

func TestGetBookCount(t *testing.T) {
	_, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Test empty database
	count := ws.getBookCount()
	if count != 0 {
		t.Errorf("Expected 0 books, got %d", count)
	}

	// Add books
	for i := 0; i < 5; i++ {
		book := models.Book{
			MainTitle: fmt.Sprintf("Book %d", i),
		}
		db.Create(&book)
	}

	count = ws.getBookCount()
	if count != 5 {
		t.Errorf("Expected 5 books, got %d", count)
	}
}

func TestGetAuthorCount(t *testing.T) {
	_, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Test empty database
	count := ws.getAuthorCount()
	if count != 0 {
		t.Errorf("Expected 0 authors, got %d", count)
	}

	// Add authors
	for i := 0; i < 3; i++ {
		author := models.Author{
			FullName: fmt.Sprintf("Author %d", i),
			Surname:  fmt.Sprintf("Surname%d", i),
		}
		db.Create(&author)
	}

	count = ws.getAuthorCount()
	if count != 3 {
		t.Errorf("Expected 3 authors, got %d", count)
	}
}

func TestGetRecentCommits(t *testing.T) {
	tmpDir, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Create multiple deployments
	_ = createTestDeployment(t, db, 5)
	_ = createTestDeployment(t, db, 10)

	// Create a non-deployment commit
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644)
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Regular commit")
	cmd.Run()

	// Create another deployment
	commit3 := createTestDeployment(t, db, 15)

	// Get recent commits
	commits, err := ws.GetRecentCommits()
	if err != nil {
		t.Fatal("Failed to get recent commits:", err)
	}

	// Should have 3 deployment commits
	deployCount := 0
	for _, commit := range commits {
		if strings.Contains(commit.Message, "[DEPLOY]") {
			deployCount++
		}
	}

	if deployCount < 3 {
		t.Errorf("Expected at least 3 deployment commits, got %d", deployCount)
	}

	// Verify most recent deployment
	if len(commits) > 0 {
		if !strings.Contains(commits[0].Hash, commit3[:7]) &&
		   !strings.Contains(commit3, commits[0].Hash[:7]) {
			t.Errorf("Most recent commit should be %s, got %s", commit3[:7], commits[0].Hash[:7])
		}
	}
}

func TestRollbackToCommit(t *testing.T) {
	_, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Create first deployment with 5 books
	commit1 := createTestDeployment(t, db, 5)

	// Verify 5 books
	var count1 int64
	db.Model(&models.Book{}).Count(&count1)
	if count1 != 5 {
		t.Errorf("Expected 5 books after first deployment, got %d", count1)
	}

	// Create second deployment with 5 more books (total 10)
	createTestDeployment(t, db, 5)

	// Verify 10 books
	var count2 int64
	db.Model(&models.Book{}).Count(&count2)
	if count2 != 10 {
		t.Errorf("Expected 10 books after second deployment, got %d", count2)
	}

	// Rollback to first commit
	err := ws.RollbackToCommit(commit1)
	if err != nil {
		t.Fatal("Failed to rollback:", err)
	}

	// Reload database connection after rollback
	sqlDB, _ := db.DB()
	sqlDB.Close()

	testDBPath := "sfwr_database.db"
	db, err = gorm.Open(sqlite.Open(testDBPath), &gorm.Config{})
	if err != nil {
		t.Fatal("Failed to reopen database after rollback:", err)
	}
	ws.db = db

	// Verify we're back to 5 books
	var count3 int64
	db.Model(&models.Book{}).Count(&count3)
	if count3 != 5 {
		t.Errorf("Expected 5 books after rollback, got %d", count3)
	}
}

func TestRollbackWithUncommittedChanges(t *testing.T) {
	_, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Create a deployment
	commit1 := createTestDeployment(t, db, 5)

	// Make uncommitted changes to database
	book := models.Book{
		MainTitle: "Uncommitted Book",
	}
	db.Create(&book)

	// Save database without committing
	sqlDB, _ := db.DB()
	sqlDB.Close()
	db, _ = gorm.Open(sqlite.Open("sfwr_database.db"), &gorm.Config{})
	ws.db = db

	// Try to rollback - should fail
	err := ws.RollbackToCommit(commit1)
	if err == nil {
		t.Error("Rollback should fail with uncommitted changes")
	}

	if !strings.Contains(err.Error(), "unsaved changes") {
		t.Errorf("Expected error about unsaved changes, got: %v", err)
	}
}

func TestDeployToGitHub(t *testing.T) {
	_, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Add test data
	author := models.Author{
		FullName: "Test Author",
		Surname:  "Author",
	}
	db.Create(&author)

	book := models.Book{
		MainTitle:      "Test Book",
		AuthorFullName: "Test Author",
	}
	db.Create(&book)

	// Mock git remote (will fail on push, but that's ok for test)
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/test/test.git")
	cmd.Run()

	// Deploy should create commit but fail on push
	_, err := ws.deployToGitHub()
	if err == nil || !strings.Contains(err.Error(), "push") {
		// Should fail on push since no real remote
		t.Log("Deploy succeeded or failed for wrong reason:", err)
	}

	// Verify deployment commit was created
	cmd = exec.Command("git", "log", "--oneline", "-1")
	output, _ := cmd.Output()
	lastCommit := string(output)

	if !strings.Contains(lastCommit, "[DEPLOY]") {
		t.Error("Deployment commit was not created")
	}

	if !strings.Contains(lastCommit, "1 books") {
		t.Error("Deployment commit doesn't show correct book count")
	}

	if !strings.Contains(lastCommit, "1 authors") {
		t.Error("Deployment commit doesn't show correct author count")
	}
}

func TestBuildStatic(t *testing.T) {
	_, db, cleanup := setupTestGitRepo(t)
	defer cleanup()

	ws := &WebServer{db: db}

	// Create the sfwr executable mock
	mockScript := `#!/bin/bash
if [ "$1" == "-build" ]; then
    mkdir -p output/public
    echo "Built" > output/public/index.html
    exit 0
fi
exit 1`

	err := os.WriteFile("sfwr", []byte(mockScript), 0755)
	if err != nil {
		t.Fatal("Failed to create mock sfwr:", err)
	}

	// Test build
	message, err := ws.buildStatic()
	if err != nil {
		t.Fatal("Build failed:", err)
	}

	if !strings.Contains(message, "successfully") {
		t.Error("Build message doesn't indicate success:", message)
	}

	// Verify output directory was created
	if _, err := os.Stat("output/public"); os.IsNotExist(err) {
		t.Error("Output directory was not created")
	}
}

func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory with files
	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("test1"), 0644)
	os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("test2"), 0644)

	// Create destination
	dstDir := filepath.Join(tmpDir, "dst")

	// Copy directory
	err := copyDir(srcDir, dstDir)
	if err != nil {
		t.Fatal("Failed to copy directory:", err)
	}

	// Verify files were copied
	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	if err != nil || string(content1) != "test1" {
		t.Error("File1 was not copied correctly")
	}

	content2, err := os.ReadFile(filepath.Join(dstDir, "file2.txt"))
	if err != nil || string(content2) != "test2" {
		t.Error("File2 was not copied correctly")
	}
}