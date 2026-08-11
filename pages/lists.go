package pages

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
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

// Ordering helpers.
//
// Every list here ties on a single field — publication year, surname, date
// added — and hundreds of books share each value. sort.Slice is not stable,
// so ties came out in a different order on every build: rebuilding without
// changing any data reshuffled the home page and the grid. These comparators
// fall through to the title and then the record ID, which are unique, so the
// output is fully determined by the data.

// lessBook breaks a tie on the primary sort key: title first, then author,
// then the record ID so nothing is left to chance.
func lessBook(a, b models.Book) bool {
	if a.MainTitle != b.MainTitle {
		return a.MainTitle < b.MainTitle
	}
	if a.AuthorSurname != b.AuthorSurname {
		return a.AuthorSurname < b.AuthorSurname
	}
	if a.AuthorFullName != b.AuthorFullName {
		return a.AuthorFullName < b.AuthorFullName
	}
	return a.ID < b.ID
}

func lessAuthor(a, b models.Author) bool {
	if a.FullName != b.FullName {
		return a.FullName < b.FullName
	}
	return a.ID < b.ID
}

func BooksByPublicationDate(books []models.Book) []models.Book {
	sort.SliceStable(books, func(left, right int) bool {
		if books[left].PubDate != books[right].PubDate {
			return books[left].PubDate > books[right].PubDate
		}
		return lessBook(books[left], books[right])
	})
	return books
}

func BooksMostRecentlyAdded(books []models.Book, listSize int) []models.Book {
	sorted := make([]models.Book, len(books))
	copy(sorted, books)
	sort.SliceStable(sorted, func(left, right int) bool {
		l, r := sorted[left].DateAdded.Unix(), sorted[right].DateAdded.Unix()
		if l != r {
			return l > r
		}
		return lessBook(sorted[left], sorted[right])
	})
	if listSize > len(sorted) {
		listSize = len(sorted)
	}
	return sorted[:listSize]
}

func BooksByAuthor(books []models.Book) []models.Book {
	sort.SliceStable(books, func(left, right int) bool {
		if books[left].AuthorSurname != books[right].AuthorSurname {
			return books[left].AuthorSurname < books[right].AuthorSurname
		}
		return lessBook(books[left], books[right])
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
	sort.SliceStable(authors, func(left, right int) bool {
		if authors[left].Surname != authors[right].Surname {
			return authors[left].Surname < authors[right].Surname
		}
		return lessAuthor(authors[left], authors[right])
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
	sort.SliceStable(books, func(left, right int) bool {
		if books[left].PubDate != books[right].PubDate {
			return books[left].PubDate < books[right].PubDate
		}
		return lessBook(books[left], books[right])
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

func RenderAuthorIndexPage(fsys fs.FS, authorTemplateFile string, authors []models.Author) (string, error) {
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

	return render(fsys, authorTemplateFile, "base.html", authorChunks)
}

// render parses a layout plus a page template from the template set and
// executes it. The set is an fs.FS so the same code serves both the copy
// compiled into the binary and a directory named in the config.
//
// Errors are returned rather than fatal: the admin server builds the site
// in-process, and a bad template must not take the server down with it.
func render(fsys fs.FS, pageTemplateFile, baseName string, data any) (string, error) {
	t, err := template.ParseFS(fsys, baseName, pageTemplateFile)
	if err != nil {
		return "", fmt.Errorf("could not parse %s with %s: %w", pageTemplateFile, baseName, err)
	}

	var doc bytes.Buffer
	if err := t.Execute(&doc, data); err != nil {
		return "", fmt.Errorf("could not render %s: %w", pageTemplateFile, err)
	}
	return doc.String(), nil
}

func RenderAuthorPage(fsys fs.FS, authorTemplateFile string, author models.Author) (string, error) {
	return render(fsys, authorTemplateFile, "child_dir_base.html", author)
}

func RenderBookPage(fsys fs.FS, bookTemplateFile string, book models.Book) (string, error) {
	return render(fsys, bookTemplateFile, "child_dir_base.html", book)
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

func RenderDecadesIndexPage(fsys fs.FS, decadeTemplateFile string, books []models.Book) (string, error) {
	decadeInfos := GroupBooksByDecade(books)

	return render(fsys, decadeTemplateFile, "base.html", decadeInfos)
}

func RenderDecadePage(fsys fs.FS, decadeTemplateFile string, books []models.Book, decade string) (string, error) {
	sort.SliceStable(books, func(left, right int) bool {
		if books[left].PubDate != books[right].PubDate {
			return books[left].PubDate < books[right].PubDate
		}
		return lessBook(books[left], books[right])
	})

	decadeInfo := DecadeInfo{
		Decade: decade,
		Books:  books,
	}

	return render(fsys, decadeTemplateFile, "child_dir_base.html", decadeInfo)
}

func RenderBookListPage(fsys fs.FS, pageTemplateFile string, books []models.Book) (string, error) {
	return render(fsys, pageTemplateFile, "base.html", books)
}

// SortByAuthorSurname sorts books by author surname alphabetically
func SortByAuthorSurname(books []models.Book) []models.Book {
	sorted := make([]models.Book, len(books))
	copy(sorted, books)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].AuthorSurname != sorted[j].AuthorSurname {
			return sorted[i].AuthorSurname < sorted[j].AuthorSurname
		}
		return lessBook(sorted[i], sorted[j])
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
	sort.SliceStable(authors, func(i, j int) bool {
		if authors[i].Surname != authors[j].Surname {
			return authors[i].Surname < authors[j].Surname
		}
		return lessAuthor(authors[i], authors[j])
	})

	return authors
}
