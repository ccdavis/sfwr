# SFWR Deployment Guide

This guide explains how to deploy your SFWR static site to GitHub Pages.

## Setup Instructions

### 1. Enable GitHub Pages

1. Go to your GitHub repository
2. Click **Settings** → **Pages**
3. Under "Build and deployment":
   - Source: **GitHub Actions**
4. Save the settings

### 2. Configure Git LFS (for cover images)

Since the database and cover images can be large:

```bash
# Initialize Git LFS in your repository
git lfs install

# Track database and image files
git lfs track "sfwr_database.db"
git lfs track "saved_cover_images/**"
git add .gitattributes
git commit -m "Configure Git LFS"
```

### 3. Initial Setup

1. Ensure your repository is connected to GitHub:
   ```bash
   git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO.git
   ```

2. Push the GitHub Actions workflow:
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

Click the **"Deploy to GitHub Pages"** button when you want to:
- Save your current work as a checkpoint
- Update the live website
- Create a recoverable backup point

Each deployment is tagged with `[DEPLOY]` and shows the book/author count.

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

- Check the Actions tab in your repository for error messages
- Ensure GitHub Pages is enabled with "GitHub Actions" as the source
- Verify the workflow file exists at `.github/workflows/deploy.yml`

### Large Files Warning

If Git complains about large files:
1. Ensure Git LFS is properly configured
2. Run `git lfs migrate import --include="*.db,*.jpg,*.png"`

### Deployment Button Not Working

- Ensure you're in a git repository: `git status`
- Check that you have a remote configured: `git remote -v`
- Verify you have push permissions to the repository

## Security Notes

- The web interface should only be run locally
- Never expose the admin interface to the internet
- Database contains all your book data - keep backups
- GitHub Pages sites are public by default