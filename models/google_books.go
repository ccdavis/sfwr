package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errGoogleBooksRateLimited = errors.New("google books API rate limited")

var googleBooksDisabled bool

type googleBooksResponse struct {
	TotalItems int              `json:"totalItems"`
	Items      []googleBookItem `json:"items"`
}

type googleBookItem struct {
	VolumeInfo struct {
		Title         string   `json:"title"`
		Authors       []string `json:"authors"`
		PublishedDate string   `json:"publishedDate"`
		ImageLinks    *struct {
			SmallThumbnail string `json:"smallThumbnail"`
			Thumbnail      string `json:"thumbnail"`
		} `json:"imageLinks"`
	} `json:"volumeInfo"`
}

type GoogleBooksResult struct {
	Title        string
	Authors      []string
	PubYear      int
	ThumbnailURL string
}

func (r GoogleBooksResult) HasCover() bool {
	return r.ThumbnailURL != ""
}

func SearchGoogleBooks(title string, author string) ([]GoogleBooksResult, error) {
	if googleBooksDisabled {
		return nil, errGoogleBooksRateLimited
	}

	q := fmt.Sprintf("intitle:%s+inauthor:%s", url.QueryEscape(title), url.QueryEscape(author))
	apiURL := fmt.Sprintf("https://www.googleapis.com/books/v1/volumes?q=%s&maxResults=5", q)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("google books search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		googleBooksDisabled = true
		log.Printf("Google Books rate limited — skipping Google Books for the rest of this run.")
		return nil, errGoogleBooksRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books returned status %d", resp.StatusCode)
	}

	var gbResp googleBooksResponse
	if err := json.NewDecoder(resp.Body).Decode(&gbResp); err != nil {
		return nil, fmt.Errorf("google books decode failed: %w", err)
	}

	var results []GoogleBooksResult
	for _, item := range gbResp.Items {
		r := GoogleBooksResult{
			Title:   item.VolumeInfo.Title,
			Authors: item.VolumeInfo.Authors,
			PubYear: parseGoogleBooksYear(item.VolumeInfo.PublishedDate),
		}
		if item.VolumeInfo.ImageLinks != nil && item.VolumeInfo.ImageLinks.Thumbnail != "" {
			r.ThumbnailURL = item.VolumeInfo.ImageLinks.Thumbnail
		}
		results = append(results, r)
	}
	return results, nil
}

func parseGoogleBooksYear(dateStr string) int {
	if dateStr == "" {
		return 0
	}
	parts := strings.Split(dateStr, "-")
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return year
}

// GoogleBooksCoverURL adjusts a Google Books thumbnail URL for the requested size.
func GoogleBooksCoverURL(thumbnailURL string, size string) string {
	if thumbnailURL == "" {
		return ""
	}
	u := strings.Replace(thumbnailURL, "&edge=curl", "", 1)
	switch size {
	case SmallCover:
		return replaceOrAddZoom(u, "1")
	case MediumCover:
		return replaceOrAddZoom(u, "2")
	case LargeCover:
		return replaceOrAddZoom(u, "0")
	}
	return u
}

func replaceOrAddZoom(rawURL string, zoom string) string {
	if strings.Contains(rawURL, "zoom=") {
		parts := strings.Split(rawURL, "zoom=")
		if len(parts) == 2 {
			rest := parts[1]
			if idx := strings.Index(rest, "&"); idx >= 0 {
				return parts[0] + "zoom=" + zoom + rest[idx:]
			}
			return parts[0] + "zoom=" + zoom
		}
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&zoom=" + zoom
	}
	return rawURL + "?zoom=" + zoom
}

func sleepForGoogleBooks() {
	r := rand.IntN(3) + 3
	time.Sleep(time.Duration(r) * time.Second)
}
