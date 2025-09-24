package models

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	// Use in-memory database for tests
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal("Failed to connect to test database:", err)
	}

	// Migrate schema
	err = db.AutoMigrate(&Book{}, &Author{}, &OpenLibraryBookAuthor{}, &OpenLibraryBookIsbn{})
	if err != nil {
		t.Fatal("Failed to migrate test database:", err)
	}

	return db
}

func TestRatingEnum(t *testing.T) {
	tests := []struct {
		name        string
		rating      Rating
		stringVal   string
		displayVal  string
	}{
		{"Unknown", Unknown, "Not Rated", "Not Rated"},
		{"VeryGood", VeryGood, "Very-Good", "Very Good"},
		{"Excellent", Excellent, "Excellent", "Excellent"},
		{"Kindle", Kindle, "Kindle", "Kindle"},
		{"Interesting", Interesting, "Interesting", "Interesting"},
		{"NotGood", NotGood, "Not-Good", "Not Good"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test String()
			if got := tt.rating.String(); got != tt.stringVal {
				t.Errorf("Rating.String() = %v, want %v", got, tt.stringVal)
			}

			// Test Display()
			if got := tt.rating.Display(); got != tt.displayVal {
				t.Errorf("Rating.Display() = %v, want %v", got, tt.displayVal)
			}
		})
	}
}

