package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
	"gorm.io/gorm"
)

type WebServer struct {
	db              *gorm.DB
	templates       map[string]*template.Template
	previewTemplate *template.Template
	imageDir        string
	config          Config
	sessions        *sessionStore
	loginThrottle   *loginThrottle
}

// pageTemplates lists every named template rendered via renderTemplate; each
// is parsed once with base.html at server startup.
var pageTemplates = []string{
	"home",
	"book_list",
	"book_form",
	"author_list",
	"author_form",
	"author_edit",
	"error",
	"decades",
	"decade",
	"backups",
	"login",
}

// templateFuncs are the helpers available to every admin template. "url"
// is how templates emit internal links: it applies the mount point, so a
// page never has to know whether the UI is served from / or /admin.
func (ws *WebServer) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"shortHash": shortHash,
		"url":       ws.config.URL,
	}
}

// booksPerPage caps how many books one listing page renders. The full
// collection is several hundred books; rendering them all made the page
// slow to load and impossible to scan.
const booksPerPage = 50

type PageData struct {
	Title   string
	Books   []models.Book
	Authors []models.Author
	Book    *models.Book
	Author  *models.Author
	Decades []pages.DecadeInfo
	Decade  *pages.DecadeInfo
	Message string
	Error   string
	SortBy  string
	Commits []GitCommit

	// Active names the nav entry to highlight for the current page.
	Active string

	// Listing search and pagination state.
	Query      string
	Total      int
	Page       int
	TotalPages int
	PrevURL    string
	NextURL    string

	// Per-request state filled in by renderTemplateStatus. CSRFToken must
	// appear in every form that posts.
	CSRFToken     string
	CSRFFieldName string
	Authenticated bool
	AuthEnabled   bool
	LocalRequest  bool
	HideNav       bool
	NextPath      string

	// Deployment context, so the UI can describe where things go and hide
	// controls that are not available.
	SiteName      string
	OutputDir     string
	ConfigFile    string
	DeployEnabled bool

	// BasePath is the mount point, for building URLs in JavaScript.
	BasePath string
}

