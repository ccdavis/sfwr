package models

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"
)

type searchQuery struct {
	title  string
	author string
}

// searchQueries returns title/author combinations to try, from most specific
// to least. Tries the full title first, then main title only (no subtitle),
// paired with each author name variant. Also tries first author only for
// multi-author books.
func searchQueries(b Book) []searchQuery {
	seen := make(map[string]bool)
	var queries []searchQuery
	add := func(title, author string) {
		key := title + "|" + author
		if seen[key] {
			return
		}
		seen[key] = true
		queries = append(queries, searchQuery{title, author})
	}

	fullTitle := b.FormatTitle()
	mainTitle := b.MainTitle

	for _, author := range b.AlternateAuthorFullNames() {
		add(fullTitle, author)
	}
	if mainTitle != fullTitle {
		for _, author := range b.AlternateAuthorFullNames() {
			add(mainTitle, author)
		}
	}

	// For multi-author books, try first author only
	firstAuthor := extractFirstAuthor(b.AuthorFullName)
	if firstAuthor != b.AuthorFullName {
		add(fullTitle, firstAuthor)
		if mainTitle != fullTitle {
			add(mainTitle, firstAuthor)
		}
	}

	return queries
}

func extractFirstAuthor(fullName string) string {
	for _, sep := range []string{" and ", ", ", " & "} {
		if idx := strings.Index(fullName, sep); idx > 0 {
			return strings.TrimSpace(fullName[:idx])
		}
	}
	return fullName
}

type CoverResult struct {
	Source  string
	URLs    map[string]string // "S" -> url, "M" -> url, "L" -> url
	PubYear int
	BaseURL string // canonical URL stored in CoverImageUrl
}

func findCoverFromGoogleBooks(title string, author string) (CoverResult, bool) {
	if googleBooksDisabled {
		return CoverResult{}, false
	}
	sleepForGoogleBooks()
	results, err := SearchGoogleBooks(title, author)
	if err != nil {
		if err != errGoogleBooksRateLimited {
			log.Printf("Google Books search error for '%s': %v", title, err)
		}
		return CoverResult{}, false
	}
	for _, r := range results {
		if r.HasCover() {
			return CoverResult{
				Source:  "googlebooks",
				BaseURL: r.ThumbnailURL,
				URLs: map[string]string{
					SmallCover:  GoogleBooksCoverURL(r.ThumbnailURL, SmallCover),
					MediumCover: GoogleBooksCoverURL(r.ThumbnailURL, MediumCover),
					LargeCover:  GoogleBooksCoverURL(r.ThumbnailURL, LargeCover),
				},
				PubYear: r.PubYear,
			}, true
		}
	}
	return CoverResult{}, false
}

func findCoverFromITunes(title string, author string) (CoverResult, bool) {
	sleepForITunes()
	results, err := SearchITunes(title, author)
	if err != nil {
		log.Printf("iTunes search error for '%s': %v", title, err)
		return CoverResult{}, false
	}
	for _, r := range results {
		if r.HasCover() {
			return CoverResult{
				Source:  "itunes",
				BaseURL: r.ArtworkURL,
				URLs: map[string]string{
					SmallCover:  ITunesCoverURL(r.ArtworkURL, SmallCover),
					MediumCover: ITunesCoverURL(r.ArtworkURL, MediumCover),
					LargeCover:  ITunesCoverURL(r.ArtworkURL, LargeCover),
				},
				PubYear: r.PubYear,
			}, true
		}
	}
	return CoverResult{}, false
}

// FindCover searches Open Library, then Google Books, then iTunes for a cover image.
func FindCover(title string, author string) (CoverResult, bool) {
	// Try Open Library first
	sleepForOpenLibrary()
	olResults := SearchBook(title, author)
	selected, ok, _ := selectCoverSearchResult(olResults)
	if ok {
		return CoverResult{
			Source:  "openlibrary",
			BaseURL: fmt.Sprintf("http://covers.openlibrary.org/b/id/%s-M.jpg", selected.CoverImageId),
			URLs: map[string]string{
				SmallCover:  selected.GetBookCoverUrl(SmallCover),
				MediumCover: selected.GetBookCoverUrl(MediumCover),
				LargeCover:  selected.GetBookCoverUrl(LargeCover),
			},
			PubYear: selected.FirstYearPublished,
		}, true
	}

	// Try Google Books
	result, found := findCoverFromGoogleBooks(title, author)
	if found {
		return result, true
	}

	// Try iTunes
	return findCoverFromITunes(title, author)
}

