package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"path/filepath"

	"github.com/ccdavis/sfwr/config"
	"github.com/ccdavis/sfwr/models"
	"github.com/ccdavis/sfwr/site"
	"github.com/ccdavis/sfwr/templates"
	"github.com/ccdavis/sfwr/tui"
	"github.com/ccdavis/sfwr/web"
	"github.com/glebarez/sqlite"
	"golang.org/x/term"
	"gorm.io/gorm"
)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

const Verbose bool = false

func loadAllBooks(db *gorm.DB) []models.Book {
	allBooks, err := models.LoadAllBooks(db)
	if err != nil {
		log.Fatal("can't retrieve books from sfwr db: ", err)
	}
	fmt.Println("Loaded ", len(allBooks), " from database.")
	return allBooks
}

func main() {
	var (
		bookFilePtr       = flag.String("load-books", "book_database.json", "A JSON file of book data")
		databaseNamePtr   = flag.String("createdb", "", "Create new database")
		configPathPtr     = flag.String("config", "", "Path to the settings file (default: ./"+config.DefaultFileName+" if present)")
		webPortPtr        = flag.String("web", "", "Start web server on specified port (e.g., -web=8080)")
		bindAddrPtr       = flag.String("bind", "", "Address the web server listens on. Use 0.0.0.0 to accept connections from other machines. Overrides the config file.")
		outputDirPtr      = flag.String("output", "", "Directory to write the generated static site into. Overrides the config file.")
		basePathPtr       = flag.String("base-path", "", "Serve the admin UI under a sub-path, e.g. -base-path=/admin. Overrides the config file.")
		envFilePtr        = flag.String("env-file", "", "Where -set-password writes the password (default: "+config.EnvFileName+" beside the config file)")
		saveImagesFlag    bool
		addBookFlag       bool
		generateSiteFlag  bool
		migrateFlag       bool
		migrateCoversFlag bool
		behindProxyFlag   bool
		hashPasswordFlag  bool
		setPasswordFlag   bool
		fingerprintFlag   bool
	)
	flag.BoolVar(&saveImagesFlag, "getimages", false, "Save cover images for all books (Open Library, Google Books, iTunes fallback).")
	flag.BoolVar(&addBookFlag, "new", false, "Add a new book using the basic text interface.")
	flag.BoolVar(&generateSiteFlag, "build", false, "Generate static site")
	flag.BoolVar(&migrateFlag, "migrate", false, "Run database migrations (rating conversions, new fields)")
	flag.BoolVar(&migrateCoversFlag, "migrate-covers", false, "Migrate cover filenames from OlCoverId to Book.ID scheme")
	flag.BoolVar(&behindProxyFlag, "behind-proxy", false, "The web server sits behind a TLS-terminating reverse proxy: mark cookies Secure and read the client address from X-Forwarded-For.")
	flag.BoolVar(&setPasswordFlag, "set-password", false, "Set the admin password and save it to "+config.EnvFileName+" (prompts; nothing to copy or redirect)")
	flag.BoolVar(&fingerprintFlag, "fingerprint", false, "Print a hash of the collection (books, authors, cover art) for comparing two installations")
	flag.BoolVar(&hashPasswordFlag, "hash-password", false, "Prompt for a password and print its hash without writing any file")
	flag.Parse()
	bookFile := *bookFilePtr

	if hashPasswordFlag {
		if err := printPasswordHash(); err != nil {
			log.Fatal(err)
		}
		return
	}

	settings, err := config.Load(*configPathPtr)
	check(err)

	if setPasswordFlag {
		if err := setPassword(settings, *envFilePtr); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Flags win over the config file so a one-off run can override it.
	if *bindAddrPtr != "" {
		settings.Bind = *bindAddrPtr
	}
	if *outputDirPtr != "" {
		settings.Output = *outputDirPtr
	}
	if *webPortPtr != "" {
		settings.Port = *webPortPtr
	}
	if behindProxyFlag {
		settings.BehindProxy = true
	}
	if *basePathPtr != "" {
		settings.BasePath = config.NormalizeBasePath(*basePathPtr)
	}

	if *databaseNamePtr != "" {
		var db *gorm.DB = models.CreateBooksDatabase(*databaseNamePtr)
		fmt.Println("Created new database.")
		check(models.TransferJsonBooksToDatabase(bookFile, db))
		fmt.Println("Saved all books to database.")
		return
	}

	// The template set: the copy compiled into the binary unless the config
	// names a directory that exists.
	templateFS := templates.FS(settings.Templates)
	if err := templates.Check(templateFS); err != nil {
		log.Fatalf("%v (templates: %s)", err, templates.Source(settings.Templates))
	}

	if fingerprintFlag {
		fp, err := models.TakeFingerprint(mustOpenDatabase(settings), settings.CoverImages)
		check(err)
		fmt.Println(fp)
		return
	}

	startWeb := *webPortPtr != "" || (settings.Port != "" && !generateSiteFlag && !saveImagesFlag && !addBookFlag && !migrateFlag && !migrateCoversFlag)
	needsDB := saveImagesFlag || generateSiteFlag || startWeb || addBookFlag || migrateFlag || migrateCoversFlag
	if !needsDB {
		flag.Usage()
		return
	}

	db := mustOpenDatabase(settings)

	if migrateFlag {
		check(models.MigrateRatingsToTags(db))
		fmt.Println("Migration complete.")
		return
	}

	// Auto-migrate new fields (CoverSource)
	check(db.AutoMigrate(&models.Book{}))

	if migrateCoversFlag {
		fmt.Println("Migrating cover image filenames...")
		check(models.MigrateCovers(db, settings.CoverImages))
		return
	}

	if saveImagesFlag {
		allBooks := loadAllBooks(db)
		fmt.Println("Saving cover images (Open Library → Google Books → iTunes fallback)...")
		models.CaptureCoverImagesWithFallback(db, allBooks, settings.CoverImages)
	}

	if generateSiteFlag {
		summary, err := site.Generate(db, site.Options{
			Templates:      templateFS,
			OutputDir:      settings.Output,
			CoverImagesDir: settings.CoverImages,
		})
		check(err)
		fmt.Println(summary)
	}

	if startWeb {
		check(settings.Check())
		server, err := web.NewWebServer(db, web.Config{
			BindAddr:       settings.Bind,
			Port:           settings.Port,
			PasswordHash:   os.Getenv(web.PasswordHashEnvVar),
			BehindProxy:    settings.BehindProxy,
			DatabasePath:   settings.Database,
			CoverImagesDir: settings.CoverImages,
			OutputDir:      settings.Output,
			Templates:      templateFS,
			TemplatesDir:   templates.Source(settings.Templates),
			RepoDir:        settings.Repo,
			SiteName:       settings.SiteName,
			ConfigFile:     settings.SourceFile,
			BasePath:       settings.BasePath,
		})
		if err != nil {
			log.Fatal("can't start web server: ", err)
		}
		log.Fatal(server.ServeHTTP())
	}

	if addBookFlag {
		tui.MainMenuTui(db, settings.CoverImages)
	}
}

// mustOpenDatabase opens the configured database or exits with a message
// naming the file, which is the usual cause when this fails.
func mustOpenDatabase(settings config.Settings) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(settings.Database), &gorm.Config{})
	if err != nil {
		log.Fatalf("can't open the database at %s. Maybe you need to make it first: %v", settings.Database, err)
	}
	return db
}

