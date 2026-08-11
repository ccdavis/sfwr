package models

import (
	"testing"
	"time"
)

// A cover search on iTunes once wrote the store's ebook release year into
// PubDate, dating "Leviathan Wakes" (2011) and "Lonely Werewolf Girl" (2008) to
// 2026 — after the day they were added to the collection. PlausiblePubYear is
// the check that makes those values impossible to store.
func TestPlausiblePubYear(t *testing.T) {
	added := time.Date(2024, 9, 3, 0, 0, 0, 0, time.UTC)
	book := Book{MainTitle: "Leviathan Wakes", DateAdded: added}

	tests := []struct {
		name string
		year int
		want bool
	}{
		{"first publication year", 2011, true},
		{"year the book was added", 2024, true},
		{"long before it was added", 1963, true},
		{"after it was added", 2025, false},
		{"iTunes reissue in the future", 2026, false},
		{"unset", 0, false},
		{"the Missing sentinel", int(Missing), false},
		{"negative", -5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := book.PlausiblePubYear(tt.year); got != tt.want {
				t.Errorf("PlausiblePubYear(%d) = %v, want %v", tt.year, got, tt.want)
			}
		})
	}
}

// A book with no DateAdded (imported, or built in memory) still cannot have been
// published in the future.
func TestPlausiblePubYearWithoutDateAdded(t *testing.T) {
	book := Book{MainTitle: "Some Book"}

	if !book.PlausiblePubYear(1969) {
		t.Error("expected a past year to be accepted when DateAdded is unset")
	}
	if book.PlausiblePubYear(time.Now().Year() + 1) {
		t.Error("expected a future year to be rejected when DateAdded is unset")
	}
}

func TestTitlesMatch(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Leviathan Wakes", "leviathan wakes", true},
		{"The Difference Engine", "Difference Engine", true},
		{"Dangerous Visions", "Again, Dangerous Visions", true}, // containment, deliberately loose
		{"Leviathan Wakes", "Caliban's War", false},
		{"Raven", "The Raven Boys", true},
		{"", "Leviathan Wakes", false},
	}

	for _, tt := range tests {
		if got := titlesMatch(tt.a, tt.b); got != tt.want {
			t.Errorf("titlesMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// FillMissingPubDate must leave a book that already has a year alone, and must
// not invent one when no source has a plausible answer.
func TestFillMissingPubDateLeavesKnownYearAlone(t *testing.T) {
	db := setupTestDB(t)
	book := Book{MainTitle: "Known Year", AuthorFullName: "Some Author", PubDate: 1969}
	if err := db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	got, err := FillMissingPubDate(db, book)
	if err != nil {
		t.Fatal(err)
	}
	if got.PubDate != 1969 {
		t.Errorf("PubDate = %d, want it left at 1969", got.PubDate)
	}
}
