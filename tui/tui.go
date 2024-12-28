package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ccdavis/sfwr/models"
	"gorm.io/gorm"
)

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

func searchBookTui() {
	var title string
	var err error
	title, err = takeLabeledInput("Search title", title)
	if err != nil {
		fmt.Println("Error reading title.")
		return
	}
	var author string
	author, err = takeLabeledInput("Search author: ", author)
	if err != nil {
		fmt.Println("Error eading author.")
		return
	}
	searchResults := models.SearchBook(title, author)
	for _, ed := range searchResults {
		fmt.Println(ed.Print())
	}
}

func selectOpenLibraryEditionTui(searchResults []models.BookSearchResult) models.BookSearchResult {
	fmt.Println("Select which edition to use for the update:")
	for num, book := range searchResults {
		fmt.Println(num, ": ", book.Print())
	}
	notChosen := true
	var choice int
	for notChosen {
		editionNumber, err := takeLabeledNumberInput("Select edition", 0)
		if err == nil && int(editionNumber) < len(searchResults) {
			choice = int(editionNumber)
			notChosen = false
		} else {
			fmt.Println("Invalid choice.")
		}
	}
	return searchResults[choice]
}

func findBookTui(db *gorm.DB) (models.Book, error) {
	var book models.Book
	author, err := findAuthorTui(db)
	if err != nil {
		fmt.Println("No author in database.")
		return book, err
	}

	var choice int64 = 9999999
	for err != nil || int(choice) >= len(author.Books) {
		fmt.Println("Select book to update with Open Library data:")
		for num, b := range author.Books {
			fmt.Println(num, ": ", b.MainTitle)
		}
		fmt.Println()
		choice, err = takeLabeledNumberInput("Choose book", 0)
	}
	return author.Books[choice], err
}

func updateBookTui(db *gorm.DB, siteCoverImagesDir string) {
	book, err := findBookTui(db)
	if err != nil {
		fmt.Println("No book found, not updating.")
		return
	}
	updateBookFromOpenLibrary(db, book, siteCoverImagesDir)
}

func updateBookFromOpenLibrary(db *gorm.DB, book models.Book, siteCoverImagesDir string) {
	searchResults := models.SearchBook(book.MainTitle, book.AuthorFullName)
	if len(searchResults) > 0 {
		selectedEdition := selectOpenLibraryEditionTui(searchResults)

		fmt.Println("Use ", selectedEdition.Print())
		b, err := book.UpdateFromOpenLibrary(db, selectedEdition)
		if err != nil {
			fmt.Println("Error saving updated book record: ", err)
		} else {
			models.CaptureAllSizeCovers(b, siteCoverImagesDir)
			fmt.Println("Book updated to ", b)
			fmt.Println()
		}
	} else {
		fmt.Println("No book edition could be found in Open Library using the given title and author of this book.")
	}
}

func addBookWithAuthorTui(db *gorm.DB, author models.Author, siteCoverImagesDir string) error {
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
			newBook.Rating = rating.String()
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
		return dbError
	}
	updateBookFromOpenLibrary(db, newBook, siteCoverImagesDir)
	return nil
}

func findAuthorTui(db *gorm.DB) (models.Author, error) {
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
				authorToUse.FullName = authorName
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
				fmt.Println("No authors with that name.")
			}

		}
	}
	return authorToUse, err
}

func findOrCreateAuthorTui(db *gorm.DB) (models.Author, error) {
	fmt.Println("\nFirst, add the author. Then add the book.")
	fmt.Println()
	authorToUse, err := findAuthorTui(db)
	if err != nil {
		// Create new author record
		newAuthor := models.Author{
			FullName: authorToUse.FullName,
			Surname:  models.ExtractSurname(authorToUse.FullName),
		}
		result := db.Create(&newAuthor)
		return newAuthor, result.Error
	} else {
		return authorToUse, err
	}
}

// Really basic 80s style text entry
func MainMenuTui(db *gorm.DB, siteCoverImagesDir string) {
	var err error
	var choice uint64
	for choice != 4 && err == nil {
		fmt.Println("(1) Add book ")
		fmt.Println("(2) Search Open Library")
		fmt.Println("(3) Update existing book with Open Library data")
		fmt.Println("(4) Quit")
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
			bookError := addBookWithAuthorTui(db, author, siteCoverImagesDir)
			if bookError != nil {
				fmt.Println("Error adding book: ", bookError, ", try again.")
				continue
			} else {
				fmt.Println("Success: Book added.")
			}
		} else if choice == 2 {
			searchBookTui()
		} else if choice == 3 {
			updateBookTui(db, siteCoverImagesDir)
		} else if choice == 4 {
			fmt.Println("Quitting")
		} else {
			fmt.Println("Invalid choice: ", choice)
		}
	}
}
