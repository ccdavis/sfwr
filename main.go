package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

const Verbose bool = false
const GeneratedSiteDir string = "output/public"

func readBooksJson(filename string) ([]models.Book, []string) {
	var allBooks []models.Book
	var authors []string
	fmt.Println("Loading books from " + filename)
	parsedBookData := models.AllBooksFromJson(filename)
	for a, books := range parsedBookData {
		authors = append(authors, a)
		allBooks = append(allBooks, books...)
	}
	return allBooks, authors
}

func generateSite(books []models.Book, authors []models.Author, outputDir string) {
	err := os.MkdirAll(outputDir, 0775)
	if err != nil {
		log.Fatal("Can't create output directory for generated site: ", outputDir)
	}

	fmt.Println("Generate static pages...")
	byAuthor := pages.RenderBookListPage("templates/by_author.html", pages.BooksByAuthor(books))
	check(os.WriteFile(path.Join(outputDir, "books_by_author.html"), []byte(byAuthor), 0644))

	authorIndex := pages.RenderAuthorIndexPage(authors)
	check(os.WriteFile(path.Join(outputDir, "author_index.html"), []byte(authorIndex), 0644))

	for _, b := range books {
		bookPage := pages.RenderBookPage(b)
		check(os.WriteFile(path.Join(outputDir, "books", b.SiteFileName()), []byte(bookPage), 0644))
	}
}

func main() {
	var (
		bookFilePtr     = flag.String("load-books", "book_database.json", "A JSON file of book data")
		databaseNamePtr = flag.String("createdb", "", "Create new database")
		saveImagesFlag  bool
	)
	flag.BoolVar(&saveImagesFlag, "getimages", false, "Save small, medium, and large cover images for all books with OLIDs.")
	flag.Parse()
	bookFile := *bookFilePtr

	if *databaseNamePtr != "" {
		var db *gorm.DB = models.CreateBooksDatabase(*databaseNamePtr)
		fmt.Println("Created new database.")
		check(models.TransferJsonBooksToDatabase(bookFile, db))
		fmt.Println("Saved all books to database.")
	}

	databaseName := "sfwr_database.db"
	db, err := gorm.Open(sqlite.Open(databaseName), &gorm.Config{})
	if err != nil {
		log.Fatal("can't open sfwr db. Maybe you need to make it first.")
	}

	var allBooks []models.Book
	var authors []models.Author
	result := db.Find(&allBooks)
	if result.Error != nil {
		log.Fatal("can't retrieve books from sfwr db: ", result.Error)
	} else {
		fmt.Println("Loaded ", result.RowsAffected, " book records.")
	}

	if saveImagesFlag {
		fmt.Println("Saving cover images...")
		siteCoverImagesDir := path.Join(GeneratedSiteDir, models.ImageDir)
		models.CaptureCoverImages(allBooks, siteCoverImagesDir)
	} else {
		result = db.Find(&authors)
		if result.Error != nil {
			log.Fatal("can't retrieve authors from sfwr db: ", result.Error)
		} else {
			fmt.Println("Retrieved ", result.RowsAffected, " author records.")
		}
		generateSite(allBooks, authors, GeneratedSiteDir)
	}
}
