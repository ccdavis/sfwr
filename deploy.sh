#!/bin/bash
set -e

echo "=== SFWR GitHub Pages Deployment ==="
echo

# Check if gh-pages worktree exists
if [ ! -d "output/public/.git" ]; then
    echo "Setting up gh-pages worktree..."

    # Create orphan gh-pages branch if it doesn't exist
    if ! git show-ref --verify --quiet refs/heads/gh-pages; then
        git checkout --orphan gh-pages
        git reset --hard
        git commit --allow-empty -m "Initialize gh-pages"
        git checkout main
    fi

    # Add worktree
    mkdir -p output
    git worktree add output/public gh-pages
    cd output/public
    git rm -rf . 2>/dev/null || true
    cd ../..
    echo "Worktree setup complete."
    echo
fi

# Clean output directory (but keep .git)
echo "Cleaning output directory..."
find output/public -mindepth 1 -not -path "output/public/.git*" -delete

# Build the site
echo "Building site..."
./sfwr -build

# Deploy to gh-pages
echo "Deploying to GitHub Pages..."
cd output/public
git add .
git commit -m "Deploy: $(date '+%Y-%m-%d %H:%M:%S')" || {
    echo "No changes to deploy"
    cd ../..
    exit 0
}
git push origin gh-pages
cd ../..

echo
echo "✓ Deployment complete!"
echo "Your site will be live at: https://YOUR_USERNAME.github.io/YOUR_REPO/"
echo "Note: GitHub Pages may take 1-2 minutes to update"
