package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccdavis/sfwr/models"
	"gorm.io/gorm"
)

type WebServer struct {
	db          *gorm.DB
	templates   *template.Template
	imageDir    string
}

type PageData struct {
	Title   string
	Books   []models.Book
	Authors []models.Author
	Book    *models.Book
	Author  *models.Author
	Message string
	Error   string
	SortBy  string
}

func NewWebServer(db *gorm.DB, imageDir string) *WebServer {
	ws := &WebServer{
		db:       db,
		imageDir: imageDir,
	}
	ws.loadTemplates()
	return ws
}

func (ws *WebServer) loadTemplates() {
	var err error
	ws.templates, err = template.ParseGlob("templates/web/*.html")
	if err != nil {
		panic(fmt.Sprintf("Failed to load web templates: %v", err))
	}
}

func (ws *WebServer) ServeHTTP(port string) error {
	http.HandleFunc("/", ws.homeHandler)
	http.HandleFunc("/books", ws.listBooksHandler)
	http.HandleFunc("/books/new", ws.newBookHandler)
	http.HandleFunc("/books/create", ws.createBookHandler)
	http.HandleFunc("/books/edit/", ws.editBookHandler)
	http.HandleFunc("/books/update/", ws.updateBookHandler)
	http.HandleFunc("/books/delete/", ws.deleteBookHandler)
	http.HandleFunc("/authors", ws.listAuthorsHandler)
	http.HandleFunc("/authors/new", ws.newAuthorHandler)
	http.HandleFunc("/authors/create", ws.createAuthorHandler)
	http.HandleFunc("/books/search-openlibrary", ws.searchOpenLibraryHandler)
	http.HandleFunc("/books/update-from-openlibrary", ws.updateFromOpenLibraryHandler)
	http.HandleFunc("/books/create-from-openlibrary", ws.createFromOpenLibraryHandler)
	http.Handle("/saved_cover_images/", http.StripPrefix("/saved_cover_images/", http.FileServer(http.Dir(ws.imageDir))))

	fmt.Printf("Web server starting on http://localhost:%s\n", port)
	return http.ListenAndServe(":"+port, nil)
}

func (ws *WebServer) homeHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title:   "SFWR Book Management",
		Message: "Welcome to the SFWR Book Management System",
	}
	ws.renderTemplate(w, "home", data)
}

func (ws *WebServer) listBooksHandler(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "recent"
	}

	var books []models.Book
	var err error
	
	switch sortBy {
	case "recent":
		err = ws.db.Preload("Authors").Order("date_added DESC").Find(&books).Error
	case "title":
		err = ws.db.Preload("Authors").Order("main_title ASC").Find(&books).Error
	case "author":
		err = ws.db.Preload("Authors").Order("author_full_name ASC").Find(&books).Error
	case "year":
		err = ws.db.Preload("Authors").Order("pub_date DESC").Find(&books).Error
	default:
		err = ws.db.Preload("Authors").Order("date_added DESC").Find(&books).Error
	}

	if err != nil {
		ws.renderError(w, "Failed to load books", err)
		return
	}

	data := PageData{
		Title:  "All Books",
		Books:  books,
		SortBy: sortBy,
	}
	ws.renderTemplate(w, "book_list", data)
}

func (ws *WebServer) newBookHandler(w http.ResponseWriter, r *http.Request) {
	var authors []models.Author
	result := ws.db.Order("full_name ASC").Find(&authors)
	if result.Error != nil {
		ws.renderError(w, "Failed to load authors", result.Error)
		return
	}

	data := PageData{
		Title:   "Add New Book",
		Authors: authors,
	}
	ws.renderTemplate(w, "book_form", data)
}

func (ws *WebServer) createBookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/books/new", http.StatusSeeOther)
		return
	}

	authorID, err := strconv.ParseUint(r.FormValue("author_id"), 10, 32)
	if err != nil {
		ws.renderError(w, "Invalid author selected", err)
		return
	}

	var author models.Author
	if err := ws.db.First(&author, authorID).Error; err != nil {
		ws.renderError(w, "Author not found", err)
		return
	}

	book := models.Book{
		MainTitle:      r.FormValue("main_title"),
		SubTitle:       r.FormValue("sub_title"),
		AuthorFullName: author.FullName,
		AuthorSurname:  author.Surname,
		Rating:         r.FormValue("rating"),
		Review:         r.FormValue("review"),
		DateAdded:      time.Now(),
	}

	pubYear, err := strconv.ParseInt(r.FormValue("pub_date"), 10, 64)
	if err != nil {
		book.PubDate = models.Missing
	} else {
		book.PubDate = pubYear
	}

	result := ws.db.Create(&book)
	if result.Error != nil {
		ws.renderError(w, "Failed to create book", result.Error)
		return
	}

	ws.db.Model(&book).Association("Authors").Append(&author)

	http.Redirect(w, r, fmt.Sprintf("/books/edit/%d?message=Book created successfully", book.ID), http.StatusSeeOther)
}

