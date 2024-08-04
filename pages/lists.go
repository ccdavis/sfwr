package pages

import (
	"bytes"
	"html/template"
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

func RenderList(books []models.Book) string {
	var doc bytes.Buffer
	tmpl := "<div> <table> <th> <td> Title</td><td> Author</td> </th>"
	tmpl += "{{range .}}"
	tmpl += "<tr> <td> {{.Title}}</td><td> {{.Author}}</td></tr>"
	tmpl += "{{end}}"
	tmpl += "</table>"
	tmpl += "</div>"
	t, err := template.New("book-list").Parse(tmpl)
	if err != nil {
		panic(err)
	}
	err = t.Execute(&doc, books)
	if err != nil {
		panic(err)
	}
	return doc.String()

}

func RenderPage(pageTemplateFile string, innerContent string) string {
	var doc bytes.Buffer
	t, _ := template.ParseFiles(pageTemplateFile)

	err := t.Execute(&doc, innerContent)
	if err != nil {
		panic(err)
	}
	return doc.String()
}
