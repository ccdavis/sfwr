# sfwr
A book recommendation web app written in Go.  This is my first Go project.

## Deployment Options

### Option 1: GitHub Pages (Automated)

Push changes to the `sfwr_database.db` file or `saved_cover_images/` directory to trigger GitHub Actions, which builds and deploys to GitHub Pages automatically.

You can also use the web UI's "Deploy" button (`./sfwr -web=8080`) to commit and push in one step.

### Option 2: Tailscale Funnel (Self-hosted)

Serve the static site directly from any machine on your Tailscale network using [Tailscale Funnel](https://tailscale.com/kb/1223/funnel). No separate webserver required.

#### Setup

1. **Build the static site:**
   ```bash
   go build -o sfwr
   ./sfwr -build
   ```

2. **Enable Funnel in Tailscale admin console:**
   Go to [admin.tailscale.com](https://login.tailscale.com/admin) → Access controls → Funnel → "Add Funnel to policy"

3. **Start serving (persistent, survives reboots):**
   ```bash
   sudo tailscale funnel -bg /path/to/sfwr/output/public
   ```

4. **Check status:**
   ```bash
   tailscale funnel status
   ```

5. **Stop serving:**
   ```bash
   sudo tailscale funnel /path/to/sfwr/output/public off
   ```

#### Finding Your Public URL

Your site URL follows the pattern: `https://<machine-name>.<tailnet-name>.ts.net`

To find your tailnet name:
```bash
tailscale status --json | grep MagicDNSSuffix
```

Example output:
```
"MagicDNSSuffix": "tail06ce8e.ts.net",
```

If your machine is named `regulus`, the URL would be: `https://regulus.tail06ce8e.ts.net`

You can also find the tailnet name in the [Tailscale admin console](https://login.tailscale.com/admin) under DNS settings.
