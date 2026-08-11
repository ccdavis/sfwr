package models

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type itunesResponse struct {
	ResultCount int          `json:"resultCount"`
	Results     []itunesItem `json:"results"`
}

type itunesItem struct {
	TrackName     string `json:"trackName"`
	ArtistName    string `json:"artistName"`
	ArtworkUrl100 string `json:"artworkUrl100"`
	ReleaseDate   string `json:"releaseDate"`
}

type ITunesResult struct {
	Title      string
	Author     string
	PubYear    int
	ArtworkURL string
}

func (r ITunesResult) HasCover() bool {
	return r.ArtworkURL != ""
}

func SearchITunes(title string, author string) ([]ITunesResult, error) {
	term := url.QueryEscape(title + " " + author)
	apiURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&media=ebook&entity=ebook&limit=5", term)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("itunes search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itunes returned status %d", resp.StatusCode)
	}

	var itResp itunesResponse
	if err := json.NewDecoder(resp.Body).Decode(&itResp); err != nil {
		return nil, fmt.Errorf("itunes decode failed: %w", err)
	}

	var results []ITunesResult
	for _, item := range itResp.Results {
		r := ITunesResult{
			Title:      item.TrackName,
			Author:     item.ArtistName,
			PubYear:    parseITunesYear(item.ReleaseDate),
			ArtworkURL: item.ArtworkUrl100,
		}
		results = append(results, r)
	}
	return results, nil
}

func parseITunesYear(dateStr string) int {
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

// ITunesCoverURL adjusts an iTunes artwork URL for the requested size.
func ITunesCoverURL(artworkURL string, size string) string {
	if artworkURL == "" {
		return ""
	}
	switch size {
	case SmallCover:
		return replaceArtworkSize(artworkURL, "100x100")
	case MediumCover:
		return replaceArtworkSize(artworkURL, "200x200")
	case LargeCover:
		return replaceArtworkSize(artworkURL, "600x600")
	}
	return artworkURL
}

func replaceArtworkSize(artworkURL string, newSize string) string {
	// iTunes artwork URLs contain a size like "100x100bb" at the end
	// Replace any NxN pattern with the desired size
	lastSlash := strings.LastIndex(artworkURL, "/")
	if lastSlash < 0 {
		return artworkURL
	}
	filename := artworkURL[lastSlash+1:]
	// Find the dimension pattern (e.g., "100x100")
	for _, oldSize := range []string{"100x100", "200x200", "300x300", "600x600", "60x60"} {
		if strings.Contains(filename, oldSize) {
			return artworkURL[:lastSlash+1] + strings.Replace(filename, oldSize, newSize, 1)
		}
	}
	return artworkURL
}

func sleepForITunes() {
	r := rand.IntN(3) + 2
	time.Sleep(time.Duration(r) * time.Second)
}
