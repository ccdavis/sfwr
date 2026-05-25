package models

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Open-pi/gol"
	"gorm.io/gorm"
)

type BookSearchResult struct {
	Number             int
	FirstYearPublished int
	Title              string
	Authors            []string
	Languages          []string
	CoverEditionKey    string
	CoverImageId       string
	AuthorIds          []string
}

func (s BookSearchResult) Print() string {
	author := "Unknown Author"
	if len(s.Authors) > 0 {
		author = s.Authors[0]
	}
	return fmt.Sprintln(s.FirstYearPublished, "\t", author, ": ", s.Title, "\tCover Image ID: ", s.CoverImageId, "\tCover Edition ID: ", s.CoverEditionKey)
}

func (s BookSearchResult) GetBookCoverUrl(size string) string {
	return gol.GetBookCoverURL("OLID", s.CoverEditionKey, size)
}

func GetBookByOlId(olid string) (gol.Book, error) {
	return gol.GetEdition(olid)
}

func SearchBook(title string, author string) []BookSearchResult {
	var results []BookSearchResult
	// Construct the SearchUrl
	url := gol.SearchUrl().All(title).Author(author).Construct()

	// search
	search, err := gol.Search(url)
	if err == nil {

		for key, child := range search.ChildrenMap() {
			if key == "docs" {
				for bookNumber, b := range child.Children() {
					var work BookSearchResult
					work.Number = bookNumber
					for fieldName, fieldValue := range b.ChildrenMap() {
						switch fieldName {
						case "first_publish_year":
							work.FirstYearPublished, err = strconv.Atoi(fieldValue.String())
							if err != nil {
								work.FirstYearPublished = 0
								fmt.Println("While retrieving search result for '", title, "', failed to convert first published date: ", err)
							}
						case "title":
							work.Title = fieldValue.String()
						case "author_name":
							var authors []string
							for _, author := range fieldValue.Children() {
								authors = append(authors, author.String())
							}
							work.Authors = authors
						case "author_key":
							var authorIds []string
							for _, child := range fieldValue.Children() {
								authorIds = append(authorIds, child.String())
							}
							work.AuthorIds = authorIds
						case "language":
							if len(fieldValue.Children()) == 0 {
								raw := strings.Trim(strings.TrimSpace(fieldValue.String()), "[]")
								if raw != "" {
									for _, entry := range strings.Split(raw, ",") {
										lang := strings.Trim(entry, "\" ")
										if lang != "" {
											work.Languages = append(work.Languages, lang)
										}
									}
								}
								break
							}
							for _, child := range fieldValue.Children() {
								lang := strings.TrimSpace(child.String())
								if lang != "" {
									work.Languages = append(work.Languages, lang)
								}
							}
						case "cover_edition_key":
							work.CoverEditionKey = strings.ReplaceAll(strings.TrimSpace(fieldValue.String()), "\"", "")
						case "cover_i":
							work.CoverImageId = strings.TrimSpace(fieldValue.String())
						}
					} // each field
					results = append(results, work)
				}
			}
		}
	} else {
		fmt.Println("Could not find: ", err)
	}
	return results
}

func (s BookSearchResult) HasCoverImageId() bool {
	return strings.TrimSpace(s.CoverImageId) != ""
}

func (s BookSearchResult) HasEnglishLanguage() bool {
	for _, lang := range s.Languages {
		if isEnglishLanguageCode(lang) {
			return true
		}
	}
	return false
}

func isEnglishLanguageCode(lang string) bool {
	normalized := strings.ToLower(strings.TrimSpace(lang))
	switch normalized {
	case "eng", "en", "english", "en-us", "en-gb":
		return true
	}
	return strings.HasPrefix(normalized, "en-")
}

func selectCoverSearchResult(results []BookSearchResult) (BookSearchResult, bool, bool) {
	var fallback *BookSearchResult
	for _, result := range results {
		if !result.HasCoverImageId() {
			continue
		}
		if result.HasEnglishLanguage() {
			return result, true, true
		}
		if fallback == nil {
			fallback = &result
		}
	}
	if fallback != nil {
		return *fallback, true, false
	}
	return BookSearchResult{}, false, false
}

func saveCoverImage(filename string, imageurl string) error {
	response, err := http.Get(imageurl)
	if err != nil {
		return fmt.Errorf("error saving cover image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("error saving cover image: unexpected status %s", response.Status)
	}
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error saving cover image: %w", err)
	}
	defer file.Close()
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return fmt.Errorf("error saving cover image: %w", err)
	}

	return nil
}

func sleepForOpenLibrary() {
	r := rand.IntN(3)
	time.Sleep(time.Duration(r) * time.Second)
}

func refreshCoverFromEdition(db *gorm.DB, b Book) (Book, bool, error) {
	if !b.HasOpenLibraryId() {
		return b, false, nil
	}
	sleepForOpenLibrary()
	olBook, err := GetBookByOlId(b.OlCoverEditionId)
	if err != nil {
		return b, false, err
	}
	coverKey := strings.TrimSpace(olBook.FirstCoverKey())
	if coverKey == "" {
		return b, false, nil
	}
	coverId, err := strconv.ParseInt(coverKey, 10, 64)
	if err != nil {
		return b, false, fmt.Errorf("invalid Open Library cover ID %q: %w", coverKey, err)
	}
	if coverId == 0 {
		return b, false, nil
	}
	b.OlCoverId = coverId
	if err := db.Save(&b).Error; err != nil {
		return b, false, err
	}
	return b, true, nil
}

