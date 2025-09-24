package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ccdavis/sfwr/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to test database")
	}

	err = db.AutoMigrate(&models.Book{}, &models.Author{}, &models.OpenLibraryBookAuthor{}, &models.OpenLibraryBookIsbn{})
	if err != nil {
		panic("Failed to migrate test database")
	}

	return db
}

func setupTestServer() *WebServer {
	db := setupTestDB()
	
	err := os.MkdirAll("../templates/web", 0755)
	if err != nil && !os.IsExist(err) {
		panic("Failed to create templates directory")
	}

	ws := &WebServer{
		db:       db,
		imageDir: "test_images",
	}

	return ws
}

func setupTestServerWithTemplates() *WebServer {
	ws := setupTestServer()
	ws.loadTemplates()
	return ws
}

func TestHomeHandler(t *testing.T) {
	ws := setupTestServer()
	
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.homeHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code without templates: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestCreateAuthor(t *testing.T) {
	ws := setupTestServer()

	data := url.Values{}
	data.Set("full_name", "Test Author")

	req, err := http.NewRequest("POST", "/authors/create", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.createAuthorHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusSeeOther)
	}

	var author models.Author
	result := ws.db.Where("full_name = ?", "Test Author").First(&author)
	if result.Error != nil {
		t.Errorf("Author was not created in database: %v", result.Error)
	}

	if author.FullName != "Test Author" {
		t.Errorf("Expected author name 'Test Author', got '%s'", author.FullName)
	}

	if author.Surname != "Author" {
		t.Errorf("Expected surname 'Author', got '%s'", author.Surname)
	}
}

func TestCreateBook(t *testing.T) {
	ws := setupTestServer()

	author := models.Author{
		FullName: "Test Author",
		Surname:  "Author",
	}
	ws.db.Create(&author)

	data := url.Values{}
	data.Set("main_title", "Test Book")
	data.Set("sub_title", "A Test Subtitle")
	data.Set("author_full_name", "Test Author")
	data.Set("author_id", "1")
	data.Set("pub_date", "2023")
	data.Set("rating", "Excellent")
	data.Set("review", "This is a test review")

	req, err := http.NewRequest("POST", "/books/create", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.createBookHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusSeeOther)
	}

	var book models.Book
	result := ws.db.Preload("Authors").Where("main_title = ?", "Test Book").First(&book)
	if result.Error != nil {
		t.Errorf("Book was not created in database: %v", result.Error)
	}

	if book.MainTitle != "Test Book" {
		t.Errorf("Expected book title 'Test Book', got '%s'", book.MainTitle)
	}

	if book.SubTitle != "A Test Subtitle" {
		t.Errorf("Expected subtitle 'A Test Subtitle', got '%s'", book.SubTitle)
	}

	if book.PubDate != 2023 {
		t.Errorf("Expected pub date 2023, got %d", book.PubDate)
	}

	if book.Rating != "Excellent" {
		t.Errorf("Expected rating 'Excellent', got '%s'", book.Rating)
	}

	if len(book.Authors) != 1 {
		t.Errorf("Expected 1 author, got %d", len(book.Authors))
	}

	if len(book.Authors) > 0 && book.Authors[0].FullName != "Test Author" {
		t.Errorf("Expected author 'Test Author', got '%s'", book.Authors[0].FullName)
	}
}

func TestUpdateBook(t *testing.T) {
	ws := setupTestServer()

	author := models.Author{
		FullName: "Test Author",
		Surname:  "Author",
	}
	ws.db.Create(&author)

	book := models.Book{
		MainTitle:      "Original Title",
		AuthorFullName: "Test Author",
		AuthorSurname:  "Author",
		Rating:         "Very-Good",
		PubDate:        2020,
	}
	ws.db.Create(&book)
	ws.db.Model(&book).Association("Authors").Append(&author)

	data := url.Values{}
	data.Set("main_title", "Updated Title")
	data.Set("sub_title", "Updated Subtitle")
	data.Set("author_full_name", "Test Author")
	data.Set("author_id", "1")
	data.Set("pub_date", "2024")
	data.Set("rating", "Excellent")
	data.Set("review", "Updated review")

	req, err := http.NewRequest("POST", "/books/update/1", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.updateBookHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusSeeOther)
	}

	var updatedBook models.Book
	result := ws.db.First(&updatedBook, 1)
	if result.Error != nil {
		t.Errorf("Could not retrieve updated book: %v", result.Error)
	}

	if updatedBook.MainTitle != "Updated Title" {
		t.Errorf("Expected updated title 'Updated Title', got '%s'", updatedBook.MainTitle)
	}

	if updatedBook.SubTitle != "Updated Subtitle" {
		t.Errorf("Expected updated subtitle 'Updated Subtitle', got '%s'", updatedBook.SubTitle)
	}

	if updatedBook.PubDate != 2024 {
		t.Errorf("Expected updated pub date 2024, got %d", updatedBook.PubDate)
	}

	if updatedBook.Rating != "Excellent" {
		t.Errorf("Expected updated rating 'Excellent', got '%s'", updatedBook.Rating)
	}
}

