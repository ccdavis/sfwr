package pages

import (
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

func renderList(books []models.Book) string {
	bookPage := ""
	for _, b := range books {
		bookPage += b.Author

	}
	return bookPage
}
