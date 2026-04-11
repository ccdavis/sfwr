package models

import "testing"

func TestIsEnglishLanguageCode(t *testing.T) {
	cases := []struct {
		name     string
		lang     string
		expected bool
	}{
		{name: "en", lang: "en", expected: true},
		{name: "eng", lang: "eng", expected: true},
		{name: "english", lang: "English", expected: true},
		{name: "en-us", lang: "EN-us", expected: true},
		{name: "en-gb", lang: "en-gb", expected: true},
		{name: "en-prefix", lang: "en-ca", expected: true},
		{name: "non-english", lang: "fr", expected: false},
		{name: "empty", lang: "", expected: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEnglishLanguageCode(tt.lang); got != tt.expected {
				t.Fatalf("isEnglishLanguageCode(%q) = %v, want %v", tt.lang, got, tt.expected)
			}
		})
	}
}

func TestHasEnglishLanguage(t *testing.T) {
	withEnglish := BookSearchResult{Languages: []string{"spa", "eng"}}
	if !withEnglish.HasEnglishLanguage() {
		t.Fatalf("expected HasEnglishLanguage to return true")
	}

	withoutEnglish := BookSearchResult{Languages: []string{"spa", "fra"}}
	if withoutEnglish.HasEnglishLanguage() {
		t.Fatalf("expected HasEnglishLanguage to return false")
	}
}

func TestSelectCoverSearchResultPrefersEnglish(t *testing.T) {
	results := []BookSearchResult{
		{CoverImageId: "123", Languages: []string{"spa"}},
		{CoverImageId: "", Languages: []string{"eng"}},
		{CoverImageId: "456", Languages: []string{"eng"}},
	}

	got, ok, hasEnglish := selectCoverSearchResult(results)
	if !ok {
		t.Fatalf("expected ok to be true")
	}
	if !hasEnglish {
		t.Fatalf("expected hasEnglish to be true")
	}
	if got.CoverImageId != "456" {
		t.Fatalf("expected cover 456, got %q", got.CoverImageId)
	}
}

func TestSelectCoverSearchResultFallback(t *testing.T) {
	results := []BookSearchResult{
		{CoverImageId: "123", Languages: []string{"spa"}},
		{CoverImageId: "456", Languages: []string{"fra"}},
	}

	got, ok, hasEnglish := selectCoverSearchResult(results)
	if !ok {
		t.Fatalf("expected ok to be true")
	}
	if hasEnglish {
		t.Fatalf("expected hasEnglish to be false")
	}
	if got.CoverImageId != "123" {
		t.Fatalf("expected cover 123, got %q", got.CoverImageId)
	}
}

func TestSelectCoverSearchResultNoCover(t *testing.T) {
	results := []BookSearchResult{
		{CoverImageId: "", Languages: []string{"eng"}},
		{CoverImageId: " ", Languages: []string{"spa"}},
	}

	_, ok, _ := selectCoverSearchResult(results)
	if ok {
		t.Fatalf("expected ok to be false")
	}
}

func TestBookSearchResultHasCoverImageId(t *testing.T) {
	withCover := BookSearchResult{CoverImageId: "123"}
	if !withCover.HasCoverImageId() {
		t.Fatalf("expected HasCoverImageId to return true")
	}

	withoutCover := BookSearchResult{CoverImageId: " "}
	if withoutCover.HasCoverImageId() {
		t.Fatalf("expected HasCoverImageId to return false")
	}
}
