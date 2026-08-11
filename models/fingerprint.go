package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// Fingerprint summarises the collection: the books, the authors, and the
// cover art on disk. Two machines holding the same collection produce the
// same fingerprint, so a sync can tell which side has changed without
// shipping the whole database across the network.
type Fingerprint struct {
	Books   int
	Authors int
	Covers  int

	// Data is a hash over every field that ends up on the published site.
	Data string
}

// String renders the fingerprint as the "key value" lines that sync.sh
// parses. Keep the format stable; the script greps it.
func (f Fingerprint) String() string {
	return fmt.Sprintf("books %d\nauthors %d\ncovers %d\ndata %s",
		f.Books, f.Authors, f.Covers, f.Data)
}

// TakeFingerprint reads the database and the cover directory. It is
// deliberately independent of row order, file order, and SQLite's on-disk
// layout: only the content that reaches the site counts, so an unrelated
// VACUUM or page rewrite does not read as a change.
func TakeFingerprint(db *gorm.DB, coverImagesDir string) (Fingerprint, error) {
	var books []Book
	if err := db.Preload("Authors").Find(&books).Error; err != nil {
		return Fingerprint{}, fmt.Errorf("could not read books: %w", err)
	}
	var authors []Author
	if err := db.Find(&authors).Error; err != nil {
		return Fingerprint{}, fmt.Errorf("could not read authors: %w", err)
	}

	sort.Slice(books, func(i, j int) bool { return books[i].ID < books[j].ID })
	sort.Slice(authors, func(i, j int) bool { return authors[i].ID < authors[j].ID })

	var b strings.Builder
	for _, book := range books {
		authorIDs := make([]string, 0, len(book.Authors))
		for _, a := range book.Authors {
			authorIDs = append(authorIDs, fmt.Sprint(a.ID))
		}
		sort.Strings(authorIDs)

		fmt.Fprintf(&b, "book\x1f%d\x1f%s\x1f%s\x1f%s\x1f%s\x1f%d\x1f%s\x1f%t\x1f%t\x1f%s\x1f%s\x1f%d\x1f%s\x1f%s\n",
			book.ID, book.MainTitle, book.SubTitle, book.AuthorFullName, book.AuthorSurname,
			book.PubDate, book.Rating, book.Indy, book.Interesting, book.Review,
			book.CoverSource, book.OlCoverId, book.OlCoverEditionId,
			strings.Join(authorIDs, ","))
	}
	for _, a := range authors {
		fmt.Fprintf(&b, "author\x1f%d\x1f%s\x1f%s\n", a.ID, a.FullName, a.Surname)
	}

	covers, err := fingerprintCovers(coverImagesDir, &b)
	if err != nil {
		return Fingerprint{}, err
	}

	sum := sha256.Sum256([]byte(b.String()))
	return Fingerprint{
		Books:   len(books),
		Authors: len(authors),
		Covers:  covers,
		Data:    hex.EncodeToString(sum[:]),
	}, nil
}

// fingerprintCovers folds the cover files into the hash by name and size.
// Size is enough to notice a re-fetched image without reading every byte of
// a thousand-odd files on each check.
func fingerprintCovers(dir string, b *strings.Builder) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("could not read %s: %w", dir, err)
	}

	names := make([]string, 0, len(entries))
	sizes := make(map[string]int64, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jpg" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		names = append(names, entry.Name())
		sizes[entry.Name()] = info.Size()
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(b, "cover\x1f%s\x1f%d\n", name, sizes[name])
	}
	return len(names), nil
}
