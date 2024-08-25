package models

import (
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func saveCoverImage(filename string, imageurl string) error {
	response, e := http.Get(imageurl)
	if e != nil {
		return fmt.Errorf("error saving cover image: %w", e)
	}
	defer response.Body.Close()
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("error saving cover image: %w", e)
	}
	defer file.Close()
	_, err = io.Copy(file, response.Body)
	if err != nil {
		log.Fatal(err)
		return fmt.Errorf("Error saving cover image: %w", e)
	}

	return nil
}

func captureCoverImage(b Book, outputDir string, size string) {
	filename := fmt.Sprint(b.OlCoverId, "_", size)
	fullpath := filepath.Join(outputDir, filename)
	url := b.MakeCoverImageUrl(size)
	err := saveCoverImage(fullpath, url)
	if err != nil {
		log.Print("ERROR retrieving or saving image with id ", b.OlAuthorId)
		log.Print("for book: ", b.FormatTitle())
		log.Print("The error was ", err)
	}
}

func CaptureCoverImages(books []Book, imageDir string) error {
	err := os.MkdirAll(imageDir, 0775)
	if err != nil {
		return fmt.Errorf("can't create directory for saved cover images: %w", err)
	}
	for _, b := range books {
		if Missing != b.OlCoverId {
			r := rand.IntN(3)
			randTime := time.Duration(r)
			time.Sleep(randTime * time.Second)
			captureCoverImage(b, imageDir, SmallCover)
			captureCoverImage(b, imageDir, MediumCover)
			captureCoverImage(b, imageDir, LargeCover)
		}
	}
	return nil
}
