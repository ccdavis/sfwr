package models

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/flytam/filenamify"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ccdavis/sfwr/load"
)

const Missing int64 = -999998
const ImageDir string = "images/cover_images"
const Verbose bool = false
const reviewPreviewWordLimit = 100

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
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("unsupported Rating scan type: %T", value)
	}
	rating, err := StringToRating(s)
	if err != nil {
		return err
	}
	*r = rating
	return nil
}

func (r Rating) String() string {
	return r.slug
}

func (r Rating) Display() string {
	return strings.ReplaceAll(r.slug, "-", " ")
}

var (
	Unknown        = Rating{"Not Rated"}
	Excellent      = Rating{"Excellent"}
	VeryGood       = Rating{"Very-Good"}
	WorthReading   = Rating{"Worth-Reading"}
	CouldNotFinish = Rating{"Could-Not-Finish"}
)

func AllRatings() []Rating {
	return []Rating{Excellent, VeryGood, WorthReading, CouldNotFinish}
}

func RatingNumericValue(r Rating) int {
	switch r {
	case Excellent:
		return 4
	case VeryGood:
		return 3
	case WorthReading:
		return 2
	case CouldNotFinish:
		return 1
	}
	return 0
}

func RatingFromNumeric(n int) Rating {
	switch n {
	case 4:
		return Excellent
	case 3:
		return VeryGood
	case 2:
		return WorthReading
	case 1:
		return CouldNotFinish
	}
	return Unknown
}

func StringToRating(s string) (Rating, error) {
	switch s {
	case Excellent.slug:
		return Excellent, nil
	case VeryGood.slug:
		return VeryGood, nil
	case WorthReading.slug:
		return WorthReading, nil
	case CouldNotFinish.slug:
		return CouldNotFinish, nil
	}
	return Unknown, errors.New("unknown rating: " + s)
}

// convertLegacyRating handles old "Kindle", "Interesting", and "Not-Good"
// ratings by converting them to the new system. Sets tag flags as needed.
func convertLegacyRating(s string, indy *bool, interesting *bool) (Rating, error) {
	switch s {
	case "Kindle":
		*indy = true
		return VeryGood, nil
	case "Interesting":
		*interesting = true
		return WorthReading, nil
	case "Not-Good":
		return CouldNotFinish, nil
	}
	return StringToRating(s)
}

type OpenLibraryBookAuthor struct {
	gorm.Model
	BookId     uint
	OlAuthorId string
}

type OpenLibraryBookIsbn struct {
	gorm.Model
	BookId uint
	Isbn   string
}

type Book struct {
	gorm.Model
	PubDate                int64
	DateAdded              time.Time
	AuthorFullName         string
	AuthorSurname          string
	MainTitle              string
	SubTitle               string
	Review                 string
	Rating                 string
	Indy                   bool
	Interesting            bool
	AmazonLink             string
	CoverImageUrl          string
	OpenLibraryUrl         string
	IsfdbUrl               string
	OpenLibraryBookIsbns   []OpenLibraryBookIsbn
	OlCoverId              int64 // Legacy Open Library cover ID, still used for OL API lookups
	OpenLibraryBookAuthors []OpenLibraryBookAuthor
	OlCoverEditionId       string   // Used to pull up an entry based on a cover
	CoverSource            string   // "openlibrary", "googlebooks", "itunes", or ""
	Authors                []Author `gorm:"many2many:book_authors;"`
}

type Author struct {
	gorm.Model
	FullName string
	Surname  string
	Books    []Book `gorm:"many2many:book_authors;"`
}

func (a Author) GetBooks() []Book {
	return a.Books
}

func (a Author) HasBook(book Book) bool {
	for _, b := range a.Books {
		if b.ID == book.ID {
			return true
		}
	}
	return false
}

func (a Author) SiteName() string {
	name, err := filenamify.Filenamify(fmt.Sprint(a.ID, "_", a.FullName), filenamify.Options{})
	if err != nil {
		exitOnError(fmt.Sprint("Can't convert author ", a.FullName, " using filenamify."), err)
	}
	return fmt.Sprint(strings.Replace(name, " ", "-", -1), ".html")
}

func LoadAllBooks(db *gorm.DB) ([]Book, error) {
	var allBooks []Book
	result := db.Preload("Authors").Find(&allBooks)
	return allBooks, result.Error
}

func (b Book) UpdateFromOpenLibrary(db *gorm.DB, olSearchResult BookSearchResult) (Book, error) {
	if olSearchResult.FirstYearPublished != 0 {
		b.PubDate = int64(olSearchResult.FirstYearPublished)
	}

	if len(olSearchResult.CoverEditionKey) > 0 {
		//		fmt.Println("Cover edition id: ", olSearchResult.CoverEditionKey)
		b.OlCoverEditionId = olSearchResult.CoverEditionKey
	} else {
		fmt.Println("Search result had no cover edition ID, not updating.")
	}
	olCoverImageId, err := strconv.Atoi(olSearchResult.CoverImageId)
	if err != nil {
		fmt.Println("Can't convert OL Cover ID '", olSearchResult.CoverImageId, "'.")
	} else {
		b.OlCoverId = int64(olCoverImageId)
		if olCoverImageId != 0 {
			b.CoverSource = "openlibrary"
		}
	}
	result := db.Save(&b)
	return b, result.Error
}

