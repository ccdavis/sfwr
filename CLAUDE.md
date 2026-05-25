# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SFWR is a book recommendation web application written in Go that functions as a static site generator. It creates HTML pages for browsing science fiction books with author information, ratings, and cover images sourced from Open Library.

## Common Commands

### Building and Running
- **Build executable**: `go build -o sfwr`
- **Create database**: `./sfwr -createdb sfwr_database.db`
- **Download cover images**: `./sfwr -getimages`
- **Generate static site**: `./sfwr -build` (outputs to `output/public/`)
- **Add new book (TUI)**: `./sfwr -new`
- **Start web UI server**: `./sfwr -web=8080`

### Testing
- **Run all tests**: `go test ./...`
- **Run single package tests**: `go test ./models` or `go test ./web`
- **Run specific test**: `go test ./models -run TestBookCreate`
- **Verbose output**: `go test -v ./...`

Tests use in-memory SQLite databases (`:memory:`) - see `setupTestDB()` helper functions in test files.

### Development
- **Run without building**: `go run main.go [flags]`
- **Install dependencies**: `go mod tidy`

## Architecture

### Core Components
- **models/**: Data structures and database operations using GORM
  - `book.go`: Book/Author models, Rating enum, Open Library data sync
  - `open_library.go`: API client for searching books and fetching cover images
- **pages/**: Static HTML page generation logic
- **templates/**: HTML templates for static site generation
  - `templates/web/`: Templates for the web admin UI
- **web/**: HTTP handlers for CRUD operations and deployment
- **tui/**: Terminal interface for adding books (alternative to web UI)

### Data Flow
1. Books can be added via TUI (`-new`), web UI (`-web`), or imported from JSON (`-createdb`)
2. Open Library API provides cover images and metadata (searched by title/author)
3. Cover images stored in `saved_cover_images/`, copied to output on build
4. `./sfwr -build` generates static HTML in `output/public/`
5. Deployment: push database to GitHub → GitHub Actions builds and deploys to Pages

### Database
SQLite database (`sfwr_database.db`) with GORM. Key tables:
- **books**: Core entity with rating, review, Open Library IDs
- **authors**: Many-to-many with books via `book_authors` join table
- **open_library_book_isbns**, **open_library_book_authors**: External API data

### Rating System
String enum stored in database: `Excellent`, `Very-Good`, `Kindle`, `Interesting`, `Not-Good`, `Not Rated`

## Deployment

GitHub Actions workflow (`.github/workflows/deploy.yml`) triggers on pushes to `sfwr_database.db` or `saved_cover_images/`. The workflow builds the Go executable, runs `./sfwr -build`, and deploys `output/public/` to GitHub Pages.

The web UI provides a "Deploy" button that commits the database and pushes to trigger the workflow.

## Open Library Integration
- Search by title and author, returns multiple potential matches
- Fetches cover images in S/M/L sizes using OLID or cover ID
- Web UI allows selecting search results to auto-populate book metadata