func (ws *WebServer) editBookHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/books/edit/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderError(w, "Invalid book ID", err)
		return
	}

	var book models.Book
	if err := ws.db.Preload("Authors").First(&book, id).Error; err != nil {
		ws.renderError(w, "Book not found", err)
		return
	}

	var authors []models.Author
	result := ws.db.Order("full_name ASC").Find(&authors)
	if result.Error != nil {
		ws.renderError(w, "Failed to load authors", result.Error)
		return
	}

	data := PageData{
		Title:   "Edit Book",
		Book:    &book,
		Authors: authors,
		Message: r.URL.Query().Get("message"),
	}
	ws.renderTemplate(w, "book_form", data)
}

func (ws *WebServer) updateBookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/books", http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/books/update/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderError(w, "Invalid book ID", err)
		return
	}

	var book models.Book
	if err := ws.db.Preload("Authors").First(&book, id).Error; err != nil {
		ws.renderError(w, "Book not found", err)
		return
	}

	authorID, err := strconv.ParseUint(r.FormValue("author_id"), 10, 32)
	if err != nil {
		ws.renderError(w, "Invalid author selected", err)
		return
	}

	var author models.Author
	if err := ws.db.First(&author, authorID).Error; err != nil {
		ws.renderError(w, "Author not found", err)
		return
	}

	book.MainTitle = r.FormValue("main_title")
	book.SubTitle = r.FormValue("sub_title")
	book.AuthorFullName = author.FullName
	book.AuthorSurname = author.Surname
	book.Rating = r.FormValue("rating")
	book.Review = r.FormValue("review")

	pubYear, err := strconv.ParseInt(r.FormValue("pub_date"), 10, 64)
	if err != nil {
		book.PubDate = models.Missing
	} else {
		book.PubDate = pubYear
	}

	result := ws.db.Save(&book)
	if result.Error != nil {
		ws.renderError(w, "Failed to update book", result.Error)
		return
	}

	ws.db.Model(&book).Association("Authors").Replace(&author)

	http.Redirect(w, r, fmt.Sprintf("/books/edit/%d?message=Book updated successfully", book.ID), http.StatusSeeOther)
}

func (ws *WebServer) deleteBookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/books", http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/books/delete/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderError(w, "Invalid book ID", err)
		return
	}

	result := ws.db.Delete(&models.Book{}, id)
	if result.Error != nil {
		ws.renderError(w, "Failed to delete book", result.Error)
		return
	}

	http.Redirect(w, r, "/books?message=Book deleted successfully", http.StatusSeeOther)
}

func (ws *WebServer) listAuthorsHandler(w http.ResponseWriter, r *http.Request) {
	var authors []models.Author
	result := ws.db.Preload("Books").Find(&authors)
	if result.Error != nil {
		ws.renderError(w, "Failed to load authors", result.Error)
		return
	}

	data := PageData{
		Title:   "All Authors",
		Authors: authors,
		Message: r.URL.Query().Get("message"),
	}
	ws.renderTemplate(w, "author_list", data)
}

func (ws *WebServer) newAuthorHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title: "Add New Author",
	}
	ws.renderTemplate(w, "author_form", data)
}

func (ws *WebServer) createAuthorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/authors/new", http.StatusSeeOther)
		return
	}

	fullName := strings.TrimSpace(r.FormValue("full_name"))
	if fullName == "" {
		ws.renderError(w, "Author name is required", fmt.Errorf("empty author name"))
		return
	}

	author := models.Author{
		FullName: fullName,
		Surname:  models.ExtractSurname(fullName),
	}

	result := ws.db.Create(&author)
	if result.Error != nil {
		ws.renderError(w, "Failed to create author", result.Error)
		return
	}

	http.Redirect(w, r, "/authors?message=Author created successfully", http.StatusSeeOther)
}

func (ws *WebServer) renderTemplate(w http.ResponseWriter, name string, data PageData) {
	if ws.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}
	err := ws.templates.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template error for %s: %v", name, err), http.StatusInternalServerError)
	}
}

func (ws *WebServer) renderError(w http.ResponseWriter, message string, err error) {
	data := PageData{
		Title: "Error",
		Error: fmt.Sprintf("%s: %v", message, err),
	}
	ws.renderTemplate(w, "error", data)
}

// OpenLibrary API request/response structures
type SearchRequest struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	BookID uint   `json:"bookId"`
}

type SearchResponse struct {
	Results []SearchResultItem `json:"results,omitempty"`
	Error   string            `json:"error,omitempty"`
}

type SearchResultItem struct {
	Title              string   `json:"title"`
	Authors            []string `json:"authors"`
	FirstYearPublished int      `json:"first_year_published"`
	CoverEditionKey    string   `json:"cover_edition_key"`
	CoverImageID       string   `json:"cover_image_id"`
	CoverURL           string   `json:"cover_url"`
	Number             int      `json:"number"`
}

type UpdateRequest struct {
	BookID         uint             `json:"bookId"`
	SelectedResult SearchResultItem `json:"selectedResult"`
}