// RefreshCover tries all sources to find a cover for a book that doesn't have one.
// It updates the book in the database and returns the updated book.
//
// It deliberately does not touch PubDate. A cover match is a match on artwork,
// and the year that comes back with it is the year of *that* edition — for
// iTunes, the date the ebook went on sale. Writing those into PubDate once put
// 2026 on books first published in the 1960s. Publication years come from
// FillMissingPubDate instead.
func RefreshCover(db *gorm.DB, b Book) (Book, bool, error) {
	if b.HasCover() {
		return b, false, nil
	}

	// Try Open Library edition endpoint first (existing logic)
	updated, ok, err := refreshCoverFromEdition(db, b)
	if err != nil {
		log.Printf("OL edition refresh error for '%s': %v", b.FormatTitle(), err)
	}
	if ok {
		updated.CoverSource = "openlibrary"
		if err := db.Save(&updated).Error; err != nil {
			return b, false, err
		}
		log.Printf("Found cover for '%s' via Open Library (edition).", b.FormatTitle())
		return updated, true, nil
	}

	// Try Open Library search (existing logic)
	updated, ok, err = refreshCoverFromSearch(db, b)
	if err != nil {
		log.Printf("OL search refresh error for '%s': %v", b.FormatTitle(), err)
	}
	if ok {
		updated.CoverSource = "openlibrary"
		if err := db.Save(&updated).Error; err != nil {
			return b, false, err
		}
		log.Printf("Found cover for '%s' via Open Library (search).", b.FormatTitle())
		return updated, true, nil
	}

	queries := searchQueries(b)

	// Try Google Books (skip all variants if rate limited)
	if !googleBooksDisabled {
		for _, q := range queries {
			result, found := findCoverFromGoogleBooks(q.title, q.author)
			if found {
				b.CoverSource = "googlebooks"
				b.CoverImageUrl = result.BaseURL
				if err := db.Save(&b).Error; err != nil {
					return b, false, err
				}
				log.Printf("Found cover for '%s' via Google Books.", b.FormatTitle())
				return b, true, nil
			}
			if googleBooksDisabled {
				break
			}
		}
	}

	// Try iTunes
	for _, q := range queries {
		result, found := findCoverFromITunes(q.title, q.author)
		if found {
			b.CoverSource = "itunes"
			b.CoverImageUrl = result.BaseURL
			if err := db.Save(&b).Error; err != nil {
				return b, false, err
			}
			log.Printf("Found cover for '%s' via iTunes.", b.FormatTitle())
			return b, true, nil
		}
	}

	log.Printf("No cover found from any source for '%s' by '%s'.", b.FormatTitle(), b.AuthorFullName)
	return b, false, nil
}

// CaptureAllCovers downloads S/M/L cover images from the book's cover source.
func CaptureAllCovers(b Book, imageDir string) {
	if !b.HasCover() {
		return
	}
	for _, size := range []string{SmallCover, MediumCover, LargeCover} {
		imageFile := b.MakeCoverImageFilename(imageDir, size)
		if !shouldDownloadCover(imageFile, imageDir, size) {
			continue
		}
		coverURL := b.MakeCoverImageUrl(size)
		err := saveCoverImage(imageFile, coverURL)
		if err != nil {
			log.Printf("ERROR saving %s cover for %q from %s: %v", size, b.FormatTitle(), b.CoverSource, err)
			if copyErr := copyPlaceholderImage(imageFile, imageDir, size); copyErr != nil {
				log.Printf("Warning: Failed to copy placeholder image for %s: %v", imageFile, copyErr)
			}
		}
	}
}

