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
- **Add new book interactively**: `./sfwr -new`
- **Load custom book data**: `./sfwr -load-books custom_file.json`
- **Start web UI server**: `./sfwr -web=8080` (launches web interface on port 8080)

### Development
- **Run without building**: `go run main.go [flags]`
- **Install dependencies**: `go mod tidy`
- **Update dependencies**: `go get -u ./...`

## Architecture

### Core Components
- **Models** (`/models/`): Data structures and database operations using GORM
  - `book.go`: Core Book/Author models with complex rating system
  - `open_library.go`: Open Library API integration for cover images
- **Pages** (`/pages/`): HTML generation logic for different page types
- **Templates** (`/templates/`): HTML templates with embedded CSS
  - `/templates/web/`: Web UI templates for CRUD operations
- **TUI** (`/tui/`): Text interface for adding books interactively
- **Web** (`/web/`): Web UI for book and author management (CRUD operations)

### Data Flow
1. Books stored in `book_database.json` are imported to SQLite database
2. Cover images downloaded from Open Library API using ISBN/OLID
3. Templates populated with database content to generate static HTML
4. Complete static site generated in `output/public/` for deployment

### Database Schema
- **Books**: Main entity with metadata, ratings, Open Library integration
- **Authors**: Many-to-many relationship with books
- **OpenLibraryBookAuthor/ISBN**: External API data storage

## Key Files
- `main.go`: CLI entry point with flag handling
- `sfwr_database.db`: SQLite database file
- `book_database.json`: Source data for book imports
- `output/public/`: Generated static site directory

## Rating System
Uses custom enum: Unknown, VeryGood, Excellent, Kindle, Interesting, NotGood

## Web UI
The web interface provides a browser-based CRUD application for managing books and authors:

### Features
- **Home dashboard** with quick actions and overview
- **Book management**: Create, read, update, delete books
- **Author management**: Create and view authors
- **Form validation** with proper error handling
- **Responsive design** consistent with existing site styling

### Endpoints
- `/` - Home dashboard
- `/books` - List all books
- `/books/new` - Add new book form
- `/books/edit/{id}` - Edit existing book
- `/authors` - List all authors
- `/authors/new` - Add new author form

### Testing
- Unit tests with in-memory SQLite database
- Full CRUD operation testing
- Error handling validation

## Open Library Integration
- Fetches cover images in multiple sizes (S, M, L)
- Uses both ISBN and OLID identifiers
- Handles missing images gracefully