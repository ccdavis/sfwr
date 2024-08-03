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
	AmazonLink       string
	CoverImage       string
	OpenLibrary      string
	Isfdb            string
	Isbn             []string
	OlCoverId        json.Number `json:"ol_cover_id"`
	OlAuthorId       []string
	OlCoverEditionId string
}

func (b RawBook) Print() {
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
	if file_err != nil {
		fmt.Println(file_err)
		os.Exit(1)
	}
	defer json_file.Close()
	byteValue, _ := io.ReadAll(json_file)

	bookData := make(BookMap)
	err := json.Unmarshal(byteValue, &bookData)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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
