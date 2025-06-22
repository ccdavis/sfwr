package web

import (
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
	books, err := models.LoadAllBooks(ws.db)
	if err != nil {
		ws.renderError(w, "Failed to load books", err)
		return
	}

	data := PageData{
		Title: "All Books",
		Books: books,
	}
	ws.renderTemplate(w, "book_list", data)
}

func (ws *WebServer) newBookHandler(w http.ResponseWriter, r *http.Request) {
	var authors []models.Author
	result := ws.db.Find(&authors)
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

	book := models.Book{
		MainTitle:      r.FormValue("main_title"),
		SubTitle:       r.FormValue("sub_title"),
		AuthorFullName: r.FormValue("author_full_name"),
		AuthorSurname:  models.ExtractSurname(r.FormValue("author_full_name")),
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
	result := ws.db.Find(&authors)
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

	book.MainTitle = r.FormValue("main_title")
	book.SubTitle = r.FormValue("sub_title")
	book.AuthorFullName = r.FormValue("author_full_name")
	book.AuthorSurname = models.ExtractSurname(r.FormValue("author_full_name"))
	book.Rating = r.FormValue("rating")
	book.Review = r.FormValue("review")

	pubYear, err := strconv.ParseInt(r.FormValue("pub_date"), 10, 64)
	if err != nil {
		book.PubDate = models.Missing
	} else {
		book.PubDate = pubYear
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (ws *WebServer) renderError(w http.ResponseWriter, message string, err error) {
	data := PageData{
		Title: "Error",
		Error: fmt.Sprintf("%s: %v", message, err),
	}
	ws.renderTemplate(w, "error", data)
}