package models

import (
	"fmt"
	"html/template"
	"strings"
)

func (b Book) DisplayRating() string {
	r, err := StringToRating(b.Rating)
	if err != nil {
		return "Unknown rating"
	}
	return r.Display()
}

func (b Book) FormatTitle() string {
	title := b.MainTitle
	if len(b.SubTitle) > 0 {
		title += ": " + b.SubTitle
	}
	return title
}

func (b Book) FormatRating() string {
	rating, err := StringToRating(b.Rating)
	if err != nil {
		return "Unrated"
	}
	return rating.Display()
}

func (b Book) FormatPubDate() string {
	if b.PubDate == Missing {
		return "  ? "
	}
	return fmt.Sprint(b.PubDate)
}

func (b Book) IndyIcon() template.HTML {
	if !b.Indy {
		return ""
	}
	return template.HTML(`<span class="tag-icon tag-indy" title="Indy / Self-published">&#9998;</span>`)
}

func (b Book) InterestingIcon() template.HTML {
	if !b.Interesting {
		return ""
	}
	return template.HTML(`<span class="tag-icon tag-interesting" title="Interesting / Unusual">?</span>`)
}

func (b Book) TagIcons() template.HTML {
	icons := string(b.IndyIcon()) + string(b.InterestingIcon())
	if icons == "" {
		return ""
	}
	return template.HTML(`<span class="book-tags">` + icons + `</span>`)
}

func (b Book) NumericRating() int {
	r, err := StringToRating(b.Rating)
	if err != nil {
		return 0
	}
	return RatingNumericValue(r)
}

func (b Book) StarRating() template.HTML {
	n := b.NumericRating()
	if n <= 0 {
		return template.HTML(`<span class="star-rating" title="Unrated">&#9734;</span>`)
	}
	stars := strings.Repeat("&#9733;", n)
	return template.HTML(fmt.Sprintf(`<span class="star-rating" title="%s">%s</span>`, b.FormatRating(), stars))
}

func (b Book) ReviewHTML() template.HTML {
	return template.HTML(formatReviewMarkdown(b.Review))
}

func (b Book) ReviewPreviewHTML() template.HTML {
	preview, _ := truncateReviewWords(b.Review, reviewPreviewWordLimit)
	return template.HTML(formatReviewMarkdown(preview))
}

func (b Book) ReviewPreviewIsTruncated() bool {
	_, truncated := truncateReviewWords(b.Review, reviewPreviewWordLimit)
	return truncated
}

func formatReviewMarkdown(review string) string {
	paragraphs := splitReviewParagraphs(review)
	if len(paragraphs) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, paragraph := range paragraphs {
		if i > 0 {
			builder.WriteByte('\n')
		}
		cleaned := strings.Join(strings.Fields(paragraph), " ")
		if cleaned == "" {
			continue
		}
		builder.WriteString("<p>")
		builder.WriteString(template.HTMLEscapeString(cleaned))
		builder.WriteString("</p>")
	}
	return builder.String()
}

func splitReviewParagraphs(review string) []string {
	normalized := normalizeReview(review)
	if normalized == "" {
		return nil
	}

	lines := strings.Split(normalized, "\n")
	var paragraphs []string
	var current []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, "\n"))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, "\n"))
	}
	return paragraphs
}

func truncateReviewWords(review string, maxWords int) (string, bool) {
	if maxWords <= 0 {
		return "", strings.TrimSpace(review) != ""
	}

	paragraphs := splitReviewParagraphs(review)
	if len(paragraphs) == 0 {
		return "", false
	}

	totalWords := 0
	for _, paragraph := range paragraphs {
		totalWords += len(strings.Fields(paragraph))
	}
	if totalWords <= maxWords {
		return strings.Join(paragraphs, "\n\n"), false
	}

	remaining := maxWords
	var preview []string
	for _, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}
		if len(words) <= remaining {
			preview = append(preview, paragraph)
			remaining -= len(words)
		} else {
			preview = append(preview, strings.Join(words[:remaining], " "))
			remaining = 0
		}
		if remaining == 0 {
			break
		}
	}
	return strings.Join(preview, "\n\n"), true
}

func normalizeReview(review string) string {
	normalized := strings.ReplaceAll(review, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.TrimSpace(normalized)
}

func (b Book) BookPageLink(args ...string) template.HTML {
	return template.HTML(fmt.Sprint("<a class=\"buttonlink\" href=\"", b.BookPageURL(args...), "\"> More </a>"))
}

func (b Book) BookPageURL(args ...string) string {
	prefix := b.imageDirPrefix(args)
	return fmt.Sprint(prefix, "/books/", b.SiteFileName())
}

func (b Book) imageDirPrefix(p []string) string {
	if len(p) > 0 {
		return p[0]
	}
	return "."
}

func (b Book) MakeLinkedSmallCoverImageTag(args ...string) template.HTML {
	return b.makeLinkedImageTag(SmallCover, b.imageDirPrefix(args))
}

func (b Book) MakeLinkedMediumCoverImageTag(args ...string) template.HTML {
	return b.makeLinkedImageTag(MediumCover, b.imageDirPrefix(args))
}

func (b Book) MakeLinkedLargeCoverImageTag(args ...string) template.HTML {
	return b.makeLinkedImageTag(LargeCover, b.imageDirPrefix(args))
}

func (b Book) makeLinkedImageTag(size string, relativePath string) template.HTML {
	imageTag := b.makeImageTagForCover(size, relativePath)
	olUrl := b.makeOpenLibraryUrl()
	linkTag := fmt.Sprintf("<a href=\"%s\"> %s </a>", olUrl, imageTag)
	return template.HTML(linkTag)
}

func (b Book) makeImageTagForCover(size string, relativeToImageDir string) template.HTML {
	completePath := relativeToImageDir + "/" + ImageDir
	link := b.MakeCoverImageFilename(completePath, size)
	label := "Open Library"
	tag := fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />", link, label)
	return template.HTML(tag)
}

func (b Book) makeOpenLibraryUrl() string {
	if b.OlCoverEditionId != "" {
		return fmt.Sprintf("http://openlibrary.org/olid/%s", b.OlCoverEditionId)
	}
	searchQuery := strings.ReplaceAll(b.MainTitle+" "+b.AuthorFullName, " ", "+")
	return fmt.Sprintf("https://openlibrary.org/search?q=%s", searchQuery)
}
