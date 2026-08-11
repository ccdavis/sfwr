// Package site renders the public static site. It lives apart from main so
// both the CLI and the admin server's "Build" button can call it directly,
// rather than the server shelling out to a copy of the executable.
package site

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
	"gorm.io/gorm"
)

// Options says where to read templates and cover art from and where the
// finished site should be written.
type Options struct {
	TemplatesDir   string
	OutputDir      string
	CoverImagesDir string
}

// Generate loads every book and author and writes the complete static site.
// It returns a short summary suitable for showing in the admin UI.
func Generate(db *gorm.DB, opts Options) (string, error) {
	books, err := models.LoadAllBooks(db)
	if err != nil {
		return "", fmt.Errorf("could not load books: %w", err)
	}

	var authors []models.Author
	if err := db.Preload("Books").Find(&authors).Error; err != nil {
		return "", fmt.Errorf("could not load authors: %w", err)
	}

	if err := Render(books, authors, opts); err != nil {
		return "", err
	}

	coverDest := path.Join(opts.OutputDir, models.ImageDir)
	copied, err := CopyCoverImages(opts.CoverImagesDir, coverDest)
	if err != nil {
		// A site without cover art is still a usable site, so this is
		// reported rather than fatal.
		return fmt.Sprintf("Built %d books and %d authors in %s, but cover images failed: %v",
			len(books), len(authors), opts.OutputDir, err), nil
	}

	return fmt.Sprintf("Built %d books and %d authors into %s (%d cover images).",
		len(books), len(authors), opts.OutputDir, copied), nil
}

// Render writes all the site's pages for an already-loaded collection.
func Render(books []models.Book, authors []models.Author, opts Options) error {
	if err := os.MkdirAll(opts.OutputDir, 0775); err != nil {
		return fmt.Errorf("could not create output directory %s: %w", opts.OutputDir, err)
	}

	if err := writeIndexPages(books, opts); err != nil {
		return err
	}
	if err := writeAuthorPages(authors, opts); err != nil {
		return err
	}
	if err := writeDecadePages(books, opts); err != nil {
		return err
	}
	return writeBookPages(books, opts)
}

func (o Options) template(name string) string {
	return filepath.Join(o.TemplatesDir, name)
}

func writeFile(dir, name, contents string) error {
	target := path.Join(dir, name)
	if err := os.WriteFile(target, []byte(contents), 0644); err != nil {
		return fmt.Errorf("could not write %s: %w", target, err)
	}
	return nil
}

func writeIndexPages(books []models.Book, opts Options) error {
	indexPage, err := pages.RenderBookListPage(opts.template("index.html"), pages.BooksMostRecentlyAdded(books, 25))
	if err != nil {
		return err
	}
	if err := writeFile(opts.OutputDir, "index.html", indexPage); err != nil {
		return err
	}

	byPubDate, err := pages.RenderBookListPage(opts.template("book_list.html"), pages.BooksByPublicationDate(books))
	if err != nil {
		return err
	}
	if err := writeFile(opts.OutputDir, "book_list_by_pub_date.html", byPubDate); err != nil {
		return err
	}

	bookGrid, err := pages.RenderBookListPage(opts.template("book_boxes.html"), pages.BooksByPublicationDate(books))
	if err != nil {
		return err
	}
	return writeFile(opts.OutputDir, "book_boxes_by_pub_date.html", bookGrid)
}

func writeAuthorPages(authors []models.Author, opts Options) error {
	authorIndex, err := pages.RenderAuthorIndexPage(opts.template("author_index.html"), authors)
	if err != nil {
		return err
	}
	if err := writeFile(opts.OutputDir, "author_index.html", authorIndex); err != nil {
		return err
	}

	authorsDir := path.Join(opts.OutputDir, "authors")
	if err := os.MkdirAll(authorsDir, 0775); err != nil {
		return fmt.Errorf("could not create %s: %w", authorsDir, err)
	}
	for _, a := range authors {
		authorPage, err := pages.RenderAuthorPage(opts.template("author.html"), a)
		if err != nil {
			return err
		}
		if err := writeFile(authorsDir, a.SiteName(), authorPage); err != nil {
			return err
		}
	}
	return nil
}

func writeDecadePages(books []models.Book, opts Options) error {
	decadesIndex, err := pages.RenderDecadesIndexPage(opts.template("decades_index.html"), books)
	if err != nil {
		return err
	}
	if err := writeFile(opts.OutputDir, "decades_index.html", decadesIndex); err != nil {
		return err
	}

	decadesDir := path.Join(opts.OutputDir, "decades")
	if err := os.MkdirAll(decadesDir, 0775); err != nil {
		return fmt.Errorf("could not create %s: %w", decadesDir, err)
	}
	for decade, decadeBooks := range pages.BooksByDecade(books) {
		decadePage, err := pages.RenderDecadePage(opts.template("decade.html"), decadeBooks, decade)
		if err != nil {
			return err
		}
		if err := writeFile(decadesDir, decade+".html", decadePage); err != nil {
			return err
		}
	}
	return nil
}

func writeBookPages(books []models.Book, opts Options) error {
	booksDir := path.Join(opts.OutputDir, "books")
	if err := os.MkdirAll(booksDir, 0775); err != nil {
		return fmt.Errorf("could not create %s: %w", booksDir, err)
	}
	for _, b := range books {
		bookPage, err := pages.RenderBookPage(opts.template("book.html"), b)
		if err != nil {
			return err
		}
		if err := writeFile(booksDir, b.SiteFileName(), bookPage); err != nil {
			return err
		}
	}
	return nil
}

// CopyCoverImages copies the saved cover art into the generated site and
// returns how many files were copied.
func CopyCoverImages(srcDir, destDir string) (int, error) {
	if err := os.MkdirAll(destDir, 0775); err != nil {
		return 0, fmt.Errorf("could not create %s: %w", destDir, err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, fmt.Errorf("could not read %s: %w", srcDir, err)
	}

	copied := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jpg") {
			continue
		}
		if err := copyFile(path.Join(srcDir, entry.Name()), path.Join(destDir, entry.Name())); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

func copyFile(srcPath, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", srcPath, err)
	}
	defer src.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("could not create %s: %w", destPath, err)
	}

	if _, err := io.Copy(dest, src); err != nil {
		dest.Close()
		return fmt.Errorf("could not copy %s: %w", srcPath, err)
	}
	return dest.Close()
}