func NewWebServer(db *gorm.DB, config Config) (*WebServer, error) {
	ws := &WebServer{
		db:            db,
		imageDir:      config.CoverImagesDir,
		config:        config,
		sessions:      newSessionStore(),
		loginThrottle: newLoginThrottle(),
		templates:     make(map[string]*template.Template, len(pageTemplates)),
	}

	adminTemplates := config.AdminTemplates()
	base := filepath.Join(adminTemplates, "base.html")
	for _, name := range pageTemplates {
		t, err := template.New(name+".html").Funcs(ws.templateFuncs()).
			ParseFiles(base, filepath.Join(adminTemplates, name+".html"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %q: %w", name, err)
		}
		ws.templates[name] = t
	}
	previewTmpl, err := template.New("preview.html").Funcs(ws.templateFuncs()).
		ParseFiles(filepath.Join(adminTemplates, "preview.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse preview template: %w", err)
	}
	ws.previewTemplate = previewTmpl
	return ws, nil
}

// ServeHTTP starts the admin server on the configured address.
func (ws *WebServer) ServeHTTP() error {
	if err := ws.config.Validate(); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.homeHandler)
	mux.HandleFunc("/books", ws.listBooksHandler)
	mux.HandleFunc("/books/new", ws.newBookHandler)
	mux.HandleFunc("/books/create", ws.createBookHandler)
	mux.HandleFunc("/books/edit/", ws.editBookHandler)
	mux.HandleFunc("/books/update/", ws.updateBookHandler)
	mux.HandleFunc("/books/delete/", ws.deleteBookHandler)
	mux.HandleFunc("/authors", ws.listAuthorsHandler)
	mux.HandleFunc("/authors/new", ws.newAuthorHandler)
	mux.HandleFunc("/authors/create", ws.createAuthorHandler)
	mux.HandleFunc("/authors/edit/", ws.editAuthorHandler)
	mux.HandleFunc("/authors/update/", ws.updateAuthorHandler)
	mux.HandleFunc("/authors/delete/", ws.deleteAuthorHandler)
	mux.HandleFunc("/decades", ws.listDecadesHandler)
	mux.HandleFunc("/decades/", ws.decadeHandler)
	mux.HandleFunc("/books/search-openlibrary", ws.searchOpenLibraryHandler)
	mux.HandleFunc("/books/update-from-openlibrary", ws.updateFromOpenLibraryHandler)
	mux.HandleFunc("/books/create-from-openlibrary", ws.createFromOpenLibraryHandler)
	mux.HandleFunc("/preview", ws.previewHandler)
	mux.HandleFunc("/logout", ws.logoutHandler)

	// Rebuilding the site and deploying are both recoverable, so any signed-in
	// session may run them — that is the point of hosting this UI.
	mux.HandleFunc("/build-local", ws.buildLocalHandler)
	mux.HandleFunc("/deploy", ws.deployHandler)
	mux.HandleFunc("/backups", ws.backupsHandler)

	// Rollback replaces the live database with an older copy and cannot be
	// undone from the UI, so it stays on the console.
	mux.Handle("/rollback", ws.requireLocal(http.HandlerFunc(ws.rollbackHandler)))

	mux.Handle("/saved_cover_images/", http.StripPrefix("/saved_cover_images/", http.FileServer(http.Dir(ws.imageDir))))
	mux.Handle("/preview-site/", http.StripPrefix("/preview-site/", http.FileServer(http.Dir(ws.config.OutputDir))))

	// Everything above sits behind the session gate; /login is the only
	// route reachable without one.
	root := http.NewServeMux()
	root.HandleFunc("/login", ws.loginHandler)
	root.Handle("/", ws.authenticate(mux))

	handler := securityHeaders(noStore(ws.mount(root)))

	ws.startSessionJanitor()
	ws.logStartup()

	server := &http.Server{
		Addr:              ws.config.ListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

// mount serves the whole application under Config.BasePath, stripping the
// prefix so every handler and route pattern keeps working in terms of plain
// paths like "/books". Requests outside the mount get a 404 rather than
// being silently served, which would otherwise hide a misconfigured proxy.
func (ws *WebServer) mount(handler http.Handler) http.Handler {
	base := ws.config.BasePath
	if base == "" {
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "/admin" with no trailing slash is what a person types; send it
		// to "/admin/" so relative resolution works from there on.
		if r.URL.Path == base {
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
			return
		}
		if !strings.HasPrefix(r.URL.Path, base+"/") {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix(base, handler).ServeHTTP(w, r)
	})
}

// logStartup prints where the server is listening and how it is protected,
// so an unauthenticated or plaintext setup is obvious in the console.
func (ws *WebServer) logStartup() {
	fmt.Printf("SFWR admin listening on http://%s%s/\n", ws.config.ListenAddr(), ws.config.BasePath)
	if ws.config.BehindProxy {
		fmt.Println("Proxy mode: TLS terminated upstream; cookies marked Secure")
	}

	if ws.config.AuthEnabled() {
		fmt.Println("Authentication: password required")
	} else {
		fmt.Printf("Authentication: DISABLED (loopback only). Set %s to require a password.\n", PasswordHashEnvVar)
	}
	if isLoopbackAddr(ws.config.BindAddr) {
		fmt.Println("Reachable from: this machine only")
	} else {
		fmt.Printf("Reachable from: any host that can reach %s\n", ws.config.BindAddr)
		fmt.Println("Rollback is restricted to browsers running on this machine.")
	}

	fmt.Printf("Publishing to: %s\n", ws.config.OutputDir)
	if ws.config.DeployEnabled() {
		fmt.Printf("Git working tree: %s\n", ws.config.RepoDir)
	} else {
		fmt.Println("Git working tree: not configured (deploy and rollback disabled)")
	}
	if ws.config.ConfigFile != "" {
		fmt.Printf("Settings: %s\n", ws.config.ConfigFile)
	}
}

func (ws *WebServer) homeHandler(w http.ResponseWriter, r *http.Request) {
	// "/" is registered as the catch-all pattern, so anything that didn't
	// match a real route lands here. Report it as missing instead of
	// silently serving the home page with a 200.
	if r.URL.Path != "/" {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Page not found",
			fmt.Errorf("no page at %s", r.URL.Path))
		return
	}

	var bookCount int64
	ws.db.Model(&models.Book{}).Count(&bookCount)

	data := PageData{
		Title:  ws.config.SiteName + " Book Management",
		Active: "home",
		Total:  int(bookCount),
	}
	ws.renderTemplate(w, r, "home", data)
}

// bookSortOrder maps a sort key from the UI to a SQL ORDER BY clause.
// Author sorting uses the surname so the list reads like a library shelf
// rather than being grouped by first name.
var bookSortOrder = map[string]string{
	"recent": "date_added DESC",
	"title":  "main_title ASC",
	"author": "author_surname ASC, author_full_name ASC, pub_date ASC",
	"year":   "pub_date DESC",
}

func (ws *WebServer) listBooksHandler(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort")
	order, ok := bookSortOrder[sortBy]
	if !ok {
		sortBy = "recent"
		order = bookSortOrder["recent"]
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Session() makes the filter reusable: Count is a finisher, and without
	// a new session its conditions would leak into the Find below.
	tx := ws.db.Model(&models.Book{})
	if query != "" {
		like := "%" + strings.ToLower(query) + "%"
		tx = tx.Where(
			"LOWER(main_title) LIKE ? OR LOWER(sub_title) LIKE ? OR LOWER(author_full_name) LIKE ?",
			like, like, like)
	}
	tx = tx.Session(&gorm.Session{})

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		ws.renderError(w, r, "Failed to count books", err)
		return
	}

	totalPages := int((total + booksPerPage - 1) / booksPerPage)
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		page = p
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}

	var books []models.Book
	err := tx.Preload("Authors").
		Order(order).
		Limit(booksPerPage).
		Offset((page - 1) * booksPerPage).
		Find(&books).Error
	if err != nil {
		ws.renderError(w, r, "Failed to load books", err)
		return
	}

	data := PageData{
		Title:      "All Books",
		Active:     "books",
		Books:      books,
		SortBy:     sortBy,
		Query:      query,
		Total:      int(total),
		Page:       page,
		TotalPages: totalPages,
		Message:    r.URL.Query().Get("message"),
	}
	if page > 1 {
		data.PrevURL = ws.booksPageURL(query, sortBy, page-1)
	}
	if page < totalPages {
		data.NextURL = ws.booksPageURL(query, sortBy, page+1)
	}
	ws.renderTemplate(w, r, "book_list", data)
}

