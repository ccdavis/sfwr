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

- `./sfwr -web=8080` binds to `127.0.0.1` and needs no password — the usual local workflow.
- GitHub Pages sites are public by default. The admin UI is not part of the published site.

### Publishing the admin UI

The admin server can run as a normal website. It refuses to start on a non-loopback
address unless both a password and TLS are in place.

**1. Set a password**

```bash
./sfwr -set-password
```

It prompts twice and writes the hash to `sfwr.env` next to your config file,
readable only by you. Nothing to copy, and nothing to redirect.

A mistyped or too-short password just re-prompts; the existing password file is
left untouched until a good one is confirmed. The file stores a bcrypt hash, never
the password itself.

Changing the password later is the same command. The running server keeps the old
one until it restarts:

```bash
./sfwr -set-password && ./restart.sh
```

If you keep secrets elsewhere (a systemd `Environment=`, a password manager),
`./sfwr -hash-password` prints the export line without writing any file.

**2. Put a TLS-terminating proxy in front**

Caddy obtains and renews certificates automatically:

```
admin.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

nginx equivalent:

```nginx
server {
    listen 443 ssl;
    server_name admin.example.com;

    ssl_certificate     /etc/letsencrypt/live/admin.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/admin.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**3. Start the server**

```bash
./sfwr -web=8080 -bind=127.0.0.1 -behind-proxy
```

Keep the bind on `127.0.0.1` so the only way in is through the proxy. `-bind=0.0.0.0`
also works but leaves the plaintext port reachable directly.

`-behind-proxy` does two things: marks the session cookie `Secure`, and reads the
client address from the last `X-Forwarded-For` entry for login rate limiting. Only
turn it on when a proxy really is in front — otherwise clients can influence the
address the throttle keys on.

### What stays local

Everything works remotely except **Rollback**, which replaces the live database with
an older copy and cannot be undone from the browser. Its buttons are hidden when you
browse remotely, and the route returns 403. To roll back, either open the admin UI on
the server itself at `http://127.0.0.1:8080` (bypassing the proxy), or restore the
file directly:

```bash
cd ~/sfwr
git log --oneline -- sfwr_database.db
git checkout <commit> -- sfwr_database.db
# then restart the admin server so it reopens the file
```

---

## Hosting on a DreamHost VPS

This walks through `sfworthreading.com` as the public site with the admin UI at
`admin.sfworthreading.com`, both on one VPS. Substitute your own names; nothing
below is specific to this collection.

> **Verified against the live VPS** (Ubuntu 22.04, Apache 2.4.52, glibc 2.35).
> No root, no sudo, and no DreamHost panel changes are required: the reverse proxy
> is done with a `.htaccess` file, which was confirmed working. What follows was
> tested on the box rather than inferred.

### 1. Directory layout

DreamHost gives each domain a document root under the user's home directory named
after the domain. Keep the application and its data outside the document root so the
database is never web-reachable:

```
/home/youruser/
├── sfwr/                       # the application: binary, database, templates, git
│   ├── sfwr
│   ├── sfwr.conf
│   ├── sfwr_database.db
│   ├── saved_cover_images/
│   └── templates/
└── sfworthreading.com/         # document root — generated site only
```

The `sfwr/` directory must not be inside `sfworthreading.com/`, or the database would
be downloadable over the web.

### 2. Build locally and copy the binary over

The server needs no Go toolchain. Build a static binary on your own machine:

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o sfwr .
scp sfwr youruser@your-vps:~/sfwr/sfwr
```

`CGO_ENABLED=0` matters. It selects Go's own SQLite implementation and DNS resolver, so
the binary carries no glibc dependency and runs on any Linux x86-64 host. A cgo build
links the *build* machine's glibc; built on Ubuntu 24.04 it will misbehave on this
22.04 server, and the first thing to fail is the DNS lookup for the cover-art APIs.

The templates are read at runtime, so copy those too (or clone the repo on the server
for them and the git history):

```bash
scp -r templates youruser@your-vps:~/sfwr/
```

### 3. Write the config

```bash
cp sfwr.conf.example sfwr.conf
```

Then edit it:

```
database     = sfwr_database.db
cover_images = saved_cover_images
templates    = templates
repo         = .

# The DreamHost document root for the public domain
output       = /home/youruser/sfworthreading.com

site_name    = Science Fiction Worth Reading
bind         = 127.0.0.1
port         = 8080
base_path    = /admin
behind_proxy = true
```

`base_path = /admin` is what makes the admin UI live at `sfworthreading.com/admin`
rather than needing its own subdomain. Every link, form action, redirect and `fetch`
the server emits gets that prefix, and the session cookie is scoped to it.

Relative paths resolve against the config file, so the server can be started from
anywhere. Check it by building the site once:

```bash
./sfwr -build
ls ~/sfworthreading.com
```

`sfworthreading.com` should now serve the generated site.

### 4. Set the admin password

```bash
./sfwr -hash-password
```

Put the printed `export` line somewhere the service reads it, such as
`~/sfwr/sfwr.env` with `chmod 600`. Never commit it.

### 5. Proxy `/admin` to the Go server with .htaccess

DreamHost's panel-managed Apache owns ports 80 and 443 and its config is root-only,
but `.htaccess` is honoured and `mod_rewrite`'s proxy flag works. Put the rules in
their own directory so a mistake cannot affect the rest of the site:

```bash
mkdir -p ~/sfworthreading.com/admin
cat > ~/sfworthreading.com/admin/.htaccess <<'EOF'
RewriteEngine On
RewriteBase /admin/
RewriteRule ^(.*)$ http://127.0.0.1:8080/admin/$1 [P,L]
EOF
```

Apache sets `X-Forwarded-For` itself, appending the real client address **after**
anything the client sent. The admin server reads the last entry, so login throttling
cannot be evaded by forging the header, and the local-only rollback check cannot be
tricked into granting access.

This reuses the existing certificate for `sfworthreading.com` — no new domain, DNS
record, or certificate is needed.

ModSecurity (OWASP CRS) runs on this server. It was tested against realistic admin
traffic — quotes, dashes, angle brackets, a literal `<script>` tag, an 8 KB review
body, and JSON posts — and passed all of them. If a future edit is ever rejected with
a 403 that the admin server has no record of, ModSecurity is the place to look.

### 6. Keep the server running

There is no sudo, so no systemd unit. Start it detached with `setsid`, which was
confirmed to survive an SSH disconnect:

```bash
cd ~/sfwr
. ./sfwr.env
setsid nohup ./sfwr -config ~/sfwr/sfwr.conf < /dev/null >> ~/sfwr/admin.log 2>&1 &
```

No `-web` flag is needed: `port` in the config file starts the server.

Restart it after a reboot with a crontab entry (`crontab -e`):

```
@reboot cd /home/youruser/sfwr && . ./sfwr.env && setsid ./sfwr -config sfwr.conf >> admin.log 2>&1
```

To restart after copying up a new binary, find and kill it by port:

```bash
kill $(ss -tlnp | grep 8080 | grep -o 'pid=[0-9]*' | cut -d= -f2)
```

### 7. Clear out the old site

The build writes its own pages but never deletes, so anything the previous site left
behind stays reachable. Clear the document root before the first publish, keeping
`admin/` and the `.well-known/` directory the certificate renewal uses:

```bash
cd ~/sfworthreading.com
ls -A | grep -vE '^(admin|\.well-known|\.dh-diag)$' | xargs rm -rf
```

### 8. Check it

Visit `https://sfworthreading.com/admin`. You should get a password prompt, then be
able to edit books and press **Rebuild Site**, which writes straight into
`~/sfworthreading.com`. Reload `https://sfworthreading.com` to see the change.

If sign-in loops back to the login page, you are on plain HTTP — the session cookie
is `Secure` and the browser is discarding it.

### Daily use

1. Edit books at `https://admin.sfworthreading.com`
2. Press **Rebuild Site** — the public site updates immediately
3. Optionally press **Commit & Push to Git** to checkpoint the database off-site

`repo` and the Git buttons are optional. Drop `repo` from the config and the site
publishes purely by rebuilding into the document root; the Git controls disappear
from the UI.

---

### Session behavior

- One password, no user accounts. Sessions are held in memory, so restarting the
  server signs everyone out.
- Sessions last 12 hours and renew while you are active.
- Five failed logins from one address trigger a 15-minute lockout.
- If you forget the password, generate a new hash and restart the server.
