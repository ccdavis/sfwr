package models

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ccdavis/sfwr/load"
)

const Missing int64 = -999998
const ImageDir string = "cover_images"
const Verbose bool = false

type BooksByAuthor map[string][]Book

const SmallCover = "S"
const MediumCover = "M"
const LargeCover = "L"

type Rating struct {
	slug string
}

// For Gorm
func (r Rating) Value() (driver.Value, error) {
	return r.slug, nil
}

func (r *Rating) Scan(value interface{}) error {
	rating, err := StringToRating(value.(string))
	if err != nil {
		*r = rating
	}
	return err
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

func StringToRating(s string) (Rating, error) {
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

type BookAuthor struct {
	gorm.Model
	BookId     uint
	OlAuthorId string
}

type BookIsbn struct {
	gorm.Model
	BookId uint
	Isbn   string
}

type Book struct {
	gorm.Model
	ID               uint
	PubDate          int64
	DateAdded        time.Time
	Author           string
	AuthorSurname    string
	MainTitle        string
	SubTitle         string
	Review           string
	Rating           Rating
	AmazonLink       string
	CoverImageUrl    string
	OpenLibraryUrl   string
	IsfdbUrl         string
	BookIsbns        []BookIsbn
	OlCoverId        int64 // Used as the base ID for the image (add suffix -M, -S, -L for sizing.)
	BookAuthors      []BookAuthor
	OlCoverEditionId string // Used to pull up an entry based on a cover
}

func (b Book) Create(db *gorm.DB) (uint, error) {
	result := db.Create(&b)
	return b.ID, result.Error
}

func (b Book) FormatTitle() string {
	title := b.MainTitle
	if len(b.SubTitle) > 0 {
		title += ": " + b.SubTitle
	}
	return title
}

func (b Book) MakeLinkedSmallCoverImageTag() template.HTML {
	return b.makeLinkedImageTag(SmallCover)
}

func (b Book) MakeLinkedMediumCoverImageTag() template.HTML {
	return b.makeLinkedImageTag(MediumCover)
}

func (b Book) makeLinkedImageTag(size string) template.HTML {
	imageTag := b.makeImageTagForCover(size)
	olUrl := b.makeOpenLibraryUrl()
	linkTag := fmt.Sprintf("<a href=\"%s\"> %s </a>", olUrl, imageTag)
	return template.HTML(linkTag)
}

func (b Book) MakeCoverImageUrl(size string) string {
	if Missing != b.OlCoverId {
		url := fmt.Sprintf("http://covers.openlibrary.org/b/id/%d-%s.jpg", b.OlCoverId, size)
		return url
	} else {
		return ""
	}
}

func (b Book) MakeCoverImageFilename(imageDir string, size string) string {
	filename := fmt.Sprintf("%d-%s.jpg", b.OlCoverId, size)
	return path.Join(imageDir, filename)
}

func MakeCoverImageUrlForIsbn(isbn string, size string) string {
	url := fmt.Sprintf("http://covers.openlibrary.org/b/isbn/%s-%s.jpg", isbn, size)
	return url
}

func (b Book) makeImageTagForCover(size string) template.HTML {
	link := b.MakeCoverImageFilename(ImageDir, size)
	label := "Cover"
	tag := fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", link, label)
	return template.HTML(tag)
}

func (b Book) makeOpenLibraryUrl() string {
	if b.OlCoverEditionId != "" {
		return fmt.Sprintf("http://openlibrary.org/olid/%s", b.OlCoverEditionId)
	} else {
		//TODO  get an isbn
		return ""
	}
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
		if Verbose {
			fmt.Fprintln(os.Stderr, "\nCan't convert publication date ", err)
		}

		if Verbose {
			book.Print()
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr)
		}
		year_published = Missing
	}
	dateAdded := time.Now()
	surname := extractSurname(book.Author)
	rating, err := StringToRating(book.Rating)
	exitOnError("Error extracting author's surname.", err)

	var subTitle = ""
	if len(book.Title) > 1 {
		subTitle = book.Title[1]
	}

	var olCoverId int64
	olCoverId, err = book.OlCoverId.Int64()
	if err != nil {
		var msg bytes.Buffer
		if Verbose {
			fmt.Fprint(&msg, "Problem converting OL cover id on '", book.Title[0], "' by ", strings.TrimRight(book.Author, "\n"))
			fmt.Fprintln(os.Stderr, msg.String())
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr)
		}
		olCoverId = Missing
	}

	var isbns []BookIsbn
	for _, i := range book.Isbn {
		newIsbn := BookIsbn{Isbn: i}
		isbns = append(isbns, newIsbn)
	}

	var authors []BookAuthor
	for _, a := range book.OlAuthorId {
		newAuthor := BookAuthor{OlAuthorId: a}
		authors = append(authors, newAuthor)
	}

	newBook := Book{
		PubDate:          year_published,
		DateAdded:        dateAdded,
		Author:           book.Author,
		AuthorSurname:    surname,
		MainTitle:        book.Title[0],
		SubTitle:         subTitle,
		Review:           book.Review,
		Rating:           rating,
		AmazonLink:       book.AmazonLink,
		CoverImageUrl:    book.CoverImage,
		OpenLibraryUrl:   book.OpenLibrary,
		IsfdbUrl:         book.Isfdb,
		BookIsbns:        isbns,
		OlCoverId:        olCoverId,
		BookAuthors:      authors,
		OlCoverEditionId: book.OlCoverEditionId,
	}

	return newBook
}

func CreateBooksDatabase(databaseName string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(databaseName), &gorm.Config{})
	exitOnError("can't connect to Sqlite database.", err)
	e := db.AutoMigrate(&Book{}, &BookAuthor{}, &BookIsbn{})
	exitOnError("error running migrations: ", e)
	return db
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
