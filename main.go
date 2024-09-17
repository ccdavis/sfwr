package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/pages"
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
	byAuthor := pages.RenderBookListPage("templates/by_author.html", pages.BooksByAuthor(books))
	check(os.WriteFile(path.Join(outputDir, "books_by_author.html"), []byte(byAuthor), 0644))

	authorIndex := pages.RenderAuthorIndexPage("templates/author_index.html", authors)
	check(os.WriteFile(path.Join(outputDir, "author_index.html"), []byte(authorIndex), 0644))

	//for _, b := range books {
	//bookPage := pages.RenderBookPage("templates/book.html", b)
	//check(os.WriteFile(path.Join(outputDir, "books", b.SiteFileName()), []byte(bookPage), 0644))
	//}
}

func takeLabeledNumberInput(label string, def int64) (int64, error) {
	var choice int64
	userInput, err := takeLabeledInput(label, "")
	if len(userInput) == 0 {
		choice = def
	} else {
		choice, err = strconv.ParseInt(userInput, 10, 64)
	}

	return choice, err
}

func takeLabeledInput(label string, def string) (string, error) {
	fmt.Print(label, " : ")
	if len(def) > 0 {
		fmt.Print("(", def, ") : ")
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if scanner.Err() == nil && len(scanner.Text()) == 0 && len(def) > 0 {
		return def, nil
	} else {
		return scanner.Text(), scanner.Err()
	}
}

func addBookWithAuthorTui(db *gorm.DB, author models.Author) error {
	var newBook models.Book
	var err error
	fmt.Println("\nAdd New Book by ", author.FullName, "--------------------")
	fmt.Println()
	/*
		db.Model(&author).Association("Books")
		dbError := db.Model(&author).Association("Books").Error
		if dbError != nil {
			return dbError
		}
	*/
	//var authorBooks []models.Book
	//db.Model(&author).Association("Books").Find(&authorBooks)
	if len(author.Books) == 0 {
		fmt.Println("Currently this database contains no books by this author.")
	} else {
		fmt.Println("Books by ", author.FullName, " currently in the database:")
		for index, b := range author.Books {
			fmt.Println(index, ".  ", b.PubDate, ": ", b.FormatTitle())
		}
		fmt.Println()
	}

	finished := false
	for !finished {
		newBook.MainTitle, err = takeLabeledInput("Enter main title", newBook.MainTitle)
		if err != nil {
			return err
		}
		if len(newBook.MainTitle) == 0 {
			fmt.Println("Error: book must have a main title!")
			continue
		}
		newBook.SubTitle, err = takeLabeledInput("Enter subtitle", newBook.SubTitle)
		if err != nil {
			return err
		}

		newBook.PubDate, err = takeLabeledNumberInput("Enter publication year", newBook.PubDate)
		if err != nil {
			return err
		}

		fmt.Println("Rating:")
		fmt.Println("(5) Excellent")
		fmt.Println("(4) Very Good")
		fmt.Println("(3) Kindle only / Self-published")
		fmt.Println("(2) ''Interesting' / What was that?")
		fmt.Println("(1) Not good. Had to put it down.")

		var ratingNumber int64
		var ratingError error
		rating := models.Unknown
		for rating == models.Unknown {
			ratingNumber, ratingError = takeLabeledNumberInput("Enter rating", 0)
			if ratingError != nil {
				fmt.Println("Please enter a rating between 1 and 5.")
				continue
			}
			switch ratingNumber {
			case 1:
				rating = models.NotGood
			case 2:
				rating = models.Interesting
			case 3:
				rating = models.Kindle
			case 4:
				rating = models.VeryGood
			case 5:
				rating = models.Excellent
			}
			if rating == models.Unknown {
				fmt.Println("Please enter a rating between 1 and 5.")
				continue
			}
			newBook.Rating = rating
		}

		newBook.AuthorFullName = author.FullName
		newBook.AuthorSurname = author.Surname
		newBook.DateAdded = time.Now()

		var checkBooks []models.Book
		result := db.Where(&models.Book{MainTitle: newBook.MainTitle}).Find(&checkBooks)
		if len(checkBooks) > 0 && result.Error == nil {
			fmt.Println("\n WARNING! ---- Possible duplicate title!")
			fmt.Println()
			for _, b := range checkBooks {
				fmt.Println(b.FormatTitle(), " ", b.PubDate, " by ", b.AuthorFullName)
			}
			fmt.Print("You entered: ")
			fmt.Println(newBook.MainTitle, ": ", newBook.SubTitle, " ", newBook.PubDate, " by ", newBook.AuthorFullName)
			response, err := takeLabeledInput("Proceed anyhow? Proceeding won't replace the duplicate entry, it will add another with the same main title.(y/n)", "n")
			if err != nil {
				return err
			}
			if response == "n" {
				fmt.Println("Ok, not adding book. Try entering a new one.")
				continue
			}
		}

		fmt.Println("Ready to add new book. Currently the database has these other books by ", newBook.AuthorFullName, "")
		fmt.Println()
		for index, b := range author.Books {
			fmt.Println(index, ": ", b.FormatTitle(), " ", b.PubDate, " by ", b.AuthorFullName)
		}
		fmt.Println("\nReady to add -- ", newBook.FormatTitle(), " ", newBook.PubDate, " by ", newBook.AuthorFullName)
		response, err := takeLabeledInput("Save book? (y/n)", "n")
		if err != nil {
			return err
		}
		if response == "n" {
			fmt.Println("Ok, not adding book. Try entering a new one.")
			continue
		}
		finished = true
	}

	result := db.Create(&newBook)
	if result.Error != nil {
		return result.Error
	}
	db.Model(&newBook).Association("Authors").Append(&author)
	dbError := db.Model(&author).Association("Books").Error
	if dbError != nil {
		fmt.Println("Error adding author to book!", dbError)
	}

	return dbError
}

func findOrCreateAuthorTui(db *gorm.DB) (models.Author, error) {
	fmt.Println("\nFirst, add the author. Then add the book.")
	fmt.Println()
	var err error
	var authorToUse models.Author

	var authorName string
	finishedAuthor := false
	for err == nil && !finishedAuthor {
		fmt.Print("Author's full name: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		err = scanner.Err()
		if err == nil {
			authorName = scanner.Text()
			var newAuthors []models.Author
			result := db.Where("full_name like ?", authorName).Find(&newAuthors)
			if result.Error != nil {
				return authorToUse, result.Error
			}
			if len(newAuthors) > 0 {
				fmt.Println("Authors matching ", authorName, ", pick one:")
				chosenId := int64(newAuthors[0].ID)
				for _, a := range newAuthors {
					fmt.Println(a.ID, ": ", a.FullName)
				}
				authorChoiceLabel := fmt.Sprint("\nType the ID of the author to use, '0' to search again,  or (enter) to choose the first(", chosenId, "):")
				enteredId, idError := takeLabeledNumberInput(authorChoiceLabel, chosenId)
				if idError == nil {
					if enteredId != 0 {
						chosenId = enteredId
					} else {
						continue
					}
				} else {
					err = nil
				}
				err := db.Preload("Books").Find(&authorToUse, chosenId).Error
				return authorToUse, err
			} else {
				// Create new author record
				authorToUse = models.Author{
					FullName: authorName,
					Surname:  models.ExtractSurname(authorName),
				}
				result = db.Create(&authorToUse)
				return authorToUse, result.Error
			}

		}
	}
	return authorToUse, err
}

// Really basic 80s style text entry
func mainMenuTui(db *gorm.DB) {
	var err error
	var choice uint64
	for choice != 2 && err == nil {
		fmt.Println("(1) Add book ")
		fmt.Println("(2) Quit")
		_, err = fmt.Scanf("%d", &choice)
		if err != nil {
			fmt.Println("Choice must be a valid number: ", err)
			continue
		}
		if choice == 1 {
			author, authorError := findOrCreateAuthorTui(db)
			if authorError != nil {
				fmt.Println("Error getting author: ", authorError, ", try again.")
				continue
			}
			bookError := addBookWithAuthorTui(db, author)
			if bookError != nil {
				fmt.Println("Error adding book: ", bookError, ", try again.")
				continue
			} else {
				fmt.Println("Success: Book added.")
			}
		} else if choice == 2 {
			fmt.Println("Quitting")
		} else {
			fmt.Println("Invalid choice: ", choice)
		}
	}
}

func loadAllBooks(db *gorm.DB) []models.Book {
	var allBooks []models.Book
	result := db.Preload("Authors").Find(&allBooks)
	if result.Error != nil {
		log.Fatal("can't retrieve books from sfwr db: ", result.Error)
	} else {
		fmt.Println("Loaded ", result.RowsAffected, " book records.")
	}
	return allBooks
}
func main() {
	var (
		bookFilePtr      = flag.String("load-books", "book_database.json", "A JSON file of book data")
		databaseNamePtr  = flag.String("createdb", "", "Create new database")
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

	if saveImagesFlag {
		allBooks := loadAllBooks(db)
		fmt.Println("Saving cover images...")
		siteCoverImagesDir := path.Join(GeneratedSiteDir, models.ImageDir)
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
	}

	if addBookFlag {
		mainMenuTui(db)
	}
}