// booksPageURL builds a /books link that preserves the active search and
// sort while changing the page.
func (ws *WebServer) booksPageURL(query, sortBy string, page int) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	values.Set("sort", sortBy)
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	return ws.config.URL("/books?" + values.Encode())
}

func (ws *WebServer) newBookHandler(w http.ResponseWriter, r *http.Request) {
	var authors []models.Author
	result := ws.db.Order("full_name ASC").Find(&authors)
	if result.Error != nil {
		ws.renderError(w, r, "Failed to load authors", result.Error)
		return
	}

	data := PageData{
		Title:   "Add New Book",
		Active:  "book-new",
		Authors: authors,
	}
	ws.renderTemplate(w, r, "book_form", data)
}

func (ws *WebServer) createBookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, ws.config.URL("/books/new"), http.StatusSeeOther)
		return
	}

	authorID, err := strconv.ParseUint(r.FormValue("author_id"), 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid author selected", err)
		return
	}

	var author models.Author
	if err := ws.db.First(&author, authorID).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Author not found", err)
		return
	}

	rating := r.FormValue("rating")
	if !validRating(rating) {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid rating",
			fmt.Errorf("%q is not a valid rating", rating))
		return
	}

	mainTitle := strings.TrimSpace(r.FormValue("main_title"))
	if mainTitle == "" {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Book title is required",
			fmt.Errorf("empty main title"))
		return
	}

	book := models.Book{
		MainTitle:      mainTitle,
		SubTitle:       strings.TrimSpace(r.FormValue("sub_title")),
		AuthorFullName: author.FullName,
		AuthorSurname:  author.Surname,
		Rating:         rating,
		Indy:           r.FormValue("indy") == "on",
		Interesting:    r.FormValue("interesting") == "on",
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
		ws.renderError(w, r, "Failed to create book", result.Error)
		return
	}

	if err := ws.db.Model(&book).Association("Authors").Append(&author); err != nil {
		ws.renderError(w, r, "Failed to link author to book", err)
		return
	}

	http.Redirect(w, r, ws.config.URL(fmt.Sprintf("/books/edit/%d?message=Book created successfully", book.ID)), http.StatusSeeOther)
}

func (ws *WebServer) editBookHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/books/edit/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid book ID", err)
		return
	}

	var book models.Book
	if err := ws.db.Preload("Authors").First(&book, id).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Book not found", err)
		return
	}

	var authors []models.Author
	result := ws.db.Order("full_name ASC").Find(&authors)
	if result.Error != nil {
		ws.renderError(w, r, "Failed to load authors", result.Error)
		return
	}

	data := PageData{
		Title:   "Edit Book",
		Active:  "books",
		Book:    &book,
		Authors: authors,
		Message: r.URL.Query().Get("message"),
	}
	ws.renderTemplate(w, r, "book_form", data)
}

func (ws *WebServer) updateBookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, ws.config.URL("/books"), http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/books/update/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid book ID", err)
		return
	}

	var book models.Book
	if err := ws.db.Preload("Authors").First(&book, id).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Book not found", err)
		return
	}

	authorID, err := strconv.ParseUint(r.FormValue("author_id"), 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid author selected", err)
		return
	}

	var author models.Author
	if err := ws.db.First(&author, authorID).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Author not found", err)
		return
	}

	rating := r.FormValue("rating")
	if !validRating(rating) {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid rating",
			fmt.Errorf("%q is not a valid rating", rating))
		return
	}

	mainTitle := strings.TrimSpace(r.FormValue("main_title"))
	if mainTitle == "" {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Book title is required",
			fmt.Errorf("empty main title"))
		return
	}

	book.MainTitle = mainTitle
	book.SubTitle = strings.TrimSpace(r.FormValue("sub_title"))
	book.AuthorFullName = author.FullName
	book.AuthorSurname = author.Surname
	book.Rating = rating
	book.Indy = r.FormValue("indy") == "on"
	book.Interesting = r.FormValue("interesting") == "on"
	book.Review = r.FormValue("review")

	pubYear, err := strconv.ParseInt(r.FormValue("pub_date"), 10, 64)
	if err != nil {
		book.PubDate = models.Missing
	} else {
		book.PubDate = pubYear
	}

	result := ws.db.Save(&book)
	if result.Error != nil {
		ws.renderError(w, r, "Failed to update book", result.Error)
		return
	}

	if err := ws.db.Model(&book).Association("Authors").Replace(&author); err != nil {
		ws.renderError(w, r, "Failed to link author to book", err)
		return
	}

	http.Redirect(w, r, ws.config.URL(fmt.Sprintf("/books/edit/%d?message=Book updated successfully", book.ID)), http.StatusSeeOther)
}