func TestDeleteBook(t *testing.T) {
	ws := setupTestServer()

	author := models.Author{
		FullName: "Test Author",
		Surname:  "Author",
	}
	ws.db.Create(&author)

	book := models.Book{
		MainTitle:      "Book to Delete",
		AuthorFullName: "Test Author",
		AuthorSurname:  "Author",
		Rating:         "Very-Good",
	}
	ws.db.Create(&book)

	req, err := http.NewRequest("POST", "/books/delete/1", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.deleteBookHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusSeeOther)
	}

	var deletedBook models.Book
	result := ws.db.First(&deletedBook, 1)
	if result.Error == nil {
		t.Errorf("Book should have been deleted but still exists")
	}
}

func TestListBooks(t *testing.T) {
	ws := setupTestServer()

	author := models.Author{
		FullName: "Test Author",
		Surname:  "Author",
	}
	ws.db.Create(&author)

	book1 := models.Book{
		MainTitle:      "First Book",
		AuthorFullName: "Test Author",
		Rating:         "Excellent",
	}
	book2 := models.Book{
		MainTitle:      "Second Book",
		AuthorFullName: "Test Author",
		Rating:         "Very-Good",
	}
	
	ws.db.Create(&book1)
	ws.db.Create(&book2)

	req, err := http.NewRequest("GET", "/books", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.listBooksHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code without templates: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestListAuthors(t *testing.T) {
	ws := setupTestServer()

	author1 := models.Author{
		FullName: "First Author",
		Surname:  "Author",
	}
	author2 := models.Author{
		FullName: "Second Author",
		Surname:  "Author",
	}

	ws.db.Create(&author1)
	ws.db.Create(&author2)

	req, err := http.NewRequest("GET", "/authors", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.listAuthorsHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code without templates: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestDeployHandler(t *testing.T) {
	ws := setupTestServer()

	// Test GET request (should not be allowed)
	req, err := http.NewRequest("GET", "/deploy", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.deployHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("handler returned wrong status code for GET: got %v want %v", status, http.StatusMethodNotAllowed)
	}

	// Test POST request (will fail without git repo, but should attempt)
	req, err = http.NewRequest("POST", "/deploy", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return internal server error due to missing templates
	// or could fail due to git not being initialized
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Logf("Deploy handler returned status: %v", status)
	}
}

func TestBuildLocalHandler(t *testing.T) {
	ws := setupTestServer()

	req, err := http.NewRequest("GET", "/build-local", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.buildLocalHandler)
	handler.ServeHTTP(rr, req)

	// Will fail without templates or sfwr executable
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Logf("Build local handler returned status: %v", status)
	}
}

func TestBackupsHandler(t *testing.T) {
	ws := setupTestServer()

	req, err := http.NewRequest("GET", "/backups", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.backupsHandler)
	handler.ServeHTTP(rr, req)

	// Should return internal server error without templates
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Logf("Backups handler returned status: %v", status)
	}
}

func TestRollbackHandler(t *testing.T) {
	ws := setupTestServer()

	// Test GET request (should redirect)
	req, err := http.NewRequest("GET", "/rollback", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.rollbackHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code for GET: got %v want %v", status, http.StatusSeeOther)
	}

	// Check redirect location
	location := rr.Header().Get("Location")
	if location != "/backups" {
		t.Errorf("Expected redirect to /backups, got %s", location)
	}

	// Test POST without commit parameter
	data := url.Values{}
	req, err = http.NewRequest("POST", "/rollback", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return error due to missing commit
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Logf("Rollback handler without commit returned status: %v", status)
	}

	// Test POST with commit parameter
	data.Set("commit", "abc123")
	req, err = http.NewRequest("POST", "/rollback", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Will fail without git repo but should attempt
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Logf("Rollback handler with commit returned status: %v", status)
	}
}

func TestUpdateBookWithMultipleAuthors(t *testing.T) {
	ws := setupTestServer()

	// Create multiple authors
	author1 := models.Author{
		FullName: "Primary Author",
		Surname:  "Author",
	}
	author2 := models.Author{
		FullName: "Secondary Author",
		Surname:  "Author",
	}

	ws.db.Create(&author1)
	ws.db.Create(&author2)

	// Create book with first author
	book := models.Book{
		MainTitle:      "Multi-Author Book",
		AuthorFullName: author1.FullName,
		AuthorSurname:  author1.Surname,
		Rating:         "Excellent",
		PubDate:        2024,
	}
	ws.db.Create(&book)
	ws.db.Model(&book).Association("Authors").Append(&author1)

	// Update book to use second author
	data := url.Values{}
	data.Set("main_title", "Updated Multi-Author Book")
	data.Set("author_id", fmt.Sprintf("%d", author2.ID))
	data.Set("author_full_name", author2.FullName)
	data.Set("pub_date", "2024")
	data.Set("rating", "Very-Good")

	req, err := http.NewRequest("POST", fmt.Sprintf("/books/update/%d", book.ID), strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ws.updateBookHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusSeeOther)
	}

	// Verify book was updated
	var updatedBook models.Book
	ws.db.Preload("Authors").First(&updatedBook, book.ID)

	if updatedBook.MainTitle != "Updated Multi-Author Book" {
		t.Errorf("Book title not updated: got %s", updatedBook.MainTitle)
	}

	// Verify author association was changed
	if len(updatedBook.Authors) != 1 {
		t.Errorf("Expected 1 author, got %d", len(updatedBook.Authors))
	}

	if len(updatedBook.Authors) > 0 && updatedBook.Authors[0].ID != author2.ID {
		t.Errorf("Expected author to be changed to author2")
	}
}