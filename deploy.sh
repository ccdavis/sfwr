#!/bin/bash
set -e

echo "=== SFWR GitHub Pages Deployment ==="
echo

WORKTREE_DIR="output/public"

# Remove the gh-pages worktree and prune its admin files.
# Safe to call repeatedly, even when the worktree doesn't exist.
cleanup_worktree() {
    git worktree remove --force "${WORKTREE_DIR}" 2>/dev/null || true
    rm -rf "${WORKTREE_DIR}"
    git worktree prune 2>/dev/null || true
}

# Always tear down the worktree on exit — success, failure, or Ctrl-C — so the
# gh-pages branch is never left checked out in a worktree. (A branch checked out
# in a worktree can't be checked out anywhere else, which is what blocks
# `git checkout main` and leaves the repo feeling "stuck".)
trap cleanup_worktree EXIT

# Start from a clean slate in case a previous run was interrupted mid-deploy.
cleanup_worktree

# Create the orphan gh-pages branch if it doesn't exist yet. This uses plumbing
# so it never disturbs the branch currently checked out in the main worktree.
if ! git show-ref --verify --quiet refs/heads/gh-pages; then
    echo "Creating gh-pages branch..."
    empty_tree=$(git hash-object -t tree /dev/null)
    init_commit=$(git commit-tree "${empty_tree}" -m "Initialize gh-pages")
    git branch gh-pages "${init_commit}"
fi

# Attach the gh-pages worktree.
echo "Setting up gh-pages worktree..."
mkdir -p output
git worktree add "${WORKTREE_DIR}" gh-pages
echo

# Clear previously-deployed files (keep .git) so pages removed from the site
# don't linger in the gh-pages branch.
find "${WORKTREE_DIR}" -mindepth 1 -not -path "${WORKTREE_DIR}/.git*" -delete

# Build the binary and static site.
echo "Building sfwr..."
if ! go build -o sfwr; then
    echo "Build failed; aborting deploy."
    exit 1
fi

echo "Building site..."
./sfwr -build

# Commit and push the built site to gh-pages.
echo "Deploying to GitHub Pages..."
(
    cd "${WORKTREE_DIR}"
    git add -A
    if git commit -m "Deploy: $(date '+%Y-%m-%d %H:%M:%S')"; then
        git push origin gh-pages
        echo
        echo "✓ Deployment complete! GitHub Pages may take 1-2 minutes to update."
    else
        echo "No changes to deploy."
    fi
)
