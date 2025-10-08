# SFWR Deployment Guide

This guide explains how to deploy your SFWR static site to GitHub Pages.

## Prerequisites

**IMPORTANT:** For deployment to work, you **must be on the `main` branch**. GitHub Actions only triggers when you push to `main`, not other branches.

To check your current branch:
```bash
git branch --show-current
```

If you're not on `main`, switch to it:
```bash
git checkout main
```

## Setup Instructions

### 1. Enable GitHub Pages

1. Go to your GitHub repository
2. Click **Settings** → **Pages**
3. Under "Build and deployment":
   - Source: **GitHub Actions**
4. Save the settings
5. Your site will be published at: `https://YOUR_USERNAME.github.io/YOUR_REPO/`
   - Example: If your username is `johndoe` and repo is `sfwr`, your site will be at `https://johndoe.github.io/sfwr/`

### 2. Configure Git LFS (Optional, for large files)

Git LFS helps manage large files like databases and images. While optional, it's recommended if you have many cover images.

**Install Git LFS:**
```bash
# Ubuntu/Debian
sudo apt-get install git-lfs

# macOS
brew install git-lfs

# Windows
# Download from https://git-lfs.github.com/
```

**Configure Git LFS:**
```bash
# Initialize Git LFS
git lfs install

# Track database and image files
git lfs track "sfwr_database.db"
git lfs track "saved_cover_images/**"
git add .gitattributes
git commit -m "Configure Git LFS"
```

**Note:** If you skip Git LFS, large files may trigger warnings from GitHub. The site will still work, but you may hit repository size limits with many images.

### 3. Initial Setup

1. Ensure your repository is connected to GitHub:
   ```bash
   git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO.git
   ```

2. Ensure `.gitignore` is present (should already exist in the repo):
   ```bash
   # The .gitignore file excludes the output/ directory from version control
   # This prevents issues when switching branches
   cat .gitignore
   ```

3. Push the GitHub Actions workflow:
   ```bash
   git add .github/workflows/deploy.yml
   git commit -m "Add deployment workflow"
   git push origin main
   ```

## Usage

### Local Admin Interface

1. Start the web interface:
   ```bash
   ./sfwr -web=8080
   ```

2. Open http://localhost:8080 in your browser

3. Add/edit books and authors as needed

### Deployment & Version Control

#### The Checkpoint System

SFWR uses a checkpoint-based deployment system:

1. **Edit freely** - Make any changes you want through the web interface
2. **Deploy = Save Checkpoint** - Click "Deploy to GitHub Pages" to:
   - Save your current database state as a checkpoint
   - Push to GitHub
   - Update your live site
3. **Continue editing** - Keep making changes without affecting the live site
4. **Deploy again or rollback** - Either save new changes or revert to any previous deployment

#### Deploying Your Changes

**Before deploying, ensure you're on the `main` branch:**
```bash
git checkout main
```

Click the **"Deploy to GitHub Pages"** button when you want to:
- Save your current work as a checkpoint
- Push changes to the `main` branch on GitHub
- Trigger GitHub Actions to build and publish your site
- Create a recoverable backup point

Each deployment is tagged with `[DEPLOY]` and shows the book/author count.

**How it works:**
1. The deploy button commits `sfwr_database.db` and `saved_cover_images/` to git
2. Pushes the commit to the `main` branch on GitHub
3. GitHub Actions detects changes to these files and automatically builds your site
4. Your updated site goes live at `https://YOUR_USERNAME.github.io/YOUR_REPO/`

**Monitoring the deployment:**
- Visit `https://github.com/YOUR_USERNAME/YOUR_REPO/actions` to watch the build progress
- Builds typically complete in 1-2 minutes
- If the build fails, check the Actions tab for error messages

#### Rolling Back Changes

If you've made changes you want to undo:

1. Click **"Deployment History"**
2. Find the checkpoint you want to restore
3. Click **"Rollback"** next to that deployment

This will restore your database to exactly how it was at that deployment.

#### Build Locally (Without Deploying)

Click the **"Build Locally"** button to generate the static site in `output/public/` without creating a checkpoint. Use this for:
- Previewing changes before deploying
- Manual uploads to other hosting services

## Alternative Hosting Options

### Netlify

1. Build locally using the "Build Locally" button
2. Drag the `output/public/` folder to [Netlify Drop](https://app.netlify.com/drop)

### Vercel

1. Build locally
2. Install Vercel CLI: `npm i -g vercel`
3. Deploy: `cd output/public && vercel`

### Static.app

1. Build locally
2. Compress: `cd output && tar -czf site.tar.gz public/`
3. Upload `site.tar.gz` to your static.app account

## Troubleshooting

### GitHub Actions Not Running

If you don't see any builds in the Actions tab after pushing:

1. **Wrong branch** - GitHub Actions only triggers on the `main` branch
   ```bash
   # Check your current branch
   git branch --show-current

   # If not on main, switch to it
   git checkout main
   ```

2. **No file changes detected** - The workflow only runs when these files change:
   - `sfwr_database.db`
   - `saved_cover_images/**`

   If you only changed code files, the workflow won't trigger.

3. **GitHub Pages not enabled** - Ensure GitHub Pages is enabled with "GitHub Actions" as the source in Settings → Pages

4. **Workflow file missing** - Verify `.github/workflows/deploy.yml` exists in your repository

5. **Check the Actions tab** - Visit `https://github.com/YOUR_USERNAME/YOUR_REPO/actions` for error messages

### Large Files Warning

If Git complains about large files:
1. Install Git LFS (see setup instructions above)
2. Configure Git LFS: `git lfs install`
3. Migrate existing files: `git lfs migrate import --include="*.db,*.jpg,*.png"`

Alternatively, you can use Git without LFS for small collections (fewer than 50-100 books).

### Deployment Button Not Working

If the deploy button doesn't trigger a build:

1. **Wrong branch** - You must be on `main`
   ```bash
   git checkout main
   ```

2. **Not in a git repository**
   ```bash
   git status
   ```

3. **No remote configured**
   ```bash
   git remote -v
   # If empty, add your remote
   git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO.git
   ```

4. **No push permissions** - Verify you can push to the repository

5. **No changes to commit** - The deploy button will push even without changes, but won't create a new checkpoint. Check the web UI message after clicking deploy.

## Security Notes

- The web interface should only be run locally
- Never expose the admin interface to the internet
- Database contains all your book data - keep backups
- GitHub Pages sites are public by default