func (ws *WebServer) deleteBookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, ws.config.URL("/books"), http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/books/delete/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid book ID", err)
		return
	}

	var book models.Book
	if err := ws.db.First(&book, id).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Book not found", err)
		return
	}

	// Book deletes are soft (gorm.DeletedAt), so the book_authors rows are
	// not removed automatically and would keep pointing at a hidden book.
	if err := ws.db.Model(&book).Association("Authors").Clear(); err != nil {
		ws.renderError(w, r, "Failed to unlink authors from book", err)
		return
	}

	if err := ws.db.Delete(&book).Error; err != nil {
		ws.renderError(w, r, "Failed to delete book", err)
		return
	}

	http.Redirect(w, r, ws.config.URL("/books?message=Book deleted successfully"), http.StatusSeeOther)
}

func (ws *WebServer) listAuthorsHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	tx := ws.db.Preload("Books").Order("surname ASC, full_name ASC")
	if query != "" {
		like := "%" + strings.ToLower(query) + "%"
		tx = tx.Where("LOWER(full_name) LIKE ?", like)
	}

	var authors []models.Author
	if err := tx.Find(&authors).Error; err != nil {
		ws.renderError(w, r, "Failed to load authors", err)
		return
	}

	data := PageData{
		Title:   "All Authors",
		Active:  "authors",
		Authors: authors,
		Query:   query,
		Total:   len(authors),
		Message: r.URL.Query().Get("message"),
	}
	ws.renderTemplate(w, r, "author_list", data)
}

func (ws *WebServer) newAuthorHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		Title:  "Add New Author",
		Active: "author-new",
	}
	ws.renderTemplate(w, r, "author_form", data)
}

func (ws *WebServer) createAuthorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, ws.config.URL("/authors/new"), http.StatusSeeOther)
		return
	}

	fullName := strings.TrimSpace(r.FormValue("full_name"))
	if fullName == "" {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Author name is required",
			fmt.Errorf("empty author name"))
		return
	}

	// Author names are the key users search and select by, so a duplicate
	// makes the picker ambiguous and silently splits a writer's books.
	var existing models.Author
	if err := ws.db.Where("full_name = ?", fullName).First(&existing).Error; err == nil {
		http.Redirect(w, r,
			ws.config.URL(fmt.Sprintf("/authors/edit/%d?message=%s already exists", existing.ID, url.QueryEscape(fullName))),
			http.StatusSeeOther)
		return
	}

	author := models.Author{
		FullName: fullName,
		Surname:  models.ExtractSurname(fullName),
	}

	result := ws.db.Create(&author)
	if result.Error != nil {
		ws.renderError(w, r, "Failed to create author", result.Error)
		return
	}

	http.Redirect(w, r, ws.config.URL("/authors?message=Author created successfully"), http.StatusSeeOther)
}

func (ws *WebServer) editAuthorHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/authors/edit/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid author ID", err)
		return
	}

	var author models.Author
	if err := ws.db.Preload("Books").First(&author, id).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Author not found", err)
		return
	}

	// Get all books for reassignment
	var allBooks []models.Book
	if err := ws.db.Preload("Authors").Order("main_title ASC").Find(&allBooks).Error; err != nil {
		ws.renderError(w, r, "Failed to load books", err)
		return
	}

	// Get all authors except the current one for reassignment options
	var otherAuthors []models.Author
	if err := ws.db.Where("id != ?", id).Order("full_name ASC").Find(&otherAuthors).Error; err != nil {
		ws.renderError(w, r, "Failed to load authors", err)
		return
	}

	data := PageData{
		Title:   "Edit Author",
		Active:  "authors",
		Author:  &author,
		Books:   allBooks,
		Authors: otherAuthors,
		Message: r.URL.Query().Get("message"),
	}
	ws.renderTemplate(w, r, "author_edit", data)
}

func (ws *WebServer) deleteAuthorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, ws.config.URL("/authors"), http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/authors/delete/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid author ID", err)
		return
	}

	var author models.Author
	if err := ws.db.Preload("Books").First(&author, id).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Author not found", err)
		return
	}

	// Deleting an author who still has books would orphan them on the
	// static site; reassign the books first.
	if len(author.Books) > 0 {
		ws.renderErrorStatus(w, r, http.StatusConflict,
			fmt.Sprintf("Cannot delete %s", author.FullName),
			fmt.Errorf("%d book(s) are still assigned; reassign them first", len(author.Books)))
		return
	}

	if err := ws.db.Delete(&author).Error; err != nil {
		ws.renderError(w, r, "Failed to delete author", err)
		return
	}

	http.Redirect(w, r, ws.config.URL("/authors?message=Author deleted successfully"), http.StatusSeeOther)
}

