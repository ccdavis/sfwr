package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/web"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupIntegrationTest creates a complete test environment
func setupIntegrationTest(t *testing.T) (string, *gorm.DB, func()) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Save current directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Change to temp directory
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create test database
	testDBPath := filepath.Join(tmpDir, "test_sfwr_database.db")
	db, err := gorm.Open(sqlite.Open(testDBPath), &gorm.Config{})
	if err != nil {
		t.Fatal("Failed to create test database:", err)
	}

	// Migrate schema
	err = db.AutoMigrate(
		&models.Book{},
		&models.Author{},
		&models.OpenLibraryBookAuthor{},
		&models.OpenLibraryBookIsbn{},
	)
	if err != nil {
		t.Fatal("Failed to migrate test database:", err)
	}

	// Initialize git repository for deployment tests
	cmd := exec.Command("git", "init")
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Run()

	// Create templates directory structure
	os.MkdirAll("templates/web", 0755)
	os.MkdirAll("saved_cover_images", 0755)
	os.MkdirAll("output/public", 0755)

	// Create minimal templates for testing
	createTestTemplates(tmpDir)

	// Cleanup function
	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.Chdir(originalDir)
		// tmpDir is automatically cleaned up by t.TempDir()
	}

	return tmpDir, db, cleanup
}

func createTestTemplates(dir string) {
	// Create base template
	baseTemplate := `<!DOCTYPE html>
<html>
<head><title>{{.Title}}</title></head>
<body>
{{template "content" .}}
</body>
</html>
{{define "content"}}{{end}}`

	os.WriteFile(filepath.Join(dir, "templates/web/base.html"), []byte(baseTemplate), 0644)

	// Create other necessary templates
	templates := map[string]string{
		"home.html": `{{template "base.html" .}}
{{define "content"}}<h1>{{.Title}}</h1>{{end}}`,
		"books.html": `{{template "base.html" .}}
{{define "content"}}<h1>Books</h1>{{range .Books}}<p>{{.MainTitle}}</p>{{end}}{{end}}`,
		"book_new.html": `{{template "base.html" .}}
{{define "content"}}<h1>New Book</h1>{{end}}`,
		"book_edit.html": `{{template "base.html" .}}
{{define "content"}}<h1>Edit Book</h1>{{end}}`,
		"authors.html": `{{template "base.html" .}}
{{define "content"}}<h1>Authors</h1>{{range .Authors}}<p>{{.FullName}}</p>{{end}}{{end}}`,
		"author_new.html": `{{template "base.html" .}}
{{define "content"}}<h1>New Author</h1>{{end}}`,
		"backups.html": `{{template "base.html" .}}
{{define "content"}}<h1>Backups</h1>{{end}}`,
	}

	for name, content := range templates {
		os.WriteFile(filepath.Join(dir, "templates/web", name), []byte(content), 0644)
	}

	// Create page templates
	os.MkdirAll(filepath.Join(dir, "templates"), 0755)
	pageTemplates := map[string]string{
		"index.html":      `<html><body><h1>Books</h1></body></html>`,
		"book_list.html":  `<html><body><h1>Book List</h1></body></html>`,
		"book_boxes.html": `<html><body><h1>Book Grid</h1></body></html>`,
	}

	for name, content := range pageTemplates {
		os.WriteFile(filepath.Join(dir, "templates", name), []byte(content), 0644)
	}
}

func TestCompleteBookLifecycle(t *testing.T) {
	_, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Step 1: Create an author
	author := models.Author{
		FullName: "Integration Author",
		Surname:  "Author",
	}
	result := db.Create(&author)
	if result.Error != nil {
		t.Fatal("Failed to create author:", result.Error)
	}

	// Step 2: Create a book
	book := models.Book{
		MainTitle:      "Integration Test Book",
		SubTitle:       "Testing Everything",
		AuthorFullName: author.FullName,
		AuthorSurname:  author.Surname,
		PubDate:        2024,
		Rating:         "Excellent",
		Review:         "This is an integration test book",
		Isbn:           "9780123456789",
		OpenLibraryId:  "OL12345M",
	}

	bookID, err := book.Create(db)
	if err != nil {
		t.Fatal("Failed to create book:", err)
	}

	// Step 3: Associate book with author
	db.Model(&book).Association("Authors").Append(&author)

	// Step 4: Load and verify
	var loadedBook models.Book
	db.Preload("Authors").First(&loadedBook, bookID)

	if loadedBook.MainTitle != book.MainTitle {
		t.Errorf("Book title mismatch: got %s, want %s", loadedBook.MainTitle, book.MainTitle)
	}

	if len(loadedBook.Authors) != 1 {
		t.Errorf("Expected 1 author, got %d", len(loadedBook.Authors))
	}

	// Step 5: Update the book
	loadedBook.Review = "Updated review"
	loadedBook.Rating = "Very-Good"
	db.Save(&loadedBook)

	// Step 6: Verify update
	var updatedBook models.Book
	db.First(&updatedBook, bookID)

	if updatedBook.Review != "Updated review" {
		t.Error("Book review was not updated")
	}

	if updatedBook.Rating != "Very-Good" {
		t.Error("Book rating was not updated")
	}

	// Step 7: Delete the book
	db.Delete(&models.Book{}, bookID)

	// Step 8: Verify deletion
	var deletedBook models.Book
	result = db.First(&deletedBook, bookID)
	if result.Error == nil {
		t.Error("Book should have been deleted")
	}

	// Step 9: Verify author still exists
	var authorStillExists models.Author
	db.First(&authorStillExists, author.ID)
	if authorStillExists.ID == 0 {
		t.Error("Author should still exist after book deletion")
	}
}