func updateCoverFromSearchResult(db *gorm.DB, b Book, result BookSearchResult) (Book, bool, error) {
	if !result.HasCoverImageId() {
		return b, false, nil
	}
	coverId, err := strconv.ParseInt(result.CoverImageId, 10, 64)
	if err != nil {
		return b, false, fmt.Errorf("invalid Open Library cover ID %q: %w", result.CoverImageId, err)
	}
	if coverId == 0 {
		return b, false, nil
	}
	b.OlCoverId = coverId
	if strings.TrimSpace(result.CoverEditionKey) != "" {
		b.OlCoverEditionId = result.CoverEditionKey
	}
	if err := db.Save(&b).Error; err != nil {
		return b, false, err
	}
	return b, true, nil
}

func refreshCoverFromSearch(db *gorm.DB, b Book) (Book, bool, error) {
	var fallback *BookSearchResult
	for _, authorName := range b.AlternateAuthorFullNames() {
		sleepForOpenLibrary()
		results := SearchBook(b.FormatTitle(), authorName)
		selected, ok, hasEnglish := selectCoverSearchResult(results)
		if !ok {
			continue
		}
		if hasEnglish {
			return updateCoverFromSearchResult(db, b, selected)
		}
		if fallback == nil {
			fallback = &selected
		}
	}
	if fallback != nil {
		return updateCoverFromSearchResult(db, b, *fallback)
	}
	log.Printf("No cover found on Open Library search for '%s' by '%s'.", b.FormatTitle(), b.AuthorFullName)
	return b, false, nil
}

func refreshCoverIdFromOpenLibrary(db *gorm.DB, b Book) (Book, bool, error) {
	if b.HasCoverImageId() {
		return b, false, nil
	}
	updated, ok, err := refreshCoverFromEdition(db, b)
	if err != nil {
		return b, false, err
	}
	if ok {
		return updated, true, nil
	}
	return refreshCoverFromSearch(db, b)
}

func captureCoverImage(b Book, outputDir string, size string) {
	imageFile := b.MakeCoverImageFilename(outputDir, size)
	if !shouldDownloadCover(imageFile, outputDir, size) {
		return
	}
	url := b.MakeCoverImageUrl(size)
	err := saveCoverImage(imageFile, url)
	if err != nil {
		log.Printf("ERROR saving cover image id=%d for %q: %v", b.OlCoverId, b.FormatTitle(), err)
		if copyErr := copyPlaceholderImage(imageFile, outputDir, size); copyErr != nil {
			log.Printf("Warning: Failed to copy placeholder image for %s: %v", imageFile, copyErr)
		} else {
			log.Printf("Copied placeholder cover for %s (%s).", b.FormatTitle(), size)
		}
	}
}

func CaptureAllSizeCovers(b Book, imageDir string) {
	captureCoverImage(b, imageDir, SmallCover)
	captureCoverImage(b, imageDir, MediumCover)
	captureCoverImage(b, imageDir, LargeCover)
}

func CaptureCoverImages(db *gorm.DB, books []Book, imageDir string) error {
	err := os.MkdirAll(imageDir, 0775)
	if err != nil {
		return fmt.Errorf("can't create directory for saved cover images: %w", err)
	}
	for _, b := range books {
		bookToProcess := b
		if !bookToProcess.HasCoverImageId() && db != nil {
			log.Printf("Refreshing cover ID for '%s' via Open Library.", bookToProcess.FormatTitle())
			updatedBook, updated, err := refreshCoverIdFromOpenLibrary(db, bookToProcess)
			if err != nil {
				log.Printf("Warning: Failed to refresh cover ID for '%s': %v", bookToProcess.FormatTitle(), err)
			} else if updated {
				bookToProcess = updatedBook
			}
		}
		if !bookToProcess.HasCoverImageId() {
			msg := fmt.Sprint("Can't retrieve cover image for '", b.FormatTitle(), "', cover ID is missing.")
			fmt.Println(msg)
			log.Print(msg)
			continue
		}
		missingSizes := missingCoverSizes(bookToProcess, imageDir)
		if len(missingSizes) == 0 {
			continue
		}
		sleepForOpenLibrary()
		for _, size := range missingSizes {
			captureCoverImage(bookToProcess, imageDir, size)
		}
	}
	return nil
}

func missingCoverSizes(b Book, imageDir string) []string {
	sizes := []string{SmallCover, MediumCover, LargeCover}
	var missing []string
	for _, size := range sizes {
		filename := b.MakeCoverImageFilename(imageDir, size)
		if !coverImageExists(filename) {
			missing = append(missing, size)
			continue
		}
		if b.HasCoverImageId() && coverImageIsPlaceholder(filename, imageDir, size) {
			missing = append(missing, size)
		}
	}
	return missing
}

func coverImageExists(filename string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func shouldDownloadCover(imageFile string, imageDir string, size string) bool {
	if !coverImageExists(imageFile) {
		return true
	}
	return coverImageIsPlaceholder(imageFile, imageDir, size)
}

func coverImageIsPlaceholder(filename string, imageDir string, size string) bool {
	placeholder := path.Join(imageDir, fmt.Sprintf("placeholder-%s.jpg", size))
	placeholderInfo, err := os.Stat(placeholder)
	if err != nil {
		return false
	}
	imageInfo, err := os.Stat(filename)
	if err != nil {
		return false
	}
	if placeholderInfo.Size() != imageInfo.Size() {
		return false
	}
	placeholderBytes, err := os.ReadFile(placeholder)
	if err != nil {
		return false
	}
	imageBytes, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	return bytes.Equal(imageBytes, placeholderBytes)
}

func copyPlaceholderImage(destFile string, imageDir string, size string) error {
	placeholder := path.Join(imageDir, fmt.Sprintf("placeholder-%s.jpg", size))
	if _, err := os.Stat(placeholder); err != nil {
		return err
	}
	src, err := os.Open(placeholder)
	if err != nil {
		return err
	}
	defer src.Close()

	dest, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, src)
	return err
}
