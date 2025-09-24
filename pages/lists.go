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
	if listSize > len(books) {
		listSize = len(books)
	}
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

func BooksByDecade(books []models.Book) map[string][]models.Book {
	sort.Slice(books, func(left, right int) bool {
		return books[left].PubDate < books[right].PubDate
	})

	groupedByDecade := GroupByProperty(books, func(b models.Book) string {
		if b.PubDate == models.Missing || b.PubDate == 0 {
			return "Unknown"
		}
		decade := (b.PubDate / 10) * 10
		return fmt.Sprintf("%ds", decade)
	})

	return groupedByDecade
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
	t, _ := template.ParseFiles("templates/child_dir_base.html", authorTemplateFile)
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

type DecadeInfo struct {
	Decade string
	Books  []models.Book
}

func RenderDecadesIndexPage(decadeTemplateFile string, books []models.Book) string {
	groupedBooks := BooksByDecade(books)
	var decades []string
	for d, _ := range groupedBooks {
		decades = append(decades, d)
	}
	
	// Sort decades newest to oldest, with "Unknown" at the end
	sort.Slice(decades, func(i, j int) bool {
		if decades[i] == "Unknown" {
			return false
		}
		if decades[j] == "Unknown" {
			return true
		}
		return decades[i] > decades[j] // Reverse alphabetical for newest first
	})

	var decadeInfos []DecadeInfo
	for _, d := range decades {
		decadeInfos = append(decadeInfos, DecadeInfo{
			Decade: d,
			Books:  groupedBooks[d],
		})
	}

	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/base.html", decadeTemplateFile)
	err := t.Execute(&doc, decadeInfos)
	if err != nil {
		log.Fatal("Error parsing decades index template: %w", err)
		os.Exit(1)
	}
	return doc.String()
}

func RenderDecadePage(decadeTemplateFile string, books []models.Book, decade string) string {
	sort.Slice(books, func(left, right int) bool {
		if books[left].PubDate != books[right].PubDate {
			return books[left].PubDate < books[right].PubDate
		}
		return books[left].MainTitle < books[right].MainTitle
	})

	decadeInfo := DecadeInfo{
		Decade: decade,
		Books:  books,
	}

	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/child_dir_base.html", decadeTemplateFile)
	err := t.Execute(&doc, decadeInfo)
	if err != nil {
		log.Fatal("Error parsing decade page template: %w", err)
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

// GroupBooksByDecade groups books by their publication decade
func GroupBooksByDecade(books []models.Book) []DecadeInfo {
	groupedBooks := BooksByDecade(books)
	var decades []string
	for d := range groupedBooks {
		decades = append(decades, d)
	}

	// Sort decades newest to oldest, with "Unknown" at the end
	sort.Slice(decades, func(i, j int) bool {
		if decades[i] == "Unknown" {
			return false
		}
		if decades[j] == "Unknown" {
			return true
		}
		return decades[i] > decades[j]
	})

	var decadeInfos []DecadeInfo
	for _, d := range decades {
		decadeInfos = append(decadeInfos, DecadeInfo{
			Decade: d,
			Books:  groupedBooks[d],
		})
	}
	return decadeInfos
}

// SortByAuthorSurname sorts books by author surname alphabetically
func SortByAuthorSurname(books []models.Book) []models.Book {
	sorted := make([]models.Book, len(books))
	copy(sorted, books)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].AuthorSurname < sorted[j].AuthorSurname
	})
	return sorted
}

// AuthorsFromBooks extracts unique authors from a list of books
func AuthorsFromBooks(books []models.Book) []models.Author {
	authorMap := make(map[uint]models.Author)

	for _, book := range books {
		for _, author := range book.Authors {
			if author.ID != 0 {
				authorMap[author.ID] = author
			}
		}
	}

	var authors []models.Author
	for _, author := range authorMap {
		authors = append(authors, author)
	}

	// Sort authors by surname for consistent output
	sort.Slice(authors, func(i, j int) bool {
		return authors[i].Surname < authors[j].Surname
	})

	return authors
}