// maxPasswordAttempts is how many times readNewPassword re-prompts before
// giving up, so a typo does not mean starting the command over.
const maxPasswordAttempts = 3

// readNewPassword prompts twice without echoing and returns the confirmed
// password. Prompts go to stderr so they are visible even if stdout is
// being captured.
func readNewPassword() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf(
			"a password has to be typed at an interactive terminal.\n" +
				"If you are running this over ssh, run it without a pipe or redirect on stdin")
	}

	for attempt := 1; ; attempt++ {
		fmt.Fprint(os.Stderr, "New admin password: ")
		first, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("could not read password: %w", err)
		}

		fmt.Fprint(os.Stderr, "Repeat password: ")
		second, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("could not read password: %w", err)
		}

		problem := ""
		switch {
		case string(first) != string(second):
			problem = "The two passwords did not match."
		case len(first) < web.MinPasswordLength:
			problem = fmt.Sprintf("Too short: use at least %d characters.", web.MinPasswordLength)
		default:
			return string(first), nil
		}

		if attempt >= maxPasswordAttempts {
			return "", fmt.Errorf("%s Giving up after %d attempts; nothing was changed", problem, attempt)
		}
		fmt.Fprintf(os.Stderr, "%s Try again.\n\n", problem)
	}
}

// setPassword prompts for a new password and writes the hash to the
// environment file itself. Doing the write here rather than asking the user
// to redirect output means a mistyped password can't truncate the existing
// file and lock them out.
func setPassword(settings config.Settings, envPath string) error {
	if envPath == "" {
		envPath = settings.EnvFilePath()
	}
	absPath, err := filepath.Abs(envPath)
	if err != nil {
		return err
	}

	replacing := false
	if _, err := os.Stat(absPath); err == nil {
		replacing = true
		fmt.Fprintf(os.Stderr, "Replacing the password in %s\n\n", absPath)
	} else {
		fmt.Fprintf(os.Stderr, "Writing a new password file at %s\n\n", absPath)
	}

	password, err := readNewPassword()
	if err != nil {
		return err
	}

	hash, err := web.HashPassword(password)
	if err != nil {
		return err
	}
	contents := fmt.Sprintf("export %s='%s'\n", web.PasswordHashEnvVar, hash)

	// Write to a temporary file and rename, so the old password survives
	// intact if anything goes wrong partway.
	temp, err := os.CreateTemp(filepath.Dir(absPath), ".sfwr.env.*")
	if err != nil {
		return fmt.Errorf("could not write next to %s: %w", absPath, err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.WriteString(contents); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, absPath); err != nil {
		return fmt.Errorf("could not update %s: %w", absPath, err)
	}

	verb := "Password set."
	if replacing {
		verb = "Password changed."
	}
	fmt.Fprintf(os.Stderr, "\n%s Saved to %s (readable only by you).\n", verb, absPath)
	fmt.Fprintf(os.Stderr, "\nThe running server still has the old password until you restart it:\n")
	if restart := filepath.Join(filepath.Dir(absPath), "restart.sh"); fileExists(restart) {
		fmt.Fprintf(os.Stderr, "    %s\n", restart)
	} else {
		fmt.Fprintf(os.Stderr, "    stop the server, then:  . %s && ./sfwr -config %s\n", absPath, settings.SourceFile)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// printPasswordHash prints the export line for a password without touching
// any file, for setups that keep secrets somewhere else. Prompts go to
// stderr and only the export line to stdout, so it can be captured.
func printPasswordHash() error {
	password, err := readNewPassword()
	if err != nil {
		return err
	}

	hash, err := web.HashPassword(password)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nAdd this to the environment that runs the server:\n\n")
	fmt.Printf("export %s='%s'\n", web.PasswordHashEnvVar, hash)
	if term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintf(os.Stderr, "\nTo have sfwr write the file for you instead, use -set-password.\n")
	}
	return nil
}
