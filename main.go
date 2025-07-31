package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
	"github.com/ccdavis/sfwr/tui"
	"github.com/ccdavis/sfwr/web"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

const Verbose bool = false
const GeneratedSiteDir string = "output/public"

func readBooksJson(filename string) ([]models.Book, []string) {
	var allBooks []models.Book
	var authors []string
	fmt.Println("Loading books from " + filename)
	parsedBookData := models.AllBooksFromJson(filename)
	for a, books := range parsedBookData {
		authors = append(authors, a)
		allBooks = append(allBooks, books...)
	}
	return allBooks, authors
}

func generateSite(books []models.Book, authors []models.Author, outputDir string) {
	err := os.MkdirAll(outputDir, 0775)
	if err != nil {
		log.Fatal("Can't create output directory for generated site: ", outputDir)
	}

	fmt.Println("Generate static pages...")
	indexPage := pages.RenderBookListPage("templates/index.html", pages.BooksMostRecentlyAdded(books, 25))
	check(os.WriteFile(path.Join(outputDir, "index.html"), []byte(indexPage), 0644))

	byPubDate := pages.RenderBookListPage("templates/book_list.html", pages.BooksByPublicationDate(books))
	check(os.WriteFile(path.Join(outputDir, "book_list_by_pub_date.html"), []byte(byPubDate), 0644))

	bookGrid := pages.RenderBookListPage("templates/book_boxes.html", pages.BooksByPublicationDate(books))
	check(os.WriteFile(path.Join(outputDir, "book_boxes_by_pub_date.html"), []byte(bookGrid), 0644))

	authorIndex := pages.RenderAuthorIndexPage("templates/author_index.html", authors)
	check(os.WriteFile(path.Join(outputDir, "author_index.html"), []byte(authorIndex), 0644))
	for _, a := range authors {
		authorPage := pages.RenderAuthorPage("templates/author.html", a)
		err = os.MkdirAll(path.Join(outputDir, "authors"), 0775)
		if err != nil {
			log.Fatal("Can't create output directory for generated site: ", outputDir)
		}
		check(os.WriteFile(path.Join(outputDir, "authors", a.SiteName()), []byte(authorPage), 0644))
	}

	decadesIndex := pages.RenderDecadesIndexPage("templates/decades_index.html", books)
	check(os.WriteFile(path.Join(outputDir, "decades_index.html"), []byte(decadesIndex), 0644))
	groupedBooks := pages.BooksByDecade(books)
	for decade, decadeBooks := range groupedBooks {
		decadePage := pages.RenderDecadePage("templates/decade.html", decadeBooks, decade)
		err = os.MkdirAll(path.Join(outputDir, "decades"), 0775)
		if err != nil {
			log.Fatal("Can't create output directory for decades: ", outputDir)
		}
		check(os.WriteFile(path.Join(outputDir, "decades", decade+".html"), []byte(decadePage), 0644))
	}

	for _, b := range books {
		//fmt.Println("Make page for ", b.AuthorFullName, ": ", b.FormatTitle())
		//fmt.Println("Rating ", b.Rating)

		bookPage := pages.RenderBookPage("templates/book.html", b)
		err = os.MkdirAll(path.Join(outputDir, "books"), 0775)
		if err != nil {
			log.Fatal("Can't create output directory for generated site: ", outputDir)
		}
		check(os.WriteFile(path.Join(outputDir, "books", b.SiteFileName()), []byte(bookPage), 0644))
	}
}

func loadAllBooks(db *gorm.DB) []models.Book {
	allBooks, err := models.LoadAllBooks(db)
	if err != nil {
		log.Fatal("can't retrieve books from sfwr db: ", err)
	} else {
		fmt.Println("Loaded ", len(allBooks), " from database.")
	}
	return allBooks
}

func main() {
	var (
		bookFilePtr      = flag.String("load-books", "book_database.json", "A JSON file of book data")
		databaseNamePtr  = flag.String("createdb", "", "Create new database")
		webPortPtr       = flag.String("web", "", "Start web server on specified port (e.g., -web=8080)")
		saveImagesFlag   bool
		addBookFlag      bool
		generateSiteFlag bool
	)
	flag.BoolVar(&saveImagesFlag, "getimages", false, "Save small, medium, and large cover images for all books with OLIDs.")
	flag.BoolVar(&addBookFlag, "new", false, "Add a new book using the basic text interface.")
	flag.BoolVar(&generateSiteFlag, "build", false, "Generate static site")
	flag.Parse()
	bookFile := *bookFilePtr

	if *databaseNamePtr != "" {
		var db *gorm.DB = models.CreateBooksDatabase(*databaseNamePtr)
		fmt.Println("Created new database.")
		check(models.TransferJsonBooksToDatabase(bookFile, db))
		fmt.Println("Saved all books to database.")
	}

	databaseName := "sfwr_database.db"
	db, err := gorm.Open(sqlite.Open(databaseName), &gorm.Config{})
	if err != nil {
		log.Fatal("can't open sfwr db. Maybe you need to make it first.")
	}

	siteCoverImagesDir := path.Join(GeneratedSiteDir, models.ImageDir)
	savedCoverImagesDir := "saved_cover_images"
	
	if saveImagesFlag {
		allBooks := loadAllBooks(db)
		fmt.Println("Saving cover images...")
		models.CaptureCoverImages(allBooks, siteCoverImagesDir)
	}

	if generateSiteFlag {
		allBooks := loadAllBooks(db)
		var authors []models.Author
		result := db.Preload("Books").Find(&authors)
		if result.Error != nil {
			log.Fatal("can't retrieve authors from sfwr db: ", result.Error)
		} else {
			fmt.Println("Retrieved ", result.RowsAffected, " author records.")
		}
		generateSite(allBooks, authors, GeneratedSiteDir)
		
		// Copy all cover images from saved_cover_images to the output directory
		err := copyAllCoverImages(savedCoverImagesDir, siteCoverImagesDir)
		if err != nil {
			log.Printf("Warning: Failed to copy cover images: %v", err)
		}
	}

	if *webPortPtr != "" {
		server := web.NewWebServer(db, savedCoverImagesDir)
		log.Fatal(server.ServeHTTP(*webPortPtr))
	}

	if addBookFlag {
		tui.MainMenuTui(db, savedCoverImagesDir)
	}
}

func copyAllCoverImages(srcDir, destDir string) error {
	// Create destination directory if it doesn't exist
	err := os.MkdirAll(destDir, 0775)
	if err != nil {
		return fmt.Errorf("failed to create destination directory %s: %v", destDir, err)
	}
	
	// Read all files from source directory
	files, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %v", srcDir, err)
	}
	
	// Copy each .jpg file
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".jpg") {
			srcFile := path.Join(srcDir, file.Name())
			destFile := path.Join(destDir, file.Name())
			
			// Open source file
			src, err := os.Open(srcFile)
			if err != nil {
				log.Printf("Warning: Failed to open source image %s: %v", srcFile, err)
				continue
			}
			
			// Create destination file
			dest, err := os.Create(destFile)
			if err != nil {
				src.Close()
				log.Printf("Warning: Failed to create destination file %s: %v", destFile, err)
				continue
			}
			
			// Copy file
			_, err = io.Copy(dest, src)
			src.Close()
			dest.Close()
			
			if err != nil {
				log.Printf("Warning: Failed to copy image %s: %v", srcFile, err)
			}
		}
	}
	
	fmt.Printf("Copied cover images from %s to %s\n", srcDir, destDir)
	return nil
}
