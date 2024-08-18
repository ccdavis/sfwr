package models

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func saveCoverImage(filename string, imageurl string) error {
	response, e := http.Get(imageurl)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	file, err := os.Create(filename)
	if err != nil {
		return e
	}
	defer file.Close()
	_, err = io.Copy(file, response.Body)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}

func captureCoverImage(b Book, outputDir string, size string) {
	filename := fmt.Sprint(b.OlCoverId, "_", size)
	url := b.MakeCoverImageUrl(size)
	err := saveCoverImage(filename, url)
	if err != nil {
		fmt.Print("ERROR retrieving or saving image with id ", b.OlAuthorId)
		fmt.Print("for book: ", b.FormatTitle())
		fmt.Print("The error was ", err)
	}
}

func captureCoverImages(books []Book, imageDir string) {
	for _, b := range books {
		if Missing != b.OlCoverId {
			captureCoverImage(b, imageDir, SmallCover)
			captureCoverImage(b, imageDir, MediumCover)
			captureCoverImage(b, imageDir, LargeCover)
		}
	}

}
