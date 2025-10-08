# Getting Started with SFWR

This guide helps you set up your own SFWR book catalog from scratch.

## Quick Start

### 1. Fork This Repository (Recommended)

1. Click the **"Fork"** button at the top of the repository page
2. This creates your own independent copy that you can customize
3. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/sfwr.git
   cd sfwr
   ```

Your fork is completely independent - you can modify templates, styles, and features without affecting the original repository.

### 2. Install Dependencies

```bash
# Install Go (if not already installed)
# See https://golang.org/dl/

# Install Git LFS (optional but recommended)
# Ubuntu/Debian:
sudo apt-get install git-lfs

# macOS:
brew install git-lfs

# Windows: Download from https://git-lfs.github.com/

# Initialize Git LFS (only needed if installed)
git lfs install

# Build the application
go build -o sfwr
```

### 3. Initialize Your Database

```bash
# Create a fresh database
./sfwr -createdb sfwr_database.db

# Start the web interface
./sfwr -web=8080
```

Open http://localhost:8080 and start adding books!

### 4. Enable GitHub Pages

1. Go to your repository on GitHub
2. Click **Settings** → **Pages**
3. Under "Source", select **GitHub Actions**
4. Save

### 5. Deploy Your First Site

1. Add some books through the web interface
2. Click **"Deploy to GitHub Pages"** on the home page
3. Your site will be live at: `https://YOUR_USERNAME.github.io/YOUR_REPO/`

## Using a Custom Domain

### Setting Up Your Domain

#### 1. Configure DNS

Add one of these DNS records at your domain registrar:

**For apex domain (example.com):**
```
A     @     185.199.108.153
A     @     185.199.109.153
A     @     185.199.110.153
A     @     185.199.111.153
```

**For subdomain (books.example.com):**
```
CNAME    books    YOUR_USERNAME.github.io
```

#### 2. Configure GitHub Pages

1. Go to **Settings** → **Pages** in your repository
2. Under "Custom domain", enter your domain (e.g., `books.example.com`)
3. Check "Enforce HTTPS" (may take up to 24 hours to be available)
4. Save

#### 3. Add CNAME File

Create a file named `CNAME` in your repository root:
```bash
echo "books.example.com" > CNAME
git add CNAME
git commit -m "Add custom domain"
git push
```

#### 4. Update the Workflow

Edit `.github/workflows/deploy.yml` to preserve your CNAME:

```yaml
      - name: Generate static site
        run: |
          ./sfwr -build
          cp -r saved_cover_images output/public/
          # Preserve CNAME for custom domain
          if [ -f CNAME ]; then
            cp CNAME output/public/
          fi
```

## Starting Fresh (Clean Database)

If you cloned this repo and want to start with your own books:

```bash
# Remove the existing database
rm sfwr_database.db

# Remove existing cover images
rm -rf saved_cover_images

# Create your own database
./sfwr -createdb sfwr_database.db

# Start fresh!
./sfwr -web=8080
```

## Importing Your Book Data

### From JSON

If you have book data in JSON format:

```bash
./sfwr -load-books my_books.json
```

JSON format example:
```json
[
  {
    "main_title": "Dune",
    "sub_title": "",
    "authors": ["Frank Herbert"],
    "publication_year": 1965,
    "isbn": "9780441172719",
    "rating": "Excellent"
  }
]
```

### Using the Interactive TUI

```bash
./sfwr -new
```

Follow the prompts to add books one by one.

## Customizing Your Site

### Modifying Templates

The HTML templates are in `/templates/`:
- `index.html` - Homepage
- `book_list.html` - Book list view
- `book_boxes.html` - Grid view
- `author.html` - Author pages
- `decade.html` - Books by decade

### Changing Styles

CSS is embedded in the templates. Look for `<style>` tags in the template files.

### Site Structure

After building, your site structure will be:
```
output/public/
├── index.html
├── book_list_by_pub_date.html
├── book_boxes_by_pub_date.html
├── authors/
│   └── [author-name].html
├── decades/
│   └── [decade].html
└── saved_cover_images/
    └── [isbn-size].jpg
```

## Backup and Recovery

### Backup Your Data

```bash
# Backup database and images
tar -czf sfwr-backup-$(date +%Y%m%d).tar.gz sfwr_database.db saved_cover_images/
```

### Restore from Backup

```bash
tar -xzf sfwr-backup-20240101.tar.gz
```

## Migrating from Another System

### From Goodreads

1. Export your Goodreads library as CSV
2. Convert to JSON format (use a script or online converter)
3. Import: `./sfwr -load-books goodreads_export.json`

### From LibraryThing

Similar process - export, convert to JSON, import.

## Troubleshooting

### "Not in a git repository" Error

```bash
git init
git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO.git
```

### GitHub Pages Not Building

1. Check **Settings** → **Pages** → Source is "GitHub Actions"
2. Check the **Actions** tab for error messages
3. Ensure the workflow file exists at `.github/workflows/deploy.yml`

### Custom Domain Not Working

- DNS changes can take up to 48 hours to propagate
- Verify DNS settings: `dig books.example.com`
- Check GitHub Pages settings show your domain
- Ensure CNAME file is in repository root

### Large Repository Warning

If you have many cover images, Git may warn about large files. Install and configure Git LFS:

```bash
# Install Git LFS first (if not already installed)
# Ubuntu/Debian: sudo apt-get install git-lfs
# macOS: brew install git-lfs
# Windows: https://git-lfs.github.com/

# Set up Git LFS
git lfs install
git lfs track "*.jpg" "*.png" "*.db"
git add .gitattributes
git lfs migrate import --include="*.jpg,*.png,*.db"
git push --force
```

## Advanced Usage

### Multiple Sites

You can maintain different book collections:

```bash
# Science Fiction collection
./sfwr -createdb scifi.db -web=8080

# Mystery collection
./sfwr -createdb mystery.db -web=8081
```

### API Integration

The Open Library integration fetches cover images:

```bash
# Download all missing covers
./sfwr -getimages

# Or use the web UI to fetch individual book covers
```

## Contributing

If you improve the templates or add features, consider contributing back to the original repository!

## Support

- Report issues: [GitHub Issues](https://github.com/original/sfwr/issues)
- Documentation: See CLAUDE.md for codebase details
- Deployment help: See DEPLOYMENT.md for hosting options