type CreateRequest struct {
	AuthorID       uint             `json:"authorId"`
	Rating         string           `json:"rating"`
	Review         string           `json:"review"`
	SelectedResult SearchResultItem `json:"selectedResult"`
}

type UpdateResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (ws *WebServer) searchOpenLibraryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Title == "" || req.Author == "" {
		ws.writeJSONError(w, "Title and author are required", http.StatusBadRequest)
		return
	}

	// Search Open Library using the existing model function
	searchResults := models.SearchBook(req.Title, req.Author)
	
	// Convert to response format and add cover URLs
	var responseItems []SearchResultItem
	for _, result := range searchResults {
		item := SearchResultItem{
			Title:              result.Title,
			Authors:            result.Authors,
			FirstYearPublished: result.FirstYearPublished,
			CoverEditionKey:    result.CoverEditionKey,
			CoverImageID:       result.CoverImageId,
			Number:             result.Number,
		}
		
		// Generate cover URL for small images
		if result.CoverEditionKey != "" {
			item.CoverURL = result.GetBookCoverUrl("S")
		}
		
		responseItems = append(responseItems, item)
	}

	response := SearchResponse{
		Results: responseItems,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) updateFromOpenLibraryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.BookID == 0 {
		ws.writeJSONError(w, "Book ID is required", http.StatusBadRequest)
		return
	}

	// Find the book
	var book models.Book
	if err := ws.db.First(&book, req.BookID).Error; err != nil {
		ws.writeJSONError(w, "Book not found", http.StatusNotFound)
		return
	}

	// Convert web result back to models.BookSearchResult format
	olResult := models.BookSearchResult{
		Number:             req.SelectedResult.Number,
		FirstYearPublished: req.SelectedResult.FirstYearPublished,
		Title:              req.SelectedResult.Title,
		Authors:            req.SelectedResult.Authors,
		CoverEditionKey:    req.SelectedResult.CoverEditionKey,
		CoverImageId:       req.SelectedResult.CoverImageID,
	}

	// Update the book using the existing model method
	updatedBook, err := book.UpdateFromOpenLibrary(ws.db, olResult)
	if err != nil {
		ws.writeJSONError(w, fmt.Sprintf("Failed to update book: %v", err), http.StatusInternalServerError)
		return
	}

	// Download cover images if we have the necessary data
	if updatedBook.HasCoverImageId() {
		go func() {
			// Run in background to avoid blocking the response
			models.CaptureAllSizeCovers(updatedBook, ws.imageDir)
		}()
	}

	response := UpdateResponse{
		Success: true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) createFromOpenLibraryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.AuthorID == 0 || req.Rating == "" {
		ws.writeJSONError(w, "Author and rating are required", http.StatusBadRequest)
		return
	}

	// Find the author
	var author models.Author
	if err := ws.db.First(&author, req.AuthorID).Error; err != nil {
		ws.writeJSONError(w, "Author not found", http.StatusNotFound)
		return
	}

	// Create the book with Open Library data
	book := models.Book{
		MainTitle:      req.SelectedResult.Title,
		AuthorFullName: author.FullName,
		AuthorSurname:  author.Surname,
		Rating:         req.Rating,
		Review:         req.Review,
		DateAdded:      time.Now(),
	}

	// Set cover ID if available
	if req.SelectedResult.CoverImageID != "" {
		if coverId, err := strconv.ParseInt(req.SelectedResult.CoverImageID, 10, 64); err == nil {
			book.OlCoverId = coverId
		}
	}

	// Set publication date if available
	if req.SelectedResult.FirstYearPublished > 0 {
		book.PubDate = int64(req.SelectedResult.FirstYearPublished)
	} else {
		book.PubDate = models.Missing
	}

	// Create the book
	result := ws.db.Create(&book)
	if result.Error != nil {
		ws.writeJSONError(w, fmt.Sprintf("Failed to create book: %v", result.Error), http.StatusInternalServerError)
		return
	}

	// Associate with author
	ws.db.Model(&book).Association("Authors").Append(&author)

	// Convert web result to models.BookSearchResult format for Open Library update
	olResult := models.BookSearchResult{
		Number:             req.SelectedResult.Number,
		FirstYearPublished: req.SelectedResult.FirstYearPublished,
		Title:              req.SelectedResult.Title,
		Authors:            req.SelectedResult.Authors,
		CoverEditionKey:    req.SelectedResult.CoverEditionKey,
		CoverImageId:       req.SelectedResult.CoverImageID,
	}

	// Update the book with Open Library metadata
	updatedBook, err := book.UpdateFromOpenLibrary(ws.db, olResult)
	if err != nil {
		// Log error but don't fail the creation since the book was already created
		fmt.Printf("Warning: Failed to update book with Open Library data: %v\n", err)
	}

	// Download cover images if we have the necessary data
	if updatedBook.HasCoverImageId() {
		go func() {
			// Run in background to avoid blocking the response
			models.CaptureAllSizeCovers(updatedBook, ws.imageDir)
		}()
	}

	response := map[string]interface{}{
		"success": true,
		"bookId":  book.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]string{"error": message}
	json.NewEncoder(w).Encode(response)
}