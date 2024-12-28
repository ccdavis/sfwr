package models

import (
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Open-pi/gol"
)

type BookSearchResult struct {
	Number             int
	FirstYearPublished int
	Title              string
	Authors            []string
	CoverEditionKey    string
	CoverImageId       string
	AuthorIds          []string // Can be more than one author
	isfdb_id           string
}

func (s BookSearchResult) Print() string {
	return fmt.Sprintln(s.FirstYearPublished, "\t", s.Authors[0], ": ", s.Title, "\tCover Image ID: ", s.CoverImageId, "\tCover Edition ID: ", s.CoverEditionKey)
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
						case "cover_edition_key":
							work.CoverEditionKey = strings.ReplaceAll(strings.TrimSpace(fieldValue.String()), "\"", "")
						case "cover_i":
							work.CoverImageId = strings.TrimSpace(fieldValue.String())
						case "id_isfdb":
							work.isfdb_id = strings.TrimSpace(fieldValue.String())
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

func saveCoverImage(filename string, imageurl string) error {
	response, e := http.Get(imageurl)
	if e != nil {
		return fmt.Errorf("error saving cover image: %w", e)
	}
	defer response.Body.Close()
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error saving cover image: %w", e)
	}
	defer file.Close()
	_, err = io.Copy(file, response.Body)
	if err != nil {
		log.Fatal(err)
		return fmt.Errorf("Error saving cover image: %w", e)
	}

	return nil
}

func captureCoverImage(b Book, outputDir string, size string) {
	imageFile := b.MakeCoverImageFilename(outputDir, size)
	url := b.MakeCoverImageUrl(size)
	err := saveCoverImage(imageFile, url)
	if err != nil {
		log.Print("ERROR retrieving or saving image with id ", b.OlCoverId)
		log.Print("for book: ", b.FormatTitle())
		log.Print("The error was ", err)
	}
}

func CaptureAllSizeCovers(b Book, imageDir string) {
	captureCoverImage(b, imageDir, SmallCover)
	captureCoverImage(b, imageDir, MediumCover)
	captureCoverImage(b, imageDir, LargeCover)
}

func CaptureCoverImages(books []Book, imageDir string) error {
	err := os.MkdirAll(imageDir, 0775)
	if err != nil {
		return fmt.Errorf("can't create directory for saved cover images: %w", err)
	}
	for _, b := range books {
		if Missing != b.OlCoverId {
			r := rand.IntN(3)
			randTime := time.Duration(r)
			time.Sleep(randTime * time.Second)
			CaptureAllSizeCovers(b, imageDir)
		}
	}
	return nil
}