func (b Book) Create(db *gorm.DB) (uint, error) {
	result := db.Create(&b)
	return b.ID, result.Error
}

func (b Book) HasOpenLibraryId() bool {
	return len(b.OlCoverEditionId) > 0 && len(strings.TrimSpace(b.OlCoverEditionId)) > 0
}

func (b Book) HasCover() bool {
	if b.OlCoverId != Missing && b.OlCoverId != 0 {
		return true
	}
	return b.CoverSource == "googlebooks" || b.CoverSource == "itunes"
}

// Deprecated: use HasCover instead. Kept for backward compatibility.
func (b Book) HasCoverImageId() bool {
	return b.HasCover()
}

func (b Book) SiteFileName() string {
	name, err := filenamify.Filenamify(fmt.Sprint(b.ID, "_", b.AuthorFullName, b.MainTitle), filenamify.Options{})
	if err != nil {
		exitOnError(fmt.Sprint("Can't convert book ", b.MainTitle, " using filenamify."), err)
	}
	return fmt.Sprint(strings.Replace(name, " ", "-", -1), ".html")
}


// Some databases like Open Library aren't consistent with their author initials, for instance
// CJ Cherryh vs C.J. Cherryh or C. J. Cherryh. We need an easy way to try all three. With all the
// sorts of names people have this is far from perfect but seems to handle 80% of problem cases in English..
func initialisedName(firstName string) ([]string, bool) {
	initials := strings.Split(firstName, ".")
	periods := strings.Count(firstName, ".")
	if periods == 2 {
		return []string{initials[0], initials[1]}, true
	} else {
		if len(initials) == 1 {
			if len(initials[0]) == 2 {
				return []string{string(initials[0][0]), string(initials[0][1])}, true
			} else {
				return []string{}, false
			}
		} else {
			return []string{}, false
		}
	}
}

func (b Book) AlternateAuthorFullNames() []string {
	removeSurname := " " + b.AuthorSurname
	firstName, success := strings.CutSuffix(b.AuthorFullName, removeSurname)
	if success {
		var names []string
		initials, hasInitials := initialisedName(firstName)
		if !hasInitials {
			return []string{b.AuthorFullName}
		} else {
			combined := strings.Join(initials, "")
			periodNoSpace := strings.Join(initials, ".") + "."
			periodWithSpacing := strings.Join(initials, ". ") + "."
			names = append(names, combined+" "+b.AuthorSurname)
			names = append(names, periodNoSpace+" "+b.AuthorSurname)
			names = append(names, periodWithSpacing+" "+b.AuthorSurname)
			return names
		}
	} else {
		return []string{b.AuthorFullName}
	}
}

func (b Book) MakeCoverImageUrl(size string) string {
	switch b.CoverSource {
	case "googlebooks":
		return GoogleBooksCoverURL(b.CoverImageUrl, size)
	case "itunes":
		return ITunesCoverURL(b.CoverImageUrl, size)
	default:
		if b.OlCoverId != Missing && b.OlCoverId != 0 {
			return fmt.Sprintf("http://covers.openlibrary.org/b/id/%d-%s.jpg", b.OlCoverId, size)
		}
		return fmt.Sprintf("placeholder-%s.jpg", size)
	}
}

func (b Book) MakeCoverImageFilename(imageDir string, size string) string {
	if !b.HasCover() {
		return path.Join(imageDir, fmt.Sprintf("placeholder-%s.jpg", size))
	}
	filename := fmt.Sprintf("book_%d-%s.jpg", b.ID, size)
	return path.Join(imageDir, filename)
}


// This might need to get more sophisticated
func ExtractSurname(fullName string) string {
	trimmed := strings.TrimSpace(fullName)
	if trimmed == "" {
		fmt.Fprintln(os.Stderr, "WARNING: Missing author name; using UNKNOWN for surname.")
		return "UNKNOWN"
	}
	names := strings.Split(trimmed, " ")
	if len(names) < 2 {
		fmt.Fprintln(os.Stderr, "WARNING: Can't determine author's surname for full name: ", trimmed)
		return names[0]
	}
	return names[len(names)-1]
}

func fromRawAuthor(authorFullName string) Author {
	fullName := strings.TrimSpace(authorFullName)
	if fullName == "" {
		fullName = "UNKNOWN"
	}
	surname := ExtractSurname(fullName)
	newAuthor := Author{
		FullName: fullName,
		Surname:  surname,
	}
	return newAuthor
}

