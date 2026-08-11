package models

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func fingerprintDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDB(t)

	author := Author{FullName: "Ursula K. Le Guin", Surname: "Le Guin"}
	db.Create(&author)
	for _, title := range []string{"The Dispossessed", "The Left Hand of Darkness"} {
		book := Book{MainTitle: title, AuthorFullName: author.FullName, PubDate: 1969, Rating: "Excellent"}
		db.Create(&book)
		db.Model(&book).Association("Authors").Append(&author)
	}
	return db
}

func take(t *testing.T, db *gorm.DB, coverDir string) Fingerprint {
	t.Helper()
	fp, err := TakeFingerprint(db, coverDir)
	if err != nil {
		t.Fatalf("TakeFingerprint failed: %v", err)
	}
	return fp
}

func TestFingerprintCountsTheCollection(t *testing.T) {
	fp := take(t, fingerprintDB(t), t.TempDir())
	if fp.Books != 2 || fp.Authors != 1 || fp.Covers != 0 {
		t.Errorf("got books=%d authors=%d covers=%d, want 2/1/0", fp.Books, fp.Authors, fp.Covers)
	}
	if fp.Data == "" {
		t.Error("expected a data hash")
	}
}

// The whole sync rests on this: identical collections must agree, or the
// script would report a difference that isn't there and copy needlessly.
func TestFingerprintIsStableAcrossCalls(t *testing.T) {
	db := fingerprintDB(t)
	dir := t.TempDir()
	if take(t, db, dir).Data != take(t, db, dir).Data {
		t.Error("two fingerprints of the same data disagree")
	}
}

// And the converse: any field that reaches the published site has to move
// the hash, or a real edit would be missed and never synced.
func TestFingerprintNoticesEveryPublishedField(t *testing.T) {
	dir := t.TempDir()

	cases := map[string]func(db *gorm.DB){
		"publication year": func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("pub_date", 1974) },
		"title":            func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("main_title", "Changed") },
		"subtitle":         func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("sub_title", "A Novel") },
		"rating":           func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("rating", "Very-Good") },
		"review":           func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("review", "Rewritten.") },
		"indy tag":         func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("indy", true) },
		"interesting tag":  func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("interesting", true) },
		"cover source":     func(db *gorm.DB) { db.Model(&Book{}).Where("id = 1").Update("cover_source", "itunes") },
		"author name":      func(db *gorm.DB) { db.Model(&Author{}).Where("id = 1").Update("full_name", "Someone Else") },
		"a deleted book":   func(db *gorm.DB) { db.Delete(&Book{}, 1) },
		"a new book":       func(db *gorm.DB) { db.Create(&Book{MainTitle: "Extra", PubDate: 2000}) },
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			db := fingerprintDB(t)
			before := take(t, db, dir).Data
			change(db)
			if after := take(t, db, dir).Data; after == before {
				t.Errorf("changing the %s did not change the fingerprint", name)
			}
		})
	}
}

func TestFingerprintNoticesCoverArt(t *testing.T) {
	db := fingerprintDB(t)
	dir := t.TempDir()
	before := take(t, db, dir).Data

	cover := filepath.Join(dir, "book_1-M.jpg")
	if err := os.WriteFile(cover, []byte("pretend-jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
	added := take(t, db, dir)
	if added.Data == before {
		t.Fatal("adding a cover did not change the fingerprint")
	}
	if added.Covers != 1 {
		t.Errorf("Covers = %d, want 1", added.Covers)
	}

	// A re-fetched cover keeps its name but changes size.
	if err := os.WriteFile(cover, []byte("a different, longer jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
	if take(t, db, dir).Data == added.Data {
		t.Error("replacing a cover with a different file did not change the fingerprint")
	}
}

// Non-cover files sitting in the directory are not published, so they must
// not make two installations look different.
func TestFingerprintIgnoresNonCoverFiles(t *testing.T) {
	db := fingerprintDB(t)
	dir := t.TempDir()
	before := take(t, db, dir).Data

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("scratch"), 0644); err != nil {
		t.Fatal(err)
	}
	if take(t, db, dir).Data != before {
		t.Error("a non-jpg file changed the fingerprint")
	}
}

func TestFingerprintHandlesAMissingCoverDirectory(t *testing.T) {
	fp, err := TakeFingerprint(fingerprintDB(t), filepath.Join(t.TempDir(), "not-there"))
	if err != nil {
		t.Fatalf("a missing cover directory should not be an error: %v", err)
	}
	if fp.Covers != 0 {
		t.Errorf("Covers = %d, want 0", fp.Covers)
	}
}

func TestFingerprintStringIsParseable(t *testing.T) {
	fp := Fingerprint{Books: 464, Authors: 251, Covers: 1338, Data: "abc123"}
	want := "books 464\nauthors 251\ncovers 1338\ndata abc123"
	if got := fp.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
