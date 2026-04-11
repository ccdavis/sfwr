package pages

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"sort"
	"strings"

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
		surname := strings.TrimSpace(a.Surname)
		if surname == "" {
			return "UNKNOWN"
		}
		first := []rune(surname)
		if len(first) == 0 {
			return "UNKNOWN"
		}
		return strings.ToUpper(string(first[0]))
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
	for l := range groupedAuthors {
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
		log.Fatalf("Error parsing author index template: %v", err)
	}
	return doc.String()
}

func RenderAuthorPage(authorTemplateFile string, author models.Author) string {
	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/child_dir_base.html", authorTemplateFile)
	err := t.Execute(&doc, author)
	if err != nil {
		log.Fatalf("Error parsing author page template: %v", err)
	}
	return doc.String()
}

func RenderBookPage(bookTemplateFile string, book models.Book) string {
	var doc bytes.Buffer
	t, parseErr := template.ParseFiles("templates/child_dir_base.html", bookTemplateFile)
	if parseErr != nil {
		log.Fatalf("Error parsing book page template: %v", parseErr)
	}
	err := t.Execute(&doc, book)
	if err != nil {
		log.Fatalf("Error rendering book page template: %v", err)
	}
	return doc.String()
}

type DecadeInfo struct {
	Decade string
	Books  []models.Book
}

// GroupBooksByDecade groups books by their publication decade, newest first,
// with "Unknown" sorted to the end.
func GroupBooksByDecade(books []models.Book) []DecadeInfo {
	groupedBooks := BooksByDecade(books)
	decades := make([]string, 0, len(groupedBooks))
	for d := range groupedBooks {
		decades = append(decades, d)
	}

	sort.Slice(decades, func(i, j int) bool {
		if decades[i] == "Unknown" {
			return false
		}
		if decades[j] == "Unknown" {
			return true
		}
		return decades[i] > decades[j]
	})

	decadeInfos := make([]DecadeInfo, 0, len(decades))
	for _, d := range decades {
		decadeInfos = append(decadeInfos, DecadeInfo{
			Decade: d,
			Books:  groupedBooks[d],
		})
	}
	return decadeInfos
}

func RenderDecadesIndexPage(decadeTemplateFile string, books []models.Book) string {
	decadeInfos := GroupBooksByDecade(books)

	var doc bytes.Buffer
	t, _ := template.ParseFiles("templates/base.html", decadeTemplateFile)
	err := t.Execute(&doc, decadeInfos)
	if err != nil {
		log.Fatalf("Error parsing decades index template: %v", err)
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
		log.Fatalf("Error parsing decade page template: %v", err)
	}
	return doc.String()
}

func RenderBookListPage(pageTemplateFile string, books []models.Book) string {
	var doc bytes.Buffer
	t, parseErr := template.ParseFiles("templates/base.html", pageTemplateFile)
	if parseErr != nil {
		log.Fatalf("Error parsing book list page template: %v", parseErr)
	}

	err := t.Execute(&doc, books)
	if err != nil {
		log.Fatalf("Error parsing book list template: %v", err)
	}
	return doc.String()
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