// CaptureCoverImagesWithFallback is the main entry point for bulk cover download.
// It uses the full fallback chain (OL -> Google Books -> iTunes).
func CaptureCoverImagesWithFallback(db *gorm.DB, books []Book, imageDir string) error {
	err := os.MkdirAll(imageDir, 0775)
	if err != nil {
		return fmt.Errorf("can't create directory for saved cover images: %w", err)
	}

	needCover := 0
	needDownload := 0
	for _, b := range books {
		if !b.HasCover() {
			needCover++
		} else if len(missingCoverSizesForBook(b, imageDir)) > 0 {
			needDownload++
		}
	}
	fmt.Printf("%d books need cover lookup, %d need image downloads, %d already complete.\n",
		needCover, needDownload, len(books)-needCover-needDownload)

	for _, b := range books {
		bookToProcess := b

		if !bookToProcess.HasCover() && db != nil {
			log.Printf("Searching all sources for cover of '%s'...", bookToProcess.FormatTitle())
			updatedBook, updated, err := RefreshCover(db, bookToProcess)
			if err != nil {
				log.Printf("Warning: Failed to find cover for '%s': %v", bookToProcess.FormatTitle(), err)
			} else if updated {
				bookToProcess = updatedBook
			}
		}

		if !bookToProcess.HasCover() {
			log.Printf("No cover available for '%s' from any source.", b.FormatTitle())
			continue
		}

		missingSizes := missingCoverSizesForBook(bookToProcess, imageDir)
		if len(missingSizes) == 0 {
			continue
		}

		switch bookToProcess.CoverSource {
		case "googlebooks":
			sleepForGoogleBooks()
		case "itunes":
			sleepForITunes()
		default:
			sleepForOpenLibrary()
		}
		for _, size := range missingSizes {
			imageFile := bookToProcess.MakeCoverImageFilename(imageDir, size)
			coverURL := bookToProcess.MakeCoverImageUrl(size)
			err := saveCoverImage(imageFile, coverURL)
			if err != nil {
				log.Printf("ERROR saving %s cover for %q: %v", size, bookToProcess.FormatTitle(), err)
				if copyErr := copyPlaceholderImage(imageFile, imageDir, size); copyErr != nil {
					log.Printf("Warning: Failed to copy placeholder: %v", copyErr)
				}
			}
		}
	}
	return nil
}

func missingCoverSizesForBook(b Book, imageDir string) []string {
	sizes := []string{SmallCover, MediumCover, LargeCover}
	var missing []string
	for _, size := range sizes {
		filename := b.MakeCoverImageFilename(imageDir, size)
		if !coverImageExists(filename) {
			missing = append(missing, size)
			continue
		}
		if coverImageIsPlaceholder(filename, imageDir, size) {
			missing = append(missing, size)
		}
	}
	return missing
}

// PlausiblePubYear rejects years that cannot be a first publication date for
// this book. A year in the future is impossible, and so is a year after the
// book was added to the collection — it cannot have been read before it existed.
//
// This is the check that catches a store re-release date masquerading as a
// publication year, which is exactly how "Leviathan Wakes" (2011) and "Lonely
// Werewolf Girl" (2008) ended up dated 2026.
func (b Book) PlausiblePubYear(year int) bool {
	if year <= 0 {
		return false
	}
	if year > time.Now().Year() {
		return false
	}
	if !b.DateAdded.IsZero() && year > b.DateAdded.Year() {
		return false
	}
	return true
}

// firstPublishedYearFromOpenLibrary returns the earliest first-publication year
// Open Library reports for this book. Open Library is the only one of the three
// sources that models a *work* separately from its editions, so its
// first_publish_year is an actual first-publication year rather than the date
// of whichever printing the search happened to surface.
func firstPublishedYearFromOpenLibrary(b Book) int {
	best := 0
	for _, q := range searchQueries(b) {
		sleepForOpenLibrary()
		for _, r := range SearchBook(q.title, q.author) {
			if !b.PlausiblePubYear(r.FirstYearPublished) {
				continue
			}
			if !titlesMatch(r.Title, q.title) {
				continue
			}
			if best == 0 || r.FirstYearPublished < best {
				best = r.FirstYearPublished
			}
		}
		if best > 0 {
			return best
		}
	}
	return best
}

// earliestGoogleBooksYear returns the earliest edition year Google Books lists.
// Google dates each edition rather than the work, so the best available estimate
// of first publication is the earliest edition it knows about — not, as this code
// used to assume, the year attached to the first result.
func earliestGoogleBooksYear(b Book) int {
	if googleBooksDisabled {
		return 0
	}
	best := 0
	for _, q := range searchQueries(b) {
		sleepForGoogleBooks()
		results, err := SearchGoogleBooks(q.title, q.author)
		if err != nil {
			if err == errGoogleBooksRateLimited {
				return best
			}
			continue
		}
		for _, r := range results {
			if !b.PlausiblePubYear(r.PubYear) {
				continue
			}
			if !titlesMatch(r.Title, q.title) {
				continue
			}
			if best == 0 || r.PubYear < best {
				best = r.PubYear
			}
		}
		if best > 0 {
			return best
		}
	}
	return best
}

// titlesMatch is a loose containment check, enough to throw out a result for a
// different book that came back on a fuzzy search.
func titlesMatch(a, b string) bool {
	na, nb := normalizeTitle(a), normalizeTitle(b)
	if na == "" || nb == "" {
		return false
	}
	return strings.Contains(na, nb) || strings.Contains(nb, na)
}