func (ws *WebServer) updateAuthorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, ws.config.URL("/authors"), http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/authors/update/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid author ID", err)
		return
	}

	var author models.Author
	if err := ws.db.Preload("Books").First(&author, id).Error; err != nil {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Author not found", err)
		return
	}

	// Update author name
	fullName := strings.TrimSpace(r.FormValue("full_name"))
	if fullName == "" {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Author name is required",
			fmt.Errorf("empty author name"))
		return
	}

	author.FullName = fullName
	author.Surname = models.ExtractSurname(fullName)

	// Update all books with this author's name change
	for i := range author.Books {
		if err := ws.applyAuthorToBook(&author.Books[i], &author); err != nil {
			ws.renderError(w, r, "Failed to update book author", err)
			return
		}
	}

	// Save the author
	if err := ws.db.Save(&author).Error; err != nil {
		ws.renderError(w, r, "Failed to update author", err)
		return
	}

	// Handle book assignments/removals
	action := r.FormValue("book_action")
	switch action {
	case "add":
		// Add selected books to this author
		bookIDs := r.Form["add_book_ids"]
		if len(bookIDs) > 0 {
			var books []models.Book
			if err := ws.db.Find(&books, bookIDs).Error; err != nil {
				ws.renderError(w, r, "Failed to find books", err)
				return
			}
			for i := range books {
				if err := ws.db.Model(&author).Association("Books").Append(&books[i]); err != nil {
					ws.renderError(w, r, "Failed to assign book to author", err)
					return
				}
				if err := ws.applyAuthorToBook(&books[i], &author); err != nil {
					ws.renderError(w, r, "Failed to update book author", err)
					return
				}
			}
		}

	case "remove":
		// Remove selected books and reassign to another author
		bookIDs := r.Form["remove_book_ids"]
		newAuthorID := r.FormValue("new_author_id")

		if len(bookIDs) == 0 {
			ws.renderErrorStatus(w, r, http.StatusBadRequest, "No books selected",
				fmt.Errorf("select at least one book to reassign"))
			return
		}
		// Without a destination author the selected books would be
		// silently left where they are, which reads as a failed save.
		if newAuthorID == "" {
			ws.renderErrorStatus(w, r, http.StatusBadRequest, "No destination author chosen",
				fmt.Errorf("pick the author to reassign the selected books to"))
			return
		}

		newAuthorIDUint, err := strconv.ParseUint(newAuthorID, 10, 32)
		if err != nil {
			ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid new author ID", err)
			return
		}

		var newAuthor models.Author
		if err := ws.db.First(&newAuthor, newAuthorIDUint).Error; err != nil {
			ws.renderErrorStatus(w, r, http.StatusNotFound, "New author not found", err)
			return
		}

		var books []models.Book
		if err := ws.db.Find(&books, bookIDs).Error; err != nil {
			ws.renderError(w, r, "Failed to find books", err)
			return
		}

		for i := range books {
			if err := ws.db.Model(&author).Association("Books").Delete(&books[i]); err != nil {
				ws.renderError(w, r, "Failed to unassign book", err)
				return
			}
			if err := ws.db.Model(&newAuthor).Association("Books").Append(&books[i]); err != nil {
				ws.renderError(w, r, "Failed to reassign book", err)
				return
			}
			if err := ws.applyAuthorToBook(&books[i], &newAuthor); err != nil {
				ws.renderError(w, r, "Failed to update book author", err)
				return
			}
		}
	}

	http.Redirect(w, r, ws.config.URL(fmt.Sprintf("/authors/edit/%d?message=Author updated successfully", author.ID)), http.StatusSeeOther)
}

// captureCoversAsync runs image capture in the background for a single book
// and recovers from any panic so a background fetch can't crash the server.
func (ws *WebServer) captureCoversAsync(b models.Book) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("cover capture panic for '%s': %v", b.FormatTitle(), r)
			}
		}()
		models.CaptureAllCovers(b, ws.imageDir)
	}()
}

// applyAuthorToBook copies the author's denormalized name fields onto a book
// and persists the change. Used from every path that assigns or reassigns
// an author to a book.
func (ws *WebServer) applyAuthorToBook(book *models.Book, author *models.Author) error {
	book.AuthorFullName = author.FullName
	book.AuthorSurname = author.Surname
	return ws.db.Save(book).Error
}

func validRating(rating string) bool {
	_, err := models.StringToRating(rating)
	return err == nil
}

func (ws *WebServer) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	ws.renderTemplateStatus(w, r, http.StatusOK, name, data)
}

// renderTemplateStatus fills in the per-request fields every page needs —
// the CSRF token above all — so no handler can forget them, then renders.
func (ws *WebServer) renderTemplateStatus(w http.ResponseWriter, r *http.Request, status int, name string, data PageData) {
	ctx := contextFromRequest(r)
	data.CSRFToken = ctx.csrfToken
	data.Authenticated = ctx.authenticated
	data.LocalRequest = ctx.local
	data.AuthEnabled = ws.config.AuthEnabled()
	data.SiteName = ws.config.SiteName
	data.OutputDir = ws.config.OutputDir
	data.ConfigFile = ws.config.ConfigFile
	data.DeployEnabled = ws.config.DeployEnabled()
	data.BasePath = ws.config.BasePath
	ws.writeTemplate(w, status, name, data)
}

