#!/bin/bash
#
# Sync the collection between this machine and the server.
#
#   ./sync.sh status   compare both sides, change nothing
#   ./sync.sh push     send this machine's database and covers to the server
#   ./sync.sh pull     bring the server's database and covers here
#
# The database is a single SQLite file and cannot be merged, so a sync is
# always a whole-file copy in one direction. That makes it possible to lose
# work, so this script:
#
#   * remembers the fingerprint of both sides after each successful sync,
#     and refuses to run when BOTH have changed since (pass --force to
#     override once you have decided which side wins)
#   * backs up the destination database before overwriting it
#   * restarts the server and rebuilds the site after a push, because the
#     running admin server holds the old database file open
#
# Configure the other end in sfwr.conf:
#
#   remote_host = user@example.com
#   remote_dir  = /home/user/sfwr
#
set -euo pipefail

cd "$(dirname "$0")"
CONFIG="${SFWR_CONFIG:-sfwr.conf}"
STATE=".sfwr-sync-state"
FORCE=no
COMMAND="${1:-status}"

for arg in "$@"; do
    [ "$arg" = "--force" ] && FORCE=yes
done

die() { echo "error: $*" >&2; exit 1; }

setting() {  # setting <key> -- read a value out of sfwr.conf
    local key="$1"
    [ -f "$CONFIG" ] || die "no config file at $CONFIG"
    grep -E "^[[:space:]]*${key}[[:space:]]*=" "$CONFIG" 2>/dev/null |
        head -1 | cut -d= -f2- | sed 's/^[[:space:]]*//; s/[[:space:]]*$//; s/^["'"'"']//; s/["'"'"']$//'
}

REMOTE_HOST=$(setting remote_host)
REMOTE_DIR=$(setting remote_dir)
DATABASE=$(setting database);       DATABASE=${DATABASE:-sfwr_database.db}
COVERS=$(setting cover_images);     COVERS=${COVERS:-saved_cover_images}

[ -n "$REMOTE_HOST" ] || die "set remote_host in $CONFIG"
[ -n "$REMOTE_DIR" ]  || die "set remote_dir in $CONFIG"

# --- fingerprints -----------------------------------------------------------

local_fingerprint() {
    ./sfwr -config "$CONFIG" -fingerprint | awk '$1=="data"{print $2}'
}

remote_fingerprint() {
    ssh "$REMOTE_HOST" "cd '$REMOTE_DIR' && ./sfwr -config sfwr.conf -fingerprint" |
        awk '$1=="data"{print $2}'
}

summarise() {  # summarise <local|remote>
    if [ "$1" = local ]; then
        ./sfwr -config "$CONFIG" -fingerprint
    else
        ssh "$REMOTE_HOST" "cd '$REMOTE_DIR' && ./sfwr -config sfwr.conf -fingerprint"
    fi
}

read_state() {  # sets WAS_LOCAL / WAS_REMOTE, empty when never synced
    WAS_LOCAL=""; WAS_REMOTE=""
    [ -f "$STATE" ] || return 0
    WAS_LOCAL=$(awk -F= '$1=="local"{print $2}' "$STATE")
    WAS_REMOTE=$(awk -F= '$1=="remote"{print $2}' "$STATE")
}

write_state() {
    printf 'local=%s\nremote=%s\nsynced=%s\n' "$1" "$2" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$STATE"
}

# --- guard ------------------------------------------------------------------

# Refuse to overwrite a side that has its own unsynced changes.
check_direction() {  # check_direction <push|pull> <local_fp> <remote_fp>
    local direction="$1" here="$2" there="$3"
    read_state

    local local_changed=yes remote_changed=yes
    [ -n "$WAS_LOCAL" ]  && [ "$WAS_LOCAL"  = "$here"  ] && local_changed=no
    [ -n "$WAS_REMOTE" ] && [ "$WAS_REMOTE" = "$there" ] && remote_changed=no

    if [ -z "$WAS_LOCAL" ]; then
        echo "note: no record of a previous sync, so changes on the other side cannot be detected."
        echo "      The destination database is backed up before it is replaced."
        return 0
    fi

    if [ "$local_changed" = yes ] && [ "$remote_changed" = yes ]; then
        echo "Both sides have changed since the last sync:" >&2
        echo "    local  $WAS_LOCAL -> $here" >&2
        echo "    remote $WAS_REMOTE -> $there" >&2
        echo >&2
        echo "A database cannot be merged, so one side's edits will be lost." >&2
        echo "Decide which side is right, then re-run with --force." >&2
        [ "$FORCE" = yes ] || exit 1
        echo "--force given: continuing." >&2
    elif [ "$direction" = push ] && [ "$remote_changed" = yes ]; then
        echo "The server has changed since the last sync and pushing would discard that." >&2
        [ "$FORCE" = yes ] || { echo "Re-run with --force if you meant to." >&2; exit 1; }
    elif [ "$direction" = pull ] && [ "$local_changed" = yes ]; then
        echo "This machine has changed since the last sync and pulling would discard that." >&2
        [ "$FORCE" = yes ] || { echo "Re-run with --force if you meant to." >&2; exit 1; }
    fi
}