func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "the ")
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FillMissingPubDate looks up a first-publication year for a book that has none.
//
// It asks Open Library first, because its first_publish_year is a property of the
// work; failing that it falls back to the earliest edition Google Books lists.
// iTunes is not consulted: its releaseDate is when the ebook went on sale in the
// store, which for older books is decades off and for recent reissues lands in
// the future. Leaving the year unknown is better than publishing a wrong one —
// the site already renders unknown years.
func FillMissingPubDate(db *gorm.DB, b Book) (Book, error) {
	if b.PubDate != Missing && b.PubDate != 0 {
		return b, nil
	}

	if year := firstPublishedYearFromOpenLibrary(b); year > 0 {
		log.Printf("Found pub date %d for '%s' via Open Library.", year, b.FormatTitle())
		b.PubDate = int64(year)
		if err := db.Save(&b).Error; err != nil {
			return b, err
		}
		return b, nil
	}

	if year := earliestGoogleBooksYear(b); year > 0 {
		log.Printf("Found pub date %d for '%s' via Google Books (earliest edition listed).", year, b.FormatTitle())
		b.PubDate = int64(year)
		if err := db.Save(&b).Error; err != nil {
			return b, err
		}
		return b, nil
	}

	log.Printf("No reliable publication year found for '%s'; leaving it unknown.", b.FormatTitle())
	return b, nil
}

// MigrateCovers renames cover image files from the old {OlCoverId}-{size}.jpg scheme
// to the new book_{ID}-{size}.jpg scheme. Idempotent.
func MigrateCovers(db *gorm.DB, imageDir string) error {
	var books []Book
	if err := db.Find(&books).Error; err != nil {
		return fmt.Errorf("failed to load books for cover migration: %w", err)
	}

	migrated := 0
	for _, b := range books {
		if b.OlCoverId == Missing || b.OlCoverId == 0 {
			continue
		}

		for _, size := range []string{SmallCover, MediumCover, LargeCover} {
			oldFile := path.Join(imageDir, fmt.Sprintf("%d-%s.jpg", b.OlCoverId, size))
			newFile := path.Join(imageDir, fmt.Sprintf("book_%d-%s.jpg", b.ID, size))

			if _, err := os.Stat(newFile); err == nil {
				continue // destination already exists
			}
			if _, err := os.Stat(oldFile); err != nil {
				continue // source doesn't exist
			}

			if err := os.Rename(oldFile, newFile); err != nil {
				log.Printf("Warning: failed to rename %s -> %s: %v", oldFile, newFile, err)
				continue
			}
			migrated++
		}

		if b.CoverSource == "" {
			b.CoverSource = "openlibrary"
			if err := db.Save(&b).Error; err != nil {
				log.Printf("Warning: failed to update CoverSource for book %d: %v", b.ID, err)
			}
		}
	}

	fmt.Printf("Cover migration complete: %d files renamed.\n", migrated)

	// Also set CoverSource for books that already have the new filenames
	var booksWithOlCovers []Book
	if err := db.Where("ol_cover_id != ? AND ol_cover_id != 0 AND (cover_source IS NULL OR cover_source = '')", Missing).Find(&booksWithOlCovers).Error; err == nil {
		for _, b := range booksWithOlCovers {
			b.CoverSource = "openlibrary"
			db.Save(&b)
		}
	}

	return nil
}

// CoverSourceDisplay returns a human-readable label for the cover source.
func (b Book) CoverSourceDisplay() string {
	switch b.CoverSource {
	case "openlibrary":
		return "Open Library"
	case "googlebooks":
		return "Google Books"
	case "itunes":
		return "Apple Books"
	default:
		if b.OlCoverId != Missing && b.OlCoverId != 0 {
			return "Open Library"
		}
		return ""
	}
}

// HasMissingPubDate returns true if the book has no publication date.
func (b Book) HasMissingPubDate() bool {
	return b.PubDate == Missing || b.PubDate == 0
}

func coverFileExists(imageDir string, b Book) bool {
	for _, size := range []string{SmallCover, MediumCover, LargeCover} {
		filename := b.MakeCoverImageFilename(imageDir, size)
		if !coverImageExists(filename) {
			return false
		}
		if coverImageIsPlaceholder(filename, imageDir, size) {
			return false
		}
	}
	return true
}

// OldCoverFilename returns the legacy filename for migration checking.
func oldCoverFilename(imageDir string, olCoverId int64, size string) string {
	return path.Join(imageDir, fmt.Sprintf("%d-%s.jpg", olCoverId, size))
}