func fromRawBook(book load.RawBook) Book {
	var authorObjects []Author

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
	authorName := strings.TrimSpace(book.Author)
	if authorName == "" {
		authorName = "UNKNOWN"
	}
	surname := ExtractSurname(authorName)
	var indy, interesting bool
	rating, err := convertLegacyRating(book.Rating, &indy, &interesting)
	if err != nil {
		if Verbose {
			fmt.Fprintf(os.Stderr, "WARNING: Unrecognized rating %q for '%s', defaulting to Unknown.\n", book.Rating, book.Title[0])
		}
		rating = Unknown
	}

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

	var olIsbns []OpenLibraryBookIsbn
	for _, i := range book.Isbn {
		newIsbn := OpenLibraryBookIsbn{Isbn: i}
		olIsbns = append(olIsbns, newIsbn)
	}

	var olAuthors []OpenLibraryBookAuthor
	for _, a := range book.OlAuthorId {
		newAuthor := OpenLibraryBookAuthor{OlAuthorId: a}
		olAuthors = append(olAuthors, newAuthor)
	}

	newBook := Book{
		PubDate:                year_published,
		DateAdded:              dateAdded,
		AuthorFullName:         authorName,
		AuthorSurname:          surname,
		MainTitle:              book.Title[0],
		SubTitle:               subTitle,
		Review:                 book.Review,
		Rating:                 rating.slug,
		Indy:                   indy,
		Interesting:            interesting,
		AmazonLink:             book.AmazonLink,
		CoverImageUrl:          book.CoverImage,
		OpenLibraryUrl:         book.OpenLibrary,
		IsfdbUrl:               book.Isfdb,
		OpenLibraryBookIsbns:   olIsbns,
		OlCoverId:              olCoverId,
		OpenLibraryBookAuthors: olAuthors,
		OlCoverEditionId:       book.OlCoverEditionId,
		Authors:                authorObjects,
	}

	return newBook
}

func AllBooksFromJson(bookFile string) BooksByAuthor {
	ret := make(BooksByAuthor)
	loadedBooks := load.MarshalledBookDataFromJsonFile(bookFile)
	for author, rawBooks := range loadedBooks {
		fmt.Println("Loading books for author ", author)

		parsedBooks := make([]Book, 0)
		for _, rawBook := range rawBooks {
			parsedBooks = append(parsedBooks, fromRawBook(rawBook))
		}
		ret[author] = parsedBooks
	}
	return ret
}

func TransferJsonBooksToDatabase(jsonFileName string, db *gorm.DB) error {
	parsedBookData := AllBooksFromJson(jsonFileName)
	for a, books := range parsedBookData {
		if Verbose {
			fmt.Println("Save books by ", a)
		}
		// Right now all books have a single author but that should not be
		// the way we model it.
		author := fromRawAuthor(a)
		result := db.Create(&author)
		if result.Error != nil {
			return result.Error
		}

		var authorObjects []Author
		authorObjects = append(authorObjects, author)

		for _, b := range books {
			b.Authors = authorObjects
			result := db.Create(&b)
			if result.Error != nil {
				return result.Error
			}

		}
	}
	return nil
}

func CreateBooksDatabase(databaseName string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(databaseName), &gorm.Config{})
	exitOnError("can't connect to Sqlite database.", err)
	e := db.AutoMigrate(&Book{}, &Author{}, &OpenLibraryBookAuthor{}, &OpenLibraryBookIsbn{})
	exitOnError("error running migrations: ", e)
	return db
}

// MigrateRatingsToTags converts legacy "Kindle" and "Interesting" ratings
// into the Indy/Interesting boolean tags, assigns default ratings, and
// renames "Not-Good" to "Could-Not-Finish".
func MigrateRatingsToTags(db *gorm.DB) error {
	err := db.AutoMigrate(&Book{})
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	result := db.Model(&Book{}).Where("rating = ?", "Kindle").Updates(map[string]interface{}{
		"indy":   true,
		"rating": VeryGood.slug,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		fmt.Printf("Migrated %d 'Kindle' books → Indy + Very-Good\n", result.RowsAffected)
	}

	result = db.Model(&Book{}).Where("rating = ?", "Interesting").Updates(map[string]interface{}{
		"interesting": true,
		"rating":      WorthReading.slug,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		fmt.Printf("Migrated %d 'Interesting' books → Interesting + Worth-Reading\n", result.RowsAffected)
	}

	result = db.Model(&Book{}).Where("rating = ?", "Not-Good").Update("rating", CouldNotFinish.slug)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		fmt.Printf("Migrated %d 'Not-Good' books → Could-Not-Finish\n", result.RowsAffected)
	}

	return nil
}

func exitOnError(msg string, err error) {
	if err != nil {
		log.Fatal(msg, ": ", err)
	}
}