func TestDeploymentAndRollbackIntegration(t *testing.T) {
	tmpDir, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ws := web.NewWebServer(db, filepath.Join(tmpDir, "saved_cover_images"))

	// Stage 1: Create initial state with 3 books
	for i := 1; i <= 3; i++ {
		author := models.Author{
			FullName: fmt.Sprintf("Author %d", i),
			Surname:  fmt.Sprintf("Surname%d", i),
		}
		db.Create(&author)

		book := models.Book{
			MainTitle:      fmt.Sprintf("Book %d", i),
			AuthorFullName: author.FullName,
			AuthorSurname:  author.Surname,
			Rating:         "Very-Good",
			PubDate:        2020 + uint(i),
		}
		db.Create(&book)
		db.Model(&book).Association("Authors").Append(&author)
	}

	// Create sfwr_database.db file for git operations
	sqlDB, _ := db.DB()
	sqlDB.Close()

	// Copy test database to expected name
	testDBPath := filepath.Join(tmpDir, "test_sfwr_database.db")
	expectedDBPath := filepath.Join(tmpDir, "sfwr_database.db")

	cmd := exec.Command("cp", testDBPath, expectedDBPath)
	cmd.Run()

	// Reopen database
	db, _ = gorm.Open(sqlite.Open(expectedDBPath), &gorm.Config{})
	ws = web.NewWebServer(db, filepath.Join(tmpDir, "saved_cover_images"))

	// Stage 2: Create first deployment
	cmd = exec.Command("git", "add", ".")
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "[DEPLOY] Initial state - 3 books, 3 authors")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Log("Git commit output:", string(output))
	}

	// Get first commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	commit1Output, _ := cmd.Output()
	commit1 := strings.TrimSpace(string(commit1Output))

	// Stage 3: Add 2 more books
	for i := 4; i <= 5; i++ {
		book := models.Book{
			MainTitle: fmt.Sprintf("Book %d", i),
			Rating:    "Excellent",
			PubDate:   2024,
		}
		db.Create(&book)
	}

	// Verify we have 5 books
	var count1 int64
	db.Model(&models.Book{}).Count(&count1)
	if count1 != 5 {
		t.Errorf("Expected 5 books after adding more, got %d", count1)
	}

	// Stage 4: Create second deployment
	cmd = exec.Command("git", "add", "sfwr_database.db")
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "[DEPLOY] Added books - 5 books, 3 authors")
	cmd.Run()

	// Stage 5: Get deployment history
	commits, err := ws.GetRecentCommits()
	if err != nil {
		t.Log("Warning: Could not get commits:", err)
		// Continue test even if this fails
	} else {
		deployCount := 0
		for _, c := range commits {
			if strings.Contains(c.Message, "[DEPLOY]") {
				deployCount++
			}
		}
		if deployCount < 2 {
			t.Errorf("Expected at least 2 deployment commits, got %d", deployCount)
		}
	}

	// Stage 6: Test rollback
	err = ws.RollbackToCommit(commit1)
	if err != nil {
		t.Fatal("Rollback failed:", err)
	}

	// Close and reopen database after rollback
	sqlDB, _ = db.DB()
	sqlDB.Close()
	db, _ = gorm.Open(sqlite.Open(expectedDBPath), &gorm.Config{})

	// Stage 7: Verify rollback worked
	var count2 int64
	db.Model(&models.Book{}).Count(&count2)
	if count2 != 3 {
		t.Errorf("Expected 3 books after rollback, got %d", count2)
	}

	// Verify specific books exist
	var books []models.Book
	db.Find(&books)
	for _, book := range books {
		bookNum := strings.TrimPrefix(book.MainTitle, "Book ")
		if bookNum == "4" || bookNum == "5" {
			t.Errorf("Book %s should not exist after rollback", book.MainTitle)
		}
	}
}

