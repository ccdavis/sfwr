package pages

import (
	"bytes"
	"html/template"
	"log"
	"os"
	"sort"

	"github.com/ccdavis/sfwr/models"
)

// GroupByProperty groups a slice of structs by a specific property.
func GroupByProperty[T any, K comparable](items []T, getProperty func(T) K) map[K][]T {
	grouped := make(map[K][]T)
	for _, item := range items {
		key := getProperty(item)
		grouped[key] = append(grouped[key], item)
	}
	return grouped
}

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
	return books[:listSize]
}

func BooksByAuthor(books []models.Book) []models.Book {
	sort.Slice(books, func(left, right int) bool {
		return books[left].AuthorSurname < books[right].AuthorSurname
	})
	return books
}

func BooksWithRating(books []models.Book, rating models.Rating) (ret []models.Book) {
	for _, b := range books {
		if b.Rating == rating.String() {
			ret = append(ret, b)
		}
	}
	return
}

func AuthorsBySurname(authors []models.Author) map[string][]models.Author {
	sort.Slice(authors, func(left, right int) bool {
		return authors[left].Surname < authors[right].Surname
	})

	groupedBySurname := GroupByProperty(authors, func(a models.Author) string {
		return string(a.Surname[0])
	})

	return groupedBySurname
}

func RenderAuthorIndexPage(authorTemplateFile string, authors []models.Author) string {
	groupedAuthors := AuthorsBySurname(authors)
	var letters []string
	for l, _ := range groupedAuthors {
		letters = append(letters, l)
	}
	sort.Strings(letters)

	var authorChunks [][]models.Author
	for _, l := range letters {
		authorChunks = append(authorChunks, groupedAuthors[l])
	}

	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/base.html", authorTemplateFile)
	err := t.Execute(&doc, authorChunks)
	if err != nil {
		log.Fatal("Error parsing author index template: %w", err)
		os.Exit(1)
	}
	return doc.String()
}

func RenderAuthorPage(authorTemplateFile string, author models.Author) string {
	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/base.html", authorTemplateFile)
	err := t.Execute(&doc, author)
	if err != nil {
		log.Fatal("Error parsing author page template: %w", err)
		os.Exit(1)
	}
	return doc.String()
}

func RenderBookPage(bookTemplateFile string, book models.Book) string {
	var doc bytes.Buffer
	t, parseErr := template.ParseFiles("templates/child_dir_base.html", bookTemplateFile)
	if parseErr != nil {
		log.Fatal("Error parsing book page template: %w", parseErr)
	}
	err := t.Execute(&doc, book)
	if err != nil {
		log.Fatal("Error  rendering book page template: %w", err)
		os.Exit(1)
	}
	return doc.String()
}

func RenderBookListPage(pageTemplateFile string, books []models.Book) string {
	var doc bytes.Buffer
	t, parseErr := template.ParseFiles("templates/base.html", pageTemplateFile)
	if parseErr != nil {
		log.Fatal("Error parsing book list page template: %w", parseErr)
	}

	err := t.Execute(&doc, books)
	if err != nil {
		log.Fatal("Error parsing book list template: %w", err)
		os.Exit(1)
	}
	return doc.String()
}