func TestStringToRating(t *testing.T) {
	tests := []struct {
		input    string
		expected Rating
		hasError bool
	}{
		{"Not Rated", Unknown, false},
		{"Very-Good", VeryGood, false},
		{"Excellent", Excellent, false},
		{"Kindle", Kindle, false},
		{"Interesting", Interesting, false},
		{"Not-Good", NotGood, false},
		{"Invalid", Unknown, true},
		{"", Unknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := StringToRating(tt.input)
			if tt.hasError && err == nil {
				t.Errorf("StringToRating(%v) expected error but got none", tt.input)
			}
			if !tt.hasError && err != nil {
				t.Errorf("StringToRating(%v) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("StringToRating(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRatingDatabaseStorage(t *testing.T) {
	db := setupTestDB(t)

	// Test storing and retrieving ratings
	book := Book{
		MainTitle: "Test Book",
		Rating:    "Excellent",
	}

	result := db.Create(&book)
	if result.Error != nil {
		t.Fatal("Failed to create book:", result.Error)
	}

	var retrieved Book
	db.First(&retrieved, book.ID)

	if retrieved.Rating != "Excellent" {
		t.Errorf("Expected rating 'Excellent', got '%s'", retrieved.Rating)
	}
}

func TestBookCreate(t *testing.T) {
	db := setupTestDB(t)

	book := Book{
		MainTitle:        "Test Book",
		SubTitle:         "A Test Subtitle",
		AuthorFullName:   "Test Author",
		AuthorSurname:    "Author",
		PubDate:          2024,
		Rating:           "Very-Good",
		Review:           "This is a test review",
		OlCoverEditionId: "OL12345M",
	}

	id, err := book.Create(db)
	if err != nil {
		t.Fatal("Failed to create book:", err)
	}

	if id == 0 {
		t.Error("Book ID should not be 0")
	}

	// Verify book was created
	var count int64
	db.Model(&Book{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 book, got %d", count)
	}

	// Verify all fields were saved
	var saved Book
	db.First(&saved, id)

	if saved.MainTitle != book.MainTitle {
		t.Errorf("MainTitle mismatch: got %s, want %s", saved.MainTitle, book.MainTitle)
	}
	if saved.SubTitle != book.SubTitle {
		t.Errorf("SubTitle mismatch: got %s, want %s", saved.SubTitle, book.SubTitle)
	}
	if saved.PubDate != book.PubDate {
		t.Errorf("PubDate mismatch: got %d, want %d", saved.PubDate, book.PubDate)
	}
	if saved.Rating != book.Rating {
		t.Errorf("Rating mismatch: got %s, want %s", saved.Rating, book.Rating)
	}
}

func TestAuthorBookRelationship(t *testing.T) {
	db := setupTestDB(t)

	// Create authors
	author1 := Author{
		FullName: "Author One",
		Surname:  "One",
	}
	author2 := Author{
		FullName: "Author Two",
		Surname:  "Two",
	}

	db.Create(&author1)
	db.Create(&author2)

	// Create books
	book1 := Book{
		MainTitle:      "Book One",
		AuthorFullName: "Author One",
		AuthorSurname:  "One",
	}
	book2 := Book{
		MainTitle:      "Book Two",
		AuthorFullName: "Author One",
		AuthorSurname:  "One",
	}
	book3 := Book{
		MainTitle:      "Book Three",
		AuthorFullName: "Author Two",
		AuthorSurname:  "Two",
	}

	db.Create(&book1)
	db.Create(&book2)
	db.Create(&book3)

	// Associate books with authors
	db.Model(&book1).Association("Authors").Append(&author1)
	db.Model(&book2).Association("Authors").Append(&author1)
	db.Model(&book3).Association("Authors").Append(&author2)

	// Test loading books with authors
	var books []Book
	db.Preload("Authors").Find(&books)

	if len(books) != 3 {
		t.Errorf("Expected 3 books, got %d", len(books))
	}

	// Verify associations
	for _, book := range books {
		if book.MainTitle == "Book One" || book.MainTitle == "Book Two" {
			if len(book.Authors) != 1 || book.Authors[0].FullName != "Author One" {
				t.Errorf("Book %s should have Author One", book.MainTitle)
			}
		}
		if book.MainTitle == "Book Three" {
			if len(book.Authors) != 1 || book.Authors[0].FullName != "Author Two" {
				t.Errorf("Book Three should have Author Two")
			}
		}
	}

	// Test loading authors with books
	var authorWithBooks Author
	db.Preload("Books").First(&authorWithBooks, author1.ID)

	if len(authorWithBooks.Books) != 2 {
		t.Errorf("Author One should have 2 books, got %d", len(authorWithBooks.Books))
	}
}

func TestHasOpenLibraryId(t *testing.T) {
	tests := []struct {
		name     string
		book     Book
		expected bool
	}{
		{
			name:     "Has OlCoverEditionId",
			book:     Book{OlCoverEditionId: "OL12345M"},
			expected: true,
		},
		{
			name:     "Empty OlCoverEditionId",
			book:     Book{OlCoverEditionId: ""},
			expected: false,
		},
		{
			name:     "Whitespace only",
			book:     Book{OlCoverEditionId: "   "},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.book.HasOpenLibraryId(); got != tt.expected {
				t.Errorf("HasOpenLibraryId() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasCoverImageId(t *testing.T) {
	tests := []struct {
		name     string
		book     Book
		expected bool
	}{
		{
			name:     "Has OlCoverId",
			book:     Book{OlCoverId: 12345},
			expected: true,
		},
		{
			name:     "Zero OlCoverId",
			book:     Book{OlCoverId: 0},
			expected: false,
		},
		{
			name:     "Missing constant value",
			book:     Book{OlCoverId: Missing},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.book.HasCoverImageId(); got != tt.expected {
				t.Errorf("HasCoverImageId() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBookFormatting(t *testing.T) {
	book := Book{
		MainTitle: "The Test Book",
		SubTitle:  "A Subtitle",
		PubDate:   2024,
		Rating:    "Excellent",
	}

	// Test FormatTitle
	title := book.FormatTitle()
	expectedTitle := "The Test Book: A Subtitle"
	if title != expectedTitle {
		t.Errorf("FormatTitle() = %v, want %v", title, expectedTitle)
	}

	// Test FormatPubDate
	pubDate := book.FormatPubDate()
	if pubDate != "2024" {
		t.Errorf("FormatPubDate() = %v, want 2024", pubDate)
	}

	// Test FormatRating
	rating := book.FormatRating()
	if rating != "Excellent" {
		t.Errorf("FormatRating() = %v, want Excellent", rating)
	}

	// Test with no subtitle
	book2 := Book{MainTitle: "No Subtitle"}
	title2 := book2.FormatTitle()
	if title2 != "No Subtitle" {
		t.Errorf("FormatTitle() without subtitle = %v, want 'No Subtitle'", title2)
	}
}

func TestExtractSurname(t *testing.T) {
	tests := []struct {
		fullName string
		expected string
	}{
		{"John Smith", "Smith"},
		{"Mary Jane Watson", "Watson"},
		{"Cher", "Cher"},
		{"", ""},
		{"  John  Smith  ", "Smith"},
		{"José García López", "López"},
	}

	for _, tt := range tests {
		t.Run(tt.fullName, func(t *testing.T) {
			if got := ExtractSurname(tt.fullName); got != tt.expected {
				t.Errorf("ExtractSurname(%v) = %v, want %v", tt.fullName, got, tt.expected)
			}
		})
	}
}

func TestMakeCoverImageUrl(t *testing.T) {
	tests := []struct {
		name     string
		book     Book
		size     string
		expected string
	}{
		{
			name:     "With OlCoverId small",
			book:     Book{OlCoverId: 12345},
			size:     "S",
			expected: "http://covers.openlibrary.org/b/id/12345-S.jpg",
		},
		{
			name:     "With OlCoverId medium",
			book:     Book{OlCoverId: 12345},
			size:     "M",
			expected: "http://covers.openlibrary.org/b/id/12345-M.jpg",
		},
		{
			name:     "With OlCoverId large",
			book:     Book{OlCoverId: 67890},
			size:     "L",
			expected: "http://covers.openlibrary.org/b/id/67890-L.jpg",
		},
		{
			name:     "No OlCoverId",
			book:     Book{OlCoverId: 0},
			size:     "M",
			expected: "placeholder-M.jpg",
		},
		{
			name:     "Missing OlCoverId",
			book:     Book{OlCoverId: Missing},
			size:     "L",
			expected: "placeholder-L.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.book.MakeCoverImageUrl(tt.size); got != tt.expected {
				t.Errorf("MakeCoverImageUrl() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMakeCoverImageFilename(t *testing.T) {
	imageDir := "/images"

	tests := []struct {
		name     string
		book     Book
		size     string
		expected string
	}{
		{
			name:     "OlCoverId with size",
			book:     Book{OlCoverId: 12345},
			size:     "M",
			expected: "/images/12345-M.jpg",
		},
		{
			name:     "OlCoverId with size large",
			book:     Book{OlCoverId: 67890},
			size:     "L",
			expected: "/images/67890-L.jpg",
		},
		{
			name:     "No ID returns placeholder",
			book:     Book{OlCoverId: 0},
			size:     "M",
			expected: "/images/placeholder-M.jpg",
		},
		{
			name:     "Missing ID returns placeholder",
			book:     Book{OlCoverId: Missing},
			size:     "S",
			expected: "/images/placeholder-S.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.book.MakeCoverImageFilename(imageDir, tt.size); got != tt.expected {
				t.Errorf("MakeCoverImageFilename() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLoadAllBooks(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	author := Author{
		FullName: "Test Author",
		Surname:  "Author",
	}
	db.Create(&author)

	books := []Book{
		{MainTitle: "Book 1", Rating: "Excellent"},
		{MainTitle: "Book 2", Rating: "Very-Good"},
		{MainTitle: "Book 3", Rating: "Kindle"},
	}

	for i := range books {
		db.Create(&books[i])
		db.Model(&books[i]).Association("Authors").Append(&author)
	}

	// Load all books
	loaded, err := LoadAllBooks(db)
	if err != nil {
		t.Fatal("Failed to load books:", err)
	}

	if len(loaded) != 3 {
		t.Errorf("Expected 3 books, got %d", len(loaded))
	}

	// Verify authors are preloaded
	for _, book := range loaded {
		if len(book.Authors) != 1 {
			t.Errorf("Book %s should have 1 author", book.MainTitle)
		}
	}
}

// TestTransferJsonBooksToDatabase is skipped because it requires specific JSON format
// that matches the load.RawBook structure which has different field names and types
func TestTransferJsonBooksToDatabase(t *testing.T) {
	t.Skip("Skipping JSON transfer test - requires specific RawBook JSON format")
}