func TestMultiAuthorBookHandling(t *testing.T) {
	_, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create multiple authors
	author1 := models.Author{FullName: "First Author", Surname: "Author"}
	author2 := models.Author{FullName: "Second Writer", Surname: "Writer"}
	author3 := models.Author{FullName: "Third Contributor", Surname: "Contributor"}

	db.Create(&author1)
	db.Create(&author2)
	db.Create(&author3)

	// Create a book with multiple authors
	book := models.Book{
		MainTitle:      "Collaborative Work",
		AuthorFullName: author1.FullName, // Primary author
		AuthorSurname:  author1.Surname,
		PubDate:        2024,
		Rating:         "Excellent",
	}
	db.Create(&book)

	// Associate all authors with the book
	db.Model(&book).Association("Authors").Append(&author1)
	db.Model(&book).Association("Authors").Append(&author2)
	db.Model(&book).Association("Authors").Append(&author3)

	// Load and verify
	var loadedBook models.Book
	db.Preload("Authors").First(&loadedBook, book.ID)

	if len(loadedBook.Authors) != 3 {
		t.Errorf("Expected 3 authors, got %d", len(loadedBook.Authors))
	}

	// Verify all authors are associated
	authorNames := make(map[string]bool)
	for _, author := range loadedBook.Authors {
		authorNames[author.FullName] = true
	}

	expectedAuthors := []string{"First Author", "Second Writer", "Third Contributor"}
	for _, name := range expectedAuthors {
		if !authorNames[name] {
			t.Errorf("Missing author: %s", name)
		}
	}

	// Remove one author
	db.Model(&book).Association("Authors").Delete(&author2)

	// Verify removal
	db.Preload("Authors").First(&loadedBook, book.ID)
	if len(loadedBook.Authors) != 2 {
		t.Errorf("Expected 2 authors after removal, got %d", len(loadedBook.Authors))
	}

	// Verify the right author was removed
	for _, author := range loadedBook.Authors {
		if author.FullName == "Second Writer" {
			t.Error("Second Writer should have been removed")
		}
	}
}

func TestConcurrentBookOperations(t *testing.T) {
	_, db, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create an author for all books
	author := models.Author{
		FullName: "Concurrent Author",
		Surname:  "Author",
	}
	db.Create(&author)

	// Number of concurrent operations
	numOperations := 10
	done := make(chan bool, numOperations)
	errors := make(chan error, numOperations)

	// Concurrent book creation
	for i := 0; i < numOperations; i++ {
		go func(index int) {
			book := models.Book{
				MainTitle:      fmt.Sprintf("Concurrent Book %d", index),
				AuthorFullName: author.FullName,
				AuthorSurname:  author.Surname,
				PubDate:        2024,
				Rating:         "Very-Good",
			}

			if _, err := book.Create(db); err != nil {
				errors <- fmt.Errorf("Failed to create book %d: %v", index, err)
			} else {
				done <- true
			}
		}(i)
	}

	// Wait for all operations with timeout
	timeout := time.After(5 * time.Second)
	successCount := 0

	for i := 0; i < numOperations; i++ {
		select {
		case <-done:
			successCount++
		case err := <-errors:
			t.Error(err)
		case <-timeout:
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	if successCount != numOperations {
		t.Errorf("Expected %d successful operations, got %d", numOperations, successCount)
	}

	// Verify all books were created
	var count int64
	db.Model(&models.Book{}).Count(&count)
	if count != int64(numOperations) {
		t.Errorf("Expected %d books, got %d", numOperations, count)
	}
}

func TestDatabaseMigrationIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration_test.db")

	// Create database with initial schema
	db1, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal("Failed to create database:", err)
	}

	// Migrate initial schema
	err = db1.AutoMigrate(&models.Book{}, &models.Author{})
	if err != nil {
		t.Fatal("Failed to migrate initial schema:", err)
	}

	// Add test data
	author := models.Author{FullName: "Test Author", Surname: "Author"}
	db1.Create(&author)

	book := models.Book{
		MainTitle:      "Test Book",
		AuthorFullName: author.FullName,
		AuthorSurname:  author.Surname,
	}
	db1.Create(&book)
	db1.Model(&book).Association("Authors").Append(&author)

	// Close first connection
	sqlDB1, _ := db1.DB()
	sqlDB1.Close()

	// Reopen and migrate with full schema
	db2, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal("Failed to reopen database:", err)
	}

	err = db2.AutoMigrate(
		&models.Book{},
		&models.Author{},
		&models.OpenLibraryBookAuthor{},
		&models.OpenLibraryBookIsbn{},
	)
	if err != nil {
		t.Fatal("Failed to migrate full schema:", err)
	}

	// Verify data integrity after migration
	var books []models.Book
	db2.Preload("Authors").Find(&books)

	if len(books) != 1 {
		t.Errorf("Expected 1 book after migration, got %d", len(books))
	}

	if len(books) > 0 {
		if books[0].MainTitle != "Test Book" {
			t.Error("Book data corrupted after migration")
		}
		if len(books[0].Authors) != 1 {
			t.Error("Book-author relationship lost after migration")
		}
	}

	// Close second connection
	sqlDB2, _ := db2.DB()
	sqlDB2.Close()
}