# --- commands ---------------------------------------------------------------

do_status() {
    echo "local  ($PWD)"
    summarise local | sed 's/^/    /'
    echo "remote ($REMOTE_HOST:$REMOTE_DIR)"
    summarise remote | sed 's/^/    /'
    echo

    local here there
    here=$(local_fingerprint); there=$(remote_fingerprint)
    if [ "$here" = "$there" ]; then
        echo "In sync."
        return 0
    fi

    echo "Out of sync."
    read_state
    if [ -n "$WAS_LOCAL" ]; then
        [ "$WAS_LOCAL"  = "$here"  ] && echo "  local:  unchanged since the last sync"  || echo "  local:  changed since the last sync"
        [ "$WAS_REMOTE" = "$there" ] && echo "  remote: unchanged since the last sync" || echo "  remote: changed since the last sync"
    else
        echo "  (no previous sync recorded)"
    fi
    echo
    echo "Run './sync.sh push' to publish this machine's copy, or './sync.sh pull' to take the server's."
}

do_push() {
    local here there
    here=$(local_fingerprint); there=$(remote_fingerprint)
    if [ "$here" = "$there" ]; then echo "Already in sync; nothing to send."; write_state "$here" "$there"; return 0; fi
    check_direction push "$here" "$there"

    echo "Backing up the server's database..."
    ssh "$REMOTE_HOST" "cd '$REMOTE_DIR' && mkdir -p backups && cp -- '$(basename "$DATABASE")' \"backups/\$(basename '$DATABASE' .db)-\$(date -u +%Y%m%dT%H%M%SZ).db\""

    echo "Sending cover images..."
    rsync -a --info=stats1 "$COVERS/" "$REMOTE_HOST:$REMOTE_DIR/$(basename "$COVERS")/"

    echo "Sending database..."
    rsync -a "$DATABASE" "$REMOTE_HOST:$REMOTE_DIR/$(basename "$DATABASE").incoming"
    ssh "$REMOTE_HOST" "cd '$REMOTE_DIR' && mv -- '$(basename "$DATABASE").incoming' '$(basename "$DATABASE")'"

    echo "Restarting the admin server so it reopens the new database..."
    ssh "$REMOTE_HOST" "cd '$REMOTE_DIR' && ./restart.sh" || echo "  (no restart.sh, or it failed - restart the server yourself)"

    echo "Rebuilding the published site..."
    ssh "$REMOTE_HOST" "cd '$REMOTE_DIR' && ./sfwr -config sfwr.conf -build"

    there=$(remote_fingerprint)
    [ "$here" = "$there" ] || die "after pushing, the server's fingerprint still differs ($here vs $there)"
    write_state "$here" "$there"
    echo "Done. Both sides now match."
}

do_pull() {
    local here there
    here=$(local_fingerprint); there=$(remote_fingerprint)
    if [ "$here" = "$there" ]; then echo "Already in sync; nothing to fetch."; write_state "$here" "$there"; return 0; fi
    check_direction pull "$here" "$there"

    echo "Backing up the local database..."
    mkdir -p backups
    cp -- "$DATABASE" "backups/$(basename "$DATABASE" .db)-$(date -u +%Y%m%dT%H%M%SZ).db"

    echo "Fetching cover images..."
    rsync -a --info=stats1 "$REMOTE_HOST:$REMOTE_DIR/$(basename "$COVERS")/" "$COVERS/"

    echo "Fetching database..."
    rsync -a "$REMOTE_HOST:$REMOTE_DIR/$(basename "$DATABASE")" "$DATABASE.incoming"
    mv -- "$DATABASE.incoming" "$DATABASE"

    here=$(local_fingerprint)
    [ "$here" = "$there" ] || die "after pulling, this machine's fingerprint still differs ($here vs $there)"
    write_state "$here" "$there"
    echo "Done. Both sides now match. Run './sfwr -build' to regenerate the local preview."
}

case "$COMMAND" in
    status) do_status ;;
    push)   do_push ;;
    pull)   do_pull ;;
    *)      echo "usage: $0 {status|push|pull} [--force]" >&2; exit 2 ;;
esac
