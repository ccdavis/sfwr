package models

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ccdavis/sfwr/load"
)

const Missing int64 = -999998

type BooksByAuthor map[string][]Book

type Rating struct {
	slug string
}

func (r Rating) String() string {
	return r.slug
}

var (
	Unknown     = Rating{""}
	VeryGood    = Rating{"Very-Good"}
	Excellent   = Rating{"Excellent"}
	Kindle      = Rating{"Kindle"}
	Interesting = Rating{"Interesting"}
	NotGood     = Rating{"Not-Good"}
)

func stringToRating(s string) (Rating, error) {
	switch s {
	case VeryGood.slug:
		return VeryGood, nil
	case Excellent.slug:
		return Excellent, nil
	case Kindle.slug:
		return Kindle, nil
	case Interesting.slug:
		return Interesting, nil
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
	MainTitle        string
	SubTitle         string
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

func (b Book) FormatTitle() string {
	title := b.Title[0]
	if len(b.Title) > 1 {
		title += ": " + b.Title[1]
	}
	return title
}

// This might need to get more sophisticated
func extractSurname(fullName string) string {
	names := strings.Split(fullName, " ")
	if len(names) < 2 {
		fmt.Fprintln(os.Stderr, "WARNING: Can't determine author's surname for full name: ", fullName)
		return names[0]
	} else {
		s := names[len(names)-1]
		return s
	}
}

func FromRawBook(book load.RawBook) Book {
	year_published, err := book.PubDate.Int64()
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nCan't convert publication date ", err)
		book.Print()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr)
		year_published = Missing
	}
	dateAdded := time.Now()
	surname := extractSurname(book.Author)
	rating, err := stringToRating(book.Rating)
	exitOnError("Error extracting author's surname.", err)

	var subTitle = ""
	if len(book.Title) > 1 {
		subTitle = book.Title[1]
	}

	var olCoverId int64
	olCoverId, err = book.OlCoverId.Int64()
	if err != nil {
		var msg bytes.Buffer
		fmt.Fprint(&msg, "Problem converting OL cover id on '", book.Title[0], "' by ", strings.TrimRight(book.Author, "\n"))
		fmt.Fprintln(os.Stderr, msg.String())
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr)
		olCoverId = Missing
	}

	newBook := Book{
		PubDate:          year_published,
		DateAdded:        dateAdded,
		Author:           book.Author,
		AuthorSurname:    surname,
		Title:            book.Title,
		MainTitle:        book.Title[0],
		SubTitle:         subTitle,
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
	return newBook
}

func AllBooksFromJson(bookFile string) BooksByAuthor {
	ret := make(BooksByAuthor)
	loadedBooks := load.MarshalledBookDataFromJsonFile(bookFile)
	for author, rawBooks := range loadedBooks {
		parsedBooks := make([]Book, 0)
		for _, rawBook := range rawBooks {
			parsedBooks = append(parsedBooks, FromRawBook(rawBook))
		}
		ret[author] = parsedBooks
	}
	return ret
}

func exitOnError(msg string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n", msg)
		fmt.Fprintln(os.Stderr, err, "")
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
}
