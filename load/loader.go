package load

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type RawBook struct {
	PubDate          json.Number `json:"pub_date"`
	Author           string
	Title            []string
	Review           string
	Rating           string
	AmazonLink       string `json:"amazon_link"`
	CoverImage       string `json:"cover_image"`
	OpenLibrary      string `json:"open_library"`
	Isfdb            string
	Isbn             []string
	OlCoverId        json.Number `json:"ol_cover_id"`
	OlAuthorId       []string    `json:"ol_author_id"`
	OlCoverEditionId string      `json:"ol_cover_edition_id"`
}

func (b RawBook) Print() {
	fmt.Println("----------------------------------------------")
	fmt.Print("Title: " + b.Title[0])
	if len(b.Title) > 1 {
		fmt.Println(": " + b.Title[1])
	} else {
		fmt.Println("")
	}
	fmt.Println("by " + b.Author)
	fmt.Println("First published: " + fmt.Sprint(b.PubDate))
	fmt.Println("Rating: " + b.Rating)
	fmt.Println()
}

// The keys are author names, the values are their books
type BookMap map[string][]RawBook

func loadBooks(bookDatabase string) BookMap {
	json_file, file_err := os.Open(bookDatabase)
	check("Error opening book database file.", file_err)

	defer json_file.Close()
	byteValue, _ := io.ReadAll(json_file)

	bookData := make(BookMap)
	err := json.Unmarshal(byteValue, &bookData)
	check("Error unmarshalling book data.", err)
	return bookData
}

func DumpMarshalledBookData(bookData BookMap) {
	for author, books := range bookData {
		fmt.Println()
		fmt.Println("-------  " + author + " -------")
		for _, book := range books {
			book.Print()
		}
	}
}

func MarshalledBookDataFromJsonFile(bookFile string) BookMap {
	message := "Loading books JSON..."
	fmt.Println(message)
	bookData := loadBooks(bookFile)
	// TODO Do some checking on the data
	return bookData
}

func check(msg string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n", msg)
		panic(err)
	}

}
