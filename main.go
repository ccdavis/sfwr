package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	bookFilePtr := flag.String("load-books", "book_database.json", "A JSON file of book data")

	flag.Parse()
	bookFile := *bookFilePtr

	fmt.Println("Loading books from " + *bookFilePtr)
	parsedBookData := models.AllBooksFromJson(bookFile)
	fmt.Println("Loaded ", len(parsedBookData), " books.")
	var allBooks []models.Book
	for _, books := range parsedBookData {
		allBooks = append(allBooks, books...)
	}

	byAuthor := pages.RenderPage("templates/by_author.html", pages.BooksByAuthor(allBooks))
	err := os.WriteFile("books_by_author.html", []byte(byAuthor), 0644)
	check(err)
}
