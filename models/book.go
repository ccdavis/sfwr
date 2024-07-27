package models

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ccdavis/sfwr/load"
)

type BooksByAuthor map[string][]Book

type Rating struct {
	slug string
}

func (r Rating) String() string {
	return r.slug
}

var (
	Unknown   = Rating{""}
	VeryGood  = Rating{"Very-Good"}
	Excellent = Rating{"Excellent"}
	Kindle    = Rating{"Kindle"}
	What      = Rating{"?"}
	NotGood   = Rating{"Not-Good"}
)

func stringToRating(s string) (Rating, error) {
	switch s {
	case VeryGood.slug:
		return VeryGood, nil
	case Excellent.slug:
		return Excellent, nil
	case Kindle.slug:
		return Kindle, nil
	case What.slug:
		return What, nil
	case NotGood.slug:
		return NotGood, nil
	}
	return Unknown, errors.New("unknown rating: " + s)
}

type Book struct {
	PubDate          int64
	DateAdded        time.Time
	Author           string
	AuthorSurname    string
	Title            []string
	Review           string
	Rating           Rating
	AmazonLink       string
	CoverImageUrl    string
	OpenLibraryUrl   string
	IsfdbUrl         string
	Isbn             []string
	OlCoverId        int64
	OlAuthorId       []string
	OlCoverEditionId string
}

// This might need to get more sophisticated
func extractSurname(fullName string) string {
	lastName := strings.Split(fullName, " ")
	s := lastName[len(lastName)-1]
	return s
}

func FromRawBook(book load.RawBook) Book {
	year_published, err := book.PubDate.Int64()
	if err != nil {
		fmt.Println("Can't convert publication date ", err)
		os.Exit(1)
	}
	dateAdded := time.Now()
	surname := extractSurname(book.Author)
	rating, err := stringToRating(book.Rating)
	if err != nil {
		fmt.Println("Error extracting author's surname from ", book.Author, ", the error was: ", err)
		os.Exit(1)
	}
	var olCoverId int64
	olCoverId, err = book.OlCoverId.Int64()
	if err != nil {
		olCoverId = -1
		fmt.Println("Problem converting OL cover id on", book.Title[0], "by ", book.Author, ", error was: ", err)
	}

	return Book{
		PubDate:          year_published,
		DateAdded:        dateAdded,
		Author:           book.Author,
		AuthorSurname:    surname,
		Title:            book.Title,
		Review:           book.Review,
		Rating:           rating,
		AmazonLink:       book.AmazonLink,
		CoverImageUrl:    book.CoverImage,
		OpenLibraryUrl:   book.OpenLibrary,
		IsfdbUrl:         book.Isfdb,
		Isbn:             book.Isbn,
		OlCoverId:        olCoverId,
		OlAuthorId:       book.OlAuthorId,
		OlCoverEditionId: book.OlCoverEditionId,
	}
}

func AllBooksFromJson(bookFile string) BooksByAuthor {
	var parsedBooks BooksByAuthor
	loadedBooks := load.MarshalledBookDataFromJsonFile(bookFile)
	for author, books := range loadedBooks {
		parsedBooks.[author] = books
	}
	return parsedBooks
}
