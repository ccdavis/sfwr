package main

import (
	"flag"
	"fmt"

	"github.com/ccdavis/sfwr/models"
)

func main() {
	bookFilePtr := flag.String("load-books", "book_database.json", "A JSON file of book data")

	flag.Parse()
	bookFile := *bookFilePtr

	fmt.Println("Loading books from " + *bookFilePtr)
	parsedBookData := models.AllBooksFromJson(bookFile)
	fmt.Println("Loaded ", len(parsedBookData), " books.")

}