// writeTemplate renders into a buffer first so a template failure halfway
// through cannot leave a half-written page under a success status.
func (ws *WebServer) writeTemplate(w http.ResponseWriter, status int, name string, data PageData) {
	// Supplied here rather than in each template so the field name can only
	// be defined in one place.
	data.CSRFFieldName = csrfFormField

	tmpl, ok := ws.templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("Unknown template %q", name), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name+".html", data); err != nil {
		log.Printf("template error for %s: %v", name, err)
		http.Error(w, fmt.Sprintf("Template error for %s: %v", name, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("failed writing %s response: %v", name, err)
	}
}

func (ws *WebServer) renderError(w http.ResponseWriter, r *http.Request, message string, err error) {
	ws.renderErrorStatus(w, r, http.StatusInternalServerError, message, err)
}

// renderErrorStatus shows the error page with a status code that matches
// what went wrong, so failures are not reported to the browser as success.
func (ws *WebServer) renderErrorStatus(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	data := PageData{
		Title: "Error",
		Error: fmt.Sprintf("%s: %v", message, err),
	}
	ws.renderTemplateStatus(w, r, status, "error", data)
}

// OpenLibrary API request/response structures
type SearchRequest struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	BookID uint   `json:"bookId"`
}

type SearchResponse struct {
	Results []SearchResultItem `json:"results,omitempty"`
	Error   string             `json:"error,omitempty"`
}

