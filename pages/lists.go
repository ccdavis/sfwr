package pages

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"sort"

	"github.com/ccdavis/sfwr/models"
)

func BooksByPublicationDate(books []models.Book) []models.Book {
	sort.Slice(books, func(left, right int) bool {
		return books[left].PubDate > books[right].PubDate
	})
	return books
}

func BooksMostRecentlyAdded(books []models.Book, listSize int) []models.Book {
	sort.Slice(books, func(left, right int) bool {
		return books[left].DateAdded.Unix() > books[right].DateAdded.Unix()
	})
	return books
}

func BooksByAuthor(books []models.Book) []models.Book {
	sort.Slice(books, func(left, right int) bool {
		return books[left].AuthorSurname < books[right].AuthorSurname
	})
	return books
}

func BooksWithRating(books []models.Book, rating models.Rating) (ret []models.Book) {
	for _, b := range books {
		if b.Rating == rating {
			ret = append(ret, b)
		}
	}
	return
}

func RenderAuthorIndexPage(authors []models.Author) string {
	return ""

}

func RenderAuthorPage(author string, books []models.Book) string {
	return ""

}

func RenderBookPage(book models.Book) string {
	return ""
}

func RenderBookListPage(pageTemplateFile string, books []models.Book) string {
	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/base.html", pageTemplateFile)

	fmt.Println("Attempt to parse template: ", pageTemplateFile)
	err := t.Execute(&doc, books)
	if err != nil {
		log.Fatal("Error parsing book list template: %w", err)
		os.Exit(1)
	}
	return doc.String()
}
