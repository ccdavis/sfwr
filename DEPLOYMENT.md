# SFWR Deployment Guide

Two deployment options for GitHub Pages:

1. **GitHub Actions** - Automatic builds when you push database/images to `main` branch
2. **Local Build Script** - Simple `./deploy.sh` script that builds and pushes to `gh-pages`

Choose one method and follow its setup below.

---

## Method 1: GitHub Actions (Automated)

### Setup

1. **Enable GitHub Pages:**
   - Settings → Pages → Source: **GitHub Actions**

2. **Optional - Git LFS for large files:**
   ```bash
   git lfs install
   git lfs track "sfwr_database.db" "saved_cover_images/**"
   git add .gitattributes && git commit -m "Configure Git LFS"
   ```

3. **Push workflow to GitHub:**

There is already a default workflow deploy.yml in this repo, but you may want to change it.

   ```bash
   git add .github/workflows/deploy.yml
   git commit -m "Add deployment workflow"
   git push origin main
   ```

### Usage

1. Start web interface: `./sfwr -web=8080`
2. Edit books/authors at http://localhost:8080
3. Click **"Deploy to GitHub Pages"**, and in the background it will:
   - Commit database and images to `main` branch
   - Trigger automatic build on GitHub
   - Deploy to `https://YOUR_USERNAME.github.io/YOUR_REPO/`

**Features:**
- Checkpoint system with rollback capability
- Builds happen on GitHub servers
- Requires database/images in git

**Monitoring:** Check `https://github.com/YOUR_USERNAME/YOUR_REPO/actions` (builds take 1-2 minutes)

---

## Method 2: Local Build Script (Simple)

### Setup

1. **Enable GitHub Pages:**
   - Settings → Pages → Source: **Deploy from a branch**
   - Branch: **gh-pages** / (root)

2. **Run first deployment** (auto-configures worktree):
   ```bash
   chmod +x deploy.sh
   ./deploy.sh
   ```

### Usage

Simply run when you want to deploy:

```bash
./deploy.sh
```

**What it does:**
- Cleans `output/` directory
- Runs `./sfwr -build` locally
- Commits to `gh-pages` branch using git worktree
- Pushes to GitHub Pages

**Features:**
- Simple one-script deployment
- Build happens locally (no database in git needed)
- Uses git worktree (no nested repo confusion)

---

## Other Hosting Options

**Netlify:** Build locally → drag `output/public/` to [app.netlify.com/drop](https://app.netlify.com/drop)

**Vercel:** Build locally → `npm i -g vercel && cd output/public && vercel`

**Static App** Build locally → Upload contents of `output/public`

---

## Troubleshooting

### GitHub Actions not triggering
- Must be on `main` branch: `git checkout main`
- Only triggers when `sfwr_database.db` or `saved_cover_images/**` change
- Verify workflow exists: `.github/workflows/deploy.yml`

### Large file warnings
- Use Git LFS: `git lfs install && git lfs track "*.db" "saved_cover_images/**"`
- Or keep database out of git (use Method 2)

### deploy.sh issues
- **"already locked"**: `rm -f output/public/.git/index.lock`
- **Remove worktree**: `git worktree remove output/public`

### General git issues
```bash
# Check remote is configured
git remote -v

# Add if missing
git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO.git
```

---

## Security Notes

- Run web interface locally only (`./sfwr -web=8080`)
- Never expose admin interface to internet
- GitHub Pages sites are public by default
