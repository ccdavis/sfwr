package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
	"gorm.io/gorm"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

const Verbose bool = false

func readBooksJson(filename string) []models.Book {
	var allBooks []models.Book
	fmt.Println("Loading books from " + filename)
	parsedBookData := models.AllBooksFromJson(filename)
	for _, books := range parsedBookData {
		allBooks = append(allBooks, books...)
	}
	return allBooks
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
		allBooks := readBooksJson(bookFile)
		for _, b := range allBooks {
			id, err := b.Create(db)
			if err != nil {
				log.Fatal("Error saving book %w", err)
			} else {
				if Verbose {
					fmt.Println("Created book ", id)
				}
			}
		}
		fmt.Println("Saved all books to database.")
	}

	if saveImagesFlag {
		allBooks := readBooksJson(bookFile)
		fmt.Println("Saving cover images...")
		models.CaptureCoverImages(allBooks, models.ImageDir)
	} else {
		allBooks := readBooksJson(bookFile)
		fmt.Println("Generate static pages...")
		byAuthor := pages.RenderBookListPage("templates/by_author.html", pages.BooksByAuthor(allBooks))
		err := os.WriteFile("books_by_author.html", []byte(byAuthor), 0644)
		check(err)
	}
}