type SearchResultItem struct {
	Title              string   `json:"title"`
	Authors            []string `json:"authors"`
	FirstYearPublished int      `json:"first_year_published"`
	CoverEditionKey    string   `json:"cover_edition_key"`
	CoverImageID       string   `json:"cover_image_id"`
	CoverURL           string   `json:"cover_url"`
	CoverBaseURL       string   `json:"cover_base_url,omitempty"`
	Source             string   `json:"source"`
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

// anyResultHasCover reports whether at least one search result came back
// with a usable cover image.
func anyResultHasCover(items []SearchResultItem) bool {
	for _, item := range items {
		if item.CoverURL != "" {
			return true
		}
	}
	return false
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

	// Search Open Library
	searchResults := models.SearchBook(req.Title, req.Author)

	var responseItems []SearchResultItem
	for _, result := range searchResults {
		item := SearchResultItem{
			Title:              result.Title,
			Authors:            result.Authors,
			FirstYearPublished: result.FirstYearPublished,
			CoverEditionKey:    result.CoverEditionKey,
			CoverImageID:       result.CoverImageId,
			Source:             "openlibrary",
			Number:             result.Number,
		}
		if result.CoverEditionKey != "" {
			item.CoverURL = result.GetBookCoverUrl("S")
		}
		responseItems = append(responseItems, item)
	}

	// Fall back to Google Books and iTunes when Open Library gave us
	// nothing, and also when none of its matches carry a cover — finding a
	// cover is the main reason to run this search.
	if !anyResultHasCover(responseItems) {
		gbResults, err := models.SearchGoogleBooks(req.Title, req.Author)
		if err == nil {
			for _, r := range gbResults {
				item := SearchResultItem{
					Title:              r.Title,
					Authors:            r.Authors,
					FirstYearPublished: r.PubYear,
					Source:             "googlebooks",
				}
				if r.HasCover() {
					item.CoverURL = models.GoogleBooksCoverURL(r.ThumbnailURL, models.SmallCover)
					item.CoverBaseURL = r.ThumbnailURL
				}
				responseItems = append(responseItems, item)
			}
		}

		itResults, err := models.SearchITunes(req.Title, req.Author)
		if err == nil {
			for _, r := range itResults {
				item := SearchResultItem{
					Title:              r.Title,
					Authors:            []string{r.Author},
					FirstYearPublished: r.PubYear,
					Source:             "itunes",
				}
				if r.HasCover() {
					item.CoverURL = models.ITunesCoverURL(r.ArtworkURL, models.SmallCover)
					item.CoverBaseURL = r.ArtworkURL
				}
				responseItems = append(responseItems, item)
			}
		}
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

	var updatedBook models.Book

	switch req.SelectedResult.Source {
	case "googlebooks", "itunes":
		book.CoverSource = req.SelectedResult.Source
		book.CoverImageUrl = req.SelectedResult.CoverBaseURL
		// Google Books and iTunes date the edition, not the work, so their year is
		// only taken when it could actually be a first publication date.
		if book.HasMissingPubDate() && book.PlausiblePubYear(req.SelectedResult.FirstYearPublished) {
			book.PubDate = int64(req.SelectedResult.FirstYearPublished)
		}
		if err := ws.db.Save(&book).Error; err != nil {
			ws.writeJSONError(w, fmt.Sprintf("Failed to update book: %v", err), http.StatusInternalServerError)
			return
		}
		updatedBook = book
	default:
		olResult := models.BookSearchResult{
			Number:             req.SelectedResult.Number,
			FirstYearPublished: req.SelectedResult.FirstYearPublished,
			Title:              req.SelectedResult.Title,
			Authors:            req.SelectedResult.Authors,
			CoverEditionKey:    req.SelectedResult.CoverEditionKey,
			CoverImageId:       req.SelectedResult.CoverImageID,
		}

		var err error
		updatedBook, err = book.UpdateFromOpenLibrary(ws.db, olResult)
		if err != nil {
			ws.writeJSONError(w, fmt.Sprintf("Failed to update book: %v", err), http.StatusInternalServerError)
			return
		}

		if updatedBook.HasCover() && updatedBook.CoverSource == "" {
			updatedBook.CoverSource = "openlibrary"
			ws.db.Save(&updatedBook)
		}

		if !updatedBook.HasCover() {
			refreshed, found, refreshErr := models.RefreshCover(ws.db, updatedBook)
			if refreshErr != nil {
				log.Printf("Cover fallback error for '%s': %v", updatedBook.FormatTitle(), refreshErr)
			}
			if found {
				updatedBook = refreshed
			}
		}
	}

	if updatedBook.HasMissingPubDate() {
		filled, fillErr := models.FillMissingPubDate(ws.db, updatedBook)
		if fillErr != nil {
			log.Printf("Pub date fallback error for '%s': %v", updatedBook.FormatTitle(), fillErr)
		} else {
			updatedBook = filled
		}
	}

	if updatedBook.HasCover() {
		ws.captureCoversAsync(updatedBook)
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

	// Set cover and source based on where the result came from
	switch req.SelectedResult.Source {
	case "googlebooks", "itunes":
		book.CoverSource = req.SelectedResult.Source
		book.CoverImageUrl = req.SelectedResult.CoverBaseURL
	default:
		if req.SelectedResult.CoverImageID != "" {
			if coverId, err := strconv.ParseInt(req.SelectedResult.CoverImageID, 10, 64); err == nil {
				book.OlCoverId = coverId
				book.CoverSource = "openlibrary"
			}
		}
	}

	if book.PlausiblePubYear(req.SelectedResult.FirstYearPublished) {
		book.PubDate = int64(req.SelectedResult.FirstYearPublished)
	} else {
		book.PubDate = models.Missing
	}

	result := ws.db.Create(&book)
	if result.Error != nil {
		ws.writeJSONError(w, fmt.Sprintf("Failed to create book: %v", result.Error), http.StatusInternalServerError)
		return
	}

	ws.db.Model(&book).Association("Authors").Append(&author)

	updatedBook := book

	// For Open Library results, fetch additional metadata from the OL edition
	if req.SelectedResult.Source == "" || req.SelectedResult.Source == "openlibrary" {
		olResult := models.BookSearchResult{
			Number:             req.SelectedResult.Number,
			FirstYearPublished: req.SelectedResult.FirstYearPublished,
			Title:              req.SelectedResult.Title,
			Authors:            req.SelectedResult.Authors,
			CoverEditionKey:    req.SelectedResult.CoverEditionKey,
			CoverImageId:       req.SelectedResult.CoverImageID,
		}

		olUpdated, err := book.UpdateFromOpenLibrary(ws.db, olResult)
		if err != nil {
			log.Printf("Warning: Failed to update book with Open Library data: %v", err)
		} else {
			updatedBook = olUpdated
		}

		if updatedBook.HasCover() && updatedBook.CoverSource == "" {
			updatedBook.CoverSource = "openlibrary"
			ws.db.Save(&updatedBook)
		}

		if !updatedBook.HasCover() {
			refreshed, found, refreshErr := models.RefreshCover(ws.db, updatedBook)
			if refreshErr != nil {
				log.Printf("Cover fallback error for '%s': %v", updatedBook.FormatTitle(), refreshErr)
			}
			if found {
				updatedBook = refreshed
			}
		}
	}

	if updatedBook.HasMissingPubDate() {
		filled, fillErr := models.FillMissingPubDate(ws.db, updatedBook)
		if fillErr != nil {
			log.Printf("Pub date fallback error for '%s': %v", updatedBook.FormatTitle(), fillErr)
		} else {
			updatedBook = filled
		}
	}

	if updatedBook.HasCover() {
		ws.captureCoversAsync(updatedBook)
	}

	response := map[string]interface{}{
		"success": true,
		"bookId":  book.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) listDecadesHandler(w http.ResponseWriter, r *http.Request) {
	var books []models.Book
	err := ws.db.Preload("Authors").Find(&books).Error
	if err != nil {
		http.Error(w, "Error fetching books", http.StatusInternalServerError)
		return
	}

	groupedBooks := pages.BooksByDecade(books)
	var decadeNames []string
	for decade, _ := range groupedBooks {
		decadeNames = append(decadeNames, decade)
	}

	// Sort decades newest to oldest, with "Unknown" at the end
	sort.Slice(decadeNames, func(i, j int) bool {
		if decadeNames[i] == "Unknown" {
			return false
		}
		if decadeNames[j] == "Unknown" {
			return true
		}
		return decadeNames[i] > decadeNames[j] // Reverse alphabetical for newest first
	})

	var decades []pages.DecadeInfo
	for _, decade := range decadeNames {
		decades = append(decades, pages.DecadeInfo{
			Decade: decade,
			Books:  groupedBooks[decade],
		})
	}

	data := PageData{
		Title:   "All Decades",
		Active:  "decades",
		Decades: decades,
	}
	ws.renderTemplate(w, r, "decades", data)
}

func (ws *WebServer) decadeHandler(w http.ResponseWriter, r *http.Request) {
	decade, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/decades/"))
	if err != nil {
		ws.renderErrorStatus(w, r, http.StatusBadRequest, "Invalid decade", err)
		return
	}
	if decade == "" {
		http.Redirect(w, r, ws.config.URL("/decades"), http.StatusFound)
		return
	}

	var books []models.Book
	if err := ws.db.Preload("Authors").Find(&books).Error; err != nil {
		ws.renderError(w, r, "Failed to load books", err)
		return
	}

	groupedBooks := pages.BooksByDecade(books)
	decadeBooks, exists := groupedBooks[decade]
	if !exists {
		ws.renderErrorStatus(w, r, http.StatusNotFound, "Decade not found",
			fmt.Errorf("no books published in %q", decade))
		return
	}

	// Within a decade, ordering by author makes it easy to spot the same
	// writer's books together.
	sort.SliceStable(decadeBooks, func(i, j int) bool {
		if decadeBooks[i].AuthorSurname != decadeBooks[j].AuthorSurname {
			return decadeBooks[i].AuthorSurname < decadeBooks[j].AuthorSurname
		}
		return decadeBooks[i].MainTitle < decadeBooks[j].MainTitle
	})

	decadeInfo := pages.DecadeInfo{
		Decade: decade,
		Books:  decadeBooks,
	}

	data := PageData{
		Title:  "Books from " + decade,
		Active: "decades",
		Decade: &decadeInfo,
	}
	ws.renderTemplate(w, r, "decade", data)
}

func (ws *WebServer) writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]string{"error": message}
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) deployHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	message, err := ws.deployToGitHub()
	if err != nil {
		data := PageData{
			Title:  "SFWR Book Management",
			Active: "home",
			Error:  fmt.Sprintf("Deployment failed: %v", err),
		}
		ws.renderTemplateStatus(w, r, http.StatusInternalServerError, "home", data)
		return
	}

	data := PageData{
		Title:   "SFWR Book Management",
		Active:  "home",
		Message: message,
	}
	ws.renderTemplate(w, r, "home", data)
}

func (ws *WebServer) buildLocalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set Content-Type BEFORE doing anything else
	w.Header().Set("Content-Type", "application/json")

	message, err := ws.buildStatic()

	if err != nil {
		response := map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("%v", err),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": message,
	}
	json.NewEncoder(w).Encode(response)
}

func (ws *WebServer) previewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if err := ws.previewTemplate.Execute(w, PageData{BasePath: ws.config.BasePath}); err != nil {
		http.Error(w, "Failed to render preview page", http.StatusInternalServerError)
	}
}

func (ws *WebServer) backupsHandler(w http.ResponseWriter, r *http.Request) {
	commits, err := ws.GetRecentCommits()
	if err != nil {
		data := PageData{
			Title:  "Database Backups",
			Active: "backups",
			Error:  fmt.Sprintf("Failed to get backup history: %v", err),
		}
		ws.renderTemplateStatus(w, r, http.StatusInternalServerError, "backups", data)
		return
	}

	data := PageData{
		Title:   "Database Backups",
		Active:  "backups",
		Commits: commits,
	}
	ws.renderTemplate(w, r, "backups", data)
}

func (ws *WebServer) rollbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, ws.config.URL("/backups"), http.StatusSeeOther)
		return
	}

	commitHash := strings.TrimSpace(r.FormValue("commit"))
	if commitHash == "" {
		ws.renderBackupsError(w, r, http.StatusBadRequest, "No commit specified for rollback")
		return
	}

	if err := ws.RollbackToCommit(commitHash); err != nil {
		ws.renderBackupsError(w, r, http.StatusInternalServerError, fmt.Sprintf("Rollback failed: %v", err))
		return
	}

	// The rollback replaced sfwr_database.db on disk, so the connection
	// this server opened at startup still points at the old file and would
	// keep serving (and re-saving) pre-rollback data.
	if err := ws.reopenDatabase(); err != nil {
		ws.renderBackupsError(w, r, http.StatusInternalServerError, fmt.Sprintf(
			"Database restored to %s, but reloading it failed: %v. Restart the server before making further edits.",
			shortHash(commitHash), err))
		return
	}

	data := PageData{
		Title:   "Database Backups",
		Active:  "backups",
		Message: fmt.Sprintf("Successfully rolled back to commit %s. The database has been restored.", shortHash(commitHash)),
	}

	// Get updated commits list
	if commits, err := ws.GetRecentCommits(); err == nil {
		data.Commits = commits
	}

	ws.renderTemplate(w, r, "backups", data)
}

// renderBackupsError redraws the backups page with an error banner and the
// commit list still in place, so the user keeps their context.
func (ws *WebServer) renderBackupsError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data := PageData{
		Title:  "Database Backups",
		Active: "backups",
		Error:  message,
	}
	if commits, err := ws.GetRecentCommits(); err == nil {
		data.Commits = commits
	}
	ws.renderTemplateStatus(w, r, status, "backups", data)
}

// shortHash abbreviates a commit hash for display without assuming the
// caller passed a full-length one.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
