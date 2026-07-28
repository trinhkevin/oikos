# Brivin Household — Design

**Date:** 2026-07-27
**Status:** Approved for planning

A Raspberry Pi–hosted site for guests in the house. Guests join the WiFi, scan a QR code,
and land on a mobile-first hub: cat biographies, drink and snack menus, live Spotify
now-playing with direct queue requests, a photo upload gallery, and a guest book.

## Goals

- Reachable from any guest phone on the home network with no app install and no typing
- Menus and cat biographies editable by hand over SSH, live on the next page refresh
- Guests see what's currently playing and add songs straight to the queue, unmoderated
- Guests upload photos that persist on the Pi and survive its SD card dying
- Runs unattended; survives reboots and power cuts without intervention

## Non-goals

Explicitly out of scope. Adding any of these is a new spec.

- Internet exposure. No port forwarding, no tunnel, no public DNS.
- HTTPS/TLS. Plain HTTP on the LAN.
- Admin UI. Content is edited as files; moderation is done over SSH.
- User accounts or logins. Guests optionally type a name; nothing is verified.
- Docker. Single binary plus systemd.
- Browser/end-to-end tests.
- Spotify Jam integration (see Music) — the Web API has no Jam endpoints.

## Hardware and OS

- **Raspberry Pi 4**, any RAM tier
- **Raspberry Pi OS Lite (64-bit), Trixie** — Debian 13, kernel 6.12 LTS
- **128GB endurance-grade microSD** — Samsung PRO Endurance or SanDisk Max Endurance

Lite rather than Desktop: the Pi is headless and accessed over SSH, so a desktop
environment only consumes RAM and widens attack surface. 64-bit because the Pi 4 is ARMv8
and the build target is `GOARCH=arm64`.

**The card grade is a real decision, not a detail.** All data — OS, database, and uploaded
photos — shares one card. Endurance-grade cards are rated for the sustained writes that
kill standard cards. The design treats the card as a consumable and makes its failure
boring rather than trying to prevent it (see Backup and recovery).

Flash with Raspberry Pi Imager, using pre-configuration (gear icon) to set the hostname,
enable SSH with a public key, and supply WiFi credentials — so the Pi is reachable over SSH
on first boot with no keyboard or monitor attached.

### Reducing writes to the card

Applied at provisioning time:

- `noatime` on the root mount
- `Storage=volatile` in `journald.conf`, so logs live in RAM
- Swap disabled (`dphys-swapfile swapoff`)
- SQLite in **WAL mode**, which has lower write amplification than the default rollback
  journal

## Network and hostname

The Pi holds a **fixed DHCP reservation at `192.168.1.50`** (UniFi: *Client Devices → Pi →
Settings → Fixed IP*). Every other network decision depends on this address not moving.

Two addresses resolve to it, and the server answers on both because it serves port 80
regardless of the `Host` header:

| Address | Role |
|---|---|
| `http://home.arpa` | Primary. What the `/share` QR encodes. |
| `http://192.168.1.50` | Fallback, shown as small text on `/share`. |

**Why `home.arpa`:** RFC 8375 reserves it for residential networks. It can never be
publicly registered, which makes the HSTS failure mode structurally impossible, and it sits
in the public suffix list so browser omniboxes treat it as a navigable host rather than a
search term.

**Why the IP fallback is load-bearing:** a guest device with a DNS-over-HTTPS profile or an
MDM-managed resolver bypasses the UniFi resolver entirely, so `home.arpa` will not resolve
for them.

### UniFi configuration

1. *Client Devices → Pi → Settings → Fixed IP* → `192.168.1.50`
2. *Settings → Routing → DNS → Local DNS Records* → **A record** `home.arpa` →
   `192.168.1.50` (exact menu path varies across UniFi OS versions)
3. *Settings → Networks → LAN → DHCP Name Server* → the gateway, so clients use UniFi's
   resolver by default

Guests join the **main WiFi network**, not a guest SSID — UniFi guest networks block
guest-to-LAN traffic by default, which would make the Pi unreachable.

### No reverse proxy

There is no Caddy or nginx. A reverse proxy would exist only to terminate TLS, and there is
no TLS. The Go binary binds `:80` directly, running as a non-root user with
`AmbientCapabilities=CAP_NET_BIND_SERVICE`.

## Stack

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go | Single static binary, cross-compiled from macOS. No runtime on the Pi to install or break. ~20MB resident. |
| Templates | [Templ](https://templ.guide) | Compile-time-checked templates. A bad field name is a build error on the Mac, not a 500 during a party. |
| Interactivity | HTMX | Form posts and upload progress return HTML fragments. No bundler, no `node_modules`, no JS build step. |
| Storage | SQLite via `modernc.org/sqlite` | Pure Go, so cross-compiling needs no CGO toolchain. |
| Images | ImageMagick via `exec` | HEIC decoding, EXIF stripping, thumbnails. Keeps the Go binary CGO-free. |
| Markdown | `goldmark` | Cat biographies. |
| QR | `github.com/yeqown/go-qrcode/v2` | WiFi and site QRs, rendered as PNG. |
| Config | `gopkg.in/yaml.v3` | Content files and `config.yaml`. |
| Fonts | Fraunces (SIL OFL) at weight 900 for headlines; Lora (SIL OFL) for body — both self-hosted `woff2`, both Google Fonts | See Visual direction. |

Pi packages: `apt install imagemagick libheif-examples restic`.

## Visual direction

**Mobile-first, and literally so** — layouts are designed at 390px and allowed to breathe
on larger screens, rather than desktop layouts squeezed down. Every interactive target is
at least 44px. The site is used one-handed, standing up, slightly drunk, in bad lighting.

**Fraunces** for headlines, **Lora** for body — both self-hosted as `woff2`, subset to Latin,
both genuinely free (SIL Open Font License, Google Fonts — no license ambiguity to check,
unlike a font sourced from a third-party foundry site). Fraunces is a high-contrast variable
serif with real display weight (used here at 900/Black) — closer to a Canela-like editorial
fashion register than Lora alone, which is why it carries headlines while Lora stays on body
text. (Instrument Serif was considered as a Canela-adjacent alternative but rejected: it
ships only a single weight — Regular and Italic, no Bold — which can't carry "genuinely
display, no timid headings" on its own.) Fraunces is built for large display sizes only —
its high contrast and thin hairlines hurt readability at body-copy sizes, so it is never
used below a heading threshold (roughly 24px and up). Lora carries every other text on the
page: menu items, cat bios, guest book entries, form labels. Lora is warmer and
lower-contrast than a Didone — it reads editorial-cozy rather than fashion-austere, which
suits a household and pairs well under a bolder display face. To keep the pairing from
reading as a default blog theme, the personality comes from typographic decisions as much
as the fonts themselves:

- Large scale jumps between levels — display sizes genuinely display, no timid `1.2rem`
  headings
- Tight negative tracking on large headings, normal tracking on body
- Restrained palette: warm off-white ground, near-black ink, one saturated accent
- Generous whitespace and a single-column measure capped around 60 characters
- Full-bleed imagery against tight text blocks for contrast
- Dark mode honored everywhere **except the two QR pages** (see QR pages)

Detailed visual execution is deferred to implementation, where the `frontend-design` skill
applies.

## Pages and navigation

Ten sections, in this order:

| # | Route | Name | Purpose |
|---|---|---|---|
| 1 | `/` | Welcome | Short hello, plus the nav hub |
| 2 | `/wifi` | Wi-Fi | WiFi join QR and credentials |
| 3 | `/coffee` | Coffee Menu | Current coffee drinks |
| 4 | `/cocktails` | Cocktail Menu | Current cocktails |
| 5 | `/refreshments` | Refreshments | Current snacks and food |
| 6 | `/music` | Music Requests | Now playing, search, and unmoderated queue requests |
| 7 | `/photos` | Upload Photos | Guest photo upload and gallery |
| 8 | `/guestbook` | Guest Book | Signed entries with optional photo |
| 9 | `/cats` | Meet the Cats | Cat biographies |
| 10 | `/share` | Share | Site URL QR |

Header reads **Brivin Household** on every page.

**Navigation pattern:** ten items exceed what a mobile tab bar can hold (roughly five), so:

- **Welcome is the hub** — a grid of large tappable cards to the other nine sections, each
  with a title and one-line description
- **Every page keeps a slim sticky header** with the household name and a menu button
  opening a full-screen overlay listing all ten sections

This scales past ten items and avoids pill nav's failure mode, where guests never discover
what scrolled off-screen.

## The two QR pages

`/wifi` and `/share` each show exactly one QR code. They are separate pages because they
serve different moments, and a page with two codes invites scanning the wrong one. `/music`
is **not** a QR page — now-playing and search work entirely within the site itself, with no
external link to encode.

Shared display constraints, all following from "scanned off a phone screen held at arm's
length":

- **White ground, black modules, regardless of the viewer's system theme.** An inverted QR
  fails on many camera apps. These two pages opt out of dark mode entirely.
- **Error correction level Q**, large render, generous quiet zone — reads at an angle, off a
  glossy screen, in dim light.
- **Minimal surrounding chrome** so nothing else competes for the camera.

### `/wifi` — join the network

Encodes **only the WiFi credentials**, nothing about the site. Displayed on the host's phone
and held out for guests to scan. The host is already on the network, so the page loads —
which is what makes this work. A guest-facing WiFi page is unreachable by definition, since
a guest who needs the password is not yet on the WiFi.

```
WIFI:T:WPA;S:<ssid>;P:<password>;;
```

Natively handled by the iOS Camera app and Android 10+. `;` `,` `:` and `\` in the SSID or
password **must be backslash-escaped**. A `hidden: true` config flag appends `H:true;` for
non-broadcast SSIDs.

Below the QR: SSID and password in selectable text, for the one guest whose camera app will
not cooperate. Nothing is leaked that the QR did not already contain.

### `/share` — open the site

Encodes `http://home.arpa`. Useful **guest-to-guest**: anyone already browsing can pull up
`/share` and pass the site to the next person, so the host is not the only distribution
channel. Below the QR: `home.arpa` large, `192.168.1.50` small as the resolver-bypass
fallback.

## Features

### Welcome (`/`)

A short hello in prose, then the card grid to the other nine sections. Copy lives in
`content/welcome.md` so it can be reworded per occasion without a deploy.

### Music Requests (`/music`)

**Live now-playing plus direct, unmoderated queue requests** — guests search for a track and
it goes straight into the host's Spotify queue, no approval step.

**Why not a Jam:** the Spotify Web API has no Jam endpoints — developers have requested them
since 2023 and they remain open forum suggestions with no status. Jam is not buildable here.

**Why this needs OAuth, unlike everything else on the site:** reading what's currently
playing and adding to the queue are both scoped to a real user's device, not a public
resource — there is no app-only equivalent, which is why every other integration in this
project deliberately avoids OAuth and this one cannot. The authorization is a **one-time
host setup step**, not something guests ever see:

1. Register a Spotify app; set the redirect URI to `http://127.0.0.1:8080/spotify/callback`
   (a loopback address — Spotify rejects non-HTTPS redirect URIs except on loopback). Port
   `8080` here is the SSH tunnel's local end, independent of whatever port the site itself
   listens on (`:80` on the Pi, `:8080` locally) — the tunnel is what remaps it.
2. From the Mac: `ssh -L 8080:localhost:80 pi@192.168.1.50` (forwards local `8080` to the
   Pi's real listening port).
3. Open `http://127.0.0.1:8080/spotify/login` and approve the requested scopes
   (`user-read-currently-playing`, `user-read-playback-state`, `user-modify-playback-state`).
4. The refresh token is stored in the `oauth_tokens` table; access tokens refresh
   automatically from then on. This step is repeated only if the refresh token is ever
   revoked from the Spotify account's connected-apps settings.

`/spotify/login` and `/spotify/callback` **reject any request whose `RemoteAddr` is not
loopback**, so the authorization flow itself is not guest-reachable even though it shares the
same port as the rest of the site.

The page shows:

- **Now playing** — track name and artist, polled from
  `GET /v1/me/player/currently-playing` on a short server-side cache (10s) so a room full of
  guests loading the page doesn't hammer the API. Shows a quiet "Nothing's playing" state
  when no active playback exists, rather than an error.
- **A search box** — `GET /music/search?q=` returns an HTMX fragment of matching tracks via
  `GET /v1/search?type=track`.
- **Tap a result to queue it** — `POST /music/request` calls
  `POST /v1/me/player/queue?uri=…` directly. No approval step, no visible moderation queue —
  a tap is a queue add.

Requests are rate-limited (see Rate limiting) and logged to `song_requests` — not for
moderation, but so a failed queue-add has a debugging trail and so "who added that" is
answerable if it ever needs to be.

### Upload Photos (`/photos`)

Guests upload photos from their phones into a shared gallery. This is the feature with the
most ways to go wrong, so its rules are explicit.

**Ingest pipeline**, per uploaded file:

1. Reject anything whose sniffed content type is not `image/jpeg`, `image/png`,
   `image/webp`, or `image/heic`. Trust sniffed bytes, never the filename or the
   client-supplied MIME type.
2. Reject files over **25MB**.
3. Reject if the upload directory is at or above its **80GB cap**.
4. Convert to JPEG and **strip all metadata** via ImageMagick
   (`-strip`), which removes GPS coordinates, device identifiers, and timestamps. Guest
   photos routinely embed the household's location; stripping is the default and there is no
   opt-out.
5. Generate a **thumbnail** (long edge 400px). A gallery of full-size photos would be
   unusable over WiFi on a phone.
6. Write both to disk under `uploads/YYYY-MM/<ulid>.jpg` and
   `uploads/YYYY-MM/<ulid>_thumb.jpg`, and insert a row.

**HEIC** is why ImageMagick exists in this design. iPhones upload HEIC, Go has no HEIC
decoder, and an in-process decoder would require CGO and `libheif` — which would break the
clean cross-compile from macOS. Shelling out keeps the Go binary CGO-free at the cost of one
`apt install`.

**The 80GB cap is mandatory, not a nicety.** Photos share a card with the OS, so guests
filling the disk breaks the operating system, not just uploads. Current usage is exposed on
`/healthz`, and past 85% the upload form shows a warning to the host.

Gallery: thumbnails in a responsive grid, newest first, paginated at 60 per page, tapping
through to the full image. Upload is `multipart/form-data` with `multiple`, showing per-file
progress via HTMX, and reports partial success honestly — three of five succeeded says so,
and names the two that did not.

### Guest Book (`/guestbook`)

A name, a short message, and an optional photo — displayed as signed entries, newest first.

Guest Book and Upload Photos **share the same storage backend and ingest pipeline** but stay
distinct experiences: `/photos` is a bulk drop into a gallery, `/guestbook` is one signed
entry. A guest book photo is stored identically and linked from its entry.

Limits: name 40 characters, message 500, one optional photo. Templ escapes output by
default, so stored text cannot inject markup.

Moderation is over SSH: `UPDATE guestbook_entries SET hidden=1 WHERE id=…`. The threat model
is friends being funny on a LAN-only site, which does not justify building and securing an
admin UI.

### Menus — Coffee, Cocktails, Refreshments

All three share **one** `Menu` type and one parser; they differ only in data.

```go
type Menu struct {
    Title    string
    Note     string
    Sections []Section
}
type Section struct {
    Name  string
    Items []Item
}
type Item struct {
    Name        string
    Description string
    Ingredients []string   // cocktails; empty elsewhere
    Tags        []string
    Price       string     // optional
}
```

```yaml
# content/cocktails.yaml
title: Cocktail Menu
note: Ask about the rotating seasonal.
sections:
  - name: Classics
    items:
      - name: Negroni
        description: Equal parts, stirred, orange peel.
        ingredients: [Gin, Campari, Sweet vermouth]
        tags: [stirred, bitter]
```

### Meet the Cats (`/cats`)

Index of cats, each linking to a profile. Markdown with YAML front matter, so biographies
can be prose:

```markdown
---
name: Mochi
slug: mochi
photo: mochi.jpg
adopted: 2023-04-01
likes: [cardboard boxes, 5am sprints]
dislikes: [the vacuum, closed doors]
---
Mochi runs this house and permits us to live in it.
```

## Content reload

Content files are parsed on demand and cached in memory keyed by path, invalidated when the
file's `mtime` changes. Editing `cocktails.yaml` over SSH takes effect on the next page
refresh — no restart, no file watcher, one `stat()` per request.

**A file that fails to parse does not take the site down.** The loader keeps serving the last
good version and logs the error. This matters because content is hand-edited on a live
server.

## Data model

SQLite in WAL mode. Schema applied by an idempotent migration at startup.

```sql
CREATE TABLE guestbook_entries (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  message    TEXT    NOT NULL,
  photo_id   INTEGER REFERENCES photos(id),   -- nullable
  created_at TEXT    NOT NULL,                -- RFC3339 UTC
  hidden     INTEGER NOT NULL DEFAULT 0,
  client_ip  TEXT    NOT NULL
);
CREATE INDEX idx_guestbook_created ON guestbook_entries(created_at DESC);

CREATE TABLE photos (
  id          INTEGER PRIMARY KEY,
  path        TEXT    NOT NULL,       -- relative to uploads dir
  thumb_path  TEXT    NOT NULL,
  byte_size   INTEGER NOT NULL,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  source      TEXT    NOT NULL,       -- 'gallery' | 'guestbook'
  caption     TEXT,
  created_at  TEXT    NOT NULL,
  hidden      INTEGER NOT NULL DEFAULT 0,
  client_ip   TEXT    NOT NULL
);
CREATE INDEX idx_photos_created ON photos(created_at DESC);

CREATE TABLE oauth_tokens (
  provider      TEXT PRIMARY KEY,       -- 'spotify'
  refresh_token TEXT NOT NULL,
  access_token  TEXT,
  expires_at    TEXT,
  scopes        TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE song_requests (
  id           INTEGER PRIMARY KEY,
  track_uri    TEXT NOT NULL,
  track_name   TEXT NOT NULL,
  artist_name  TEXT NOT NULL,
  requested_by TEXT,                    -- optional nickname
  created_at   TEXT NOT NULL,
  client_ip    TEXT NOT NULL,
  status       TEXT NOT NULL            -- 'queued' | 'failed'
);
```

`oauth_tokens` holds exactly one row (`provider = 'spotify'`) — the refresh token from the
one-time host authorization. `song_requests` is not a moderation queue (nothing here is
approved before reaching Spotify) — it exists so a failed queue-add has a debugging trail
and so "who requested that" is answerable.

## Repository layout

```
homesite/
  cmd/homesite/main.go        wiring, config load, server start
  internal/content/           files → typed Menu and Cat structs
  internal/guestbook/         entry create/list, validation
  internal/photos/            ingest pipeline, gallery queries, disk cap
  internal/imaging/           ImageMagick exec wrapper: convert, strip, thumbnail
  internal/spotify/           OAuth flow, token refresh, now-playing, search, queue-add
  internal/qr/                PNG render; WiFi and URL payload builders
  internal/web/               routes, handlers, templ components
  views/                      .templ files
  static/                     CSS, htmx.min.js, Fraunces + Lora woff2 (go:embed)
  deploy/homesite.service
  deploy/backup.sh
  docs/RUNBOOK.md             SD card failure recovery
  Makefile
```

`internal/web` consumes the others through narrow interfaces, so handler tests need neither
a real Spotify, a real ImageMagick, nor a real filesystem.

### On-Pi layout

```
/srv/homesite/
  homesite            binary, scp'd from the Mac
  config.yaml         secrets; mode 0600, owned by homesite
  content/            hand-edited; owned kevin:homesite, mode 0775
    welcome.md
    coffee.yaml
    cocktails.yaml
    refreshments.yaml
    cats/mochi.md
    cats/mochi.jpg
  data/homesite.db    SQLite
  uploads/YYYY-MM/    guest photos and thumbnails
```

## Routes

| Route | Purpose |
|---|---|
| `GET /` | Welcome and nav hub |
| `GET /wifi` | WiFi join QR |
| `GET /wifi/qr.png` | The WiFi QR's PNG image |
| `GET /coffee`, `/cocktails`, `/refreshments` | Menus |
| `GET /music` | Now playing, search box, recent requests |
| `GET /music/search?q=` | HTMX fragment of matching tracks |
| `POST /music/request` | Queue a track → HTMX fragment |
| `GET /spotify/login`, `GET /spotify/callback` | One-time host OAuth; **loopback-only** |
| `GET /photos` | Gallery plus upload form |
| `POST /photos` | Multi-file upload → HTMX fragment |
| `GET /guestbook` | Entries plus form |
| `POST /guestbook` | Create entry → HTMX fragment |
| `GET /cats`, `GET /cats/{slug}` | Cat index and profiles |
| `GET /share` | Site URL QR |
| `GET /share/qr.png` | The Share QR's PNG image |
| `GET /healthz` | Liveness, disk usage, last backup time |
| `GET /static/*` | Embedded assets |
| `GET /media/*` | Cat photos from `content/cats/` |
| `GET /uploads/*` | Guest photos and thumbnails |

## Rate limiting and error handling

Limits are in-memory, keyed by session cookie **and** client IP, so clearing cookies does
not reset them. A restart resets all counters, which is acceptable.

- Guest book entries: 2 per 15 minutes
- Photo uploads: 30 files per 15 minutes
- Song requests: 3 per 15 minutes — the one limit that matters most here, since queueing is
  unmoderated and goes straight to Spotify with no approval step

Every failure renders a friendly fragment. Raw errors and status codes are logged, never
shown.

| Failure | Guest sees |
|---|---|
| No active Spotify device | "Nothing's playing yet — ask the host to start the music 🎵" |
| Refresh token revoked or absent | "Song requests are down right now" (logged for the host; re-run the one-time OAuth setup) |
| Spotify 429 on search/queue/now-playing | "Spotify's throttling us — try again in a minute" |
| Spotify 5xx or timeout | "Couldn't reach Spotify, try again" |
| Search returns nothing | "No matches — try the artist name" |
| Upload is not a real image | "That file isn't a photo — JPEG, PNG, WebP, or HEIC please" |
| Upload over 25MB | "That photo's too big — 25MB max" |
| Disk cap reached | "Photo storage is full — tell Kevin" (and a host warning past 85%) |
| ImageMagick fails or times out | "Couldn't process that photo, try another" (file discarded, error logged) |
| Partial multi-file upload | "3 of 5 uploaded" naming the two that failed |
| Guest book field empty or oversized | Inline validation; typed text is preserved |
| Rate limit hit | "You've had your turn — let someone else in" |
| Content file fails to parse | Last good version renders; error logged |

## Backup and recovery

The design assumes **the card will eventually fail** and optimizes for that being boring.

**Nightly `restic` to Backblaze B2** via systemd timer, covering `data/` and `uploads/`.
Encrypted, deduplicated, and versioned — so it also recovers from deleting the wrong thing,
not only from hardware death. At realistic photo volumes this costs cents per month.

**SQLite is snapshotted, not copied.** `deploy/backup.sh` runs
`VACUUM INTO '/tmp/homesite-snapshot.db'` first and backs up the snapshot. Copying a live
`.db` file can capture a torn write and yield a corrupt restore — the difference between a
backup and the illusion of one.

The timer reports success to `/healthz` as a last-successful-backup timestamp, so a silently
broken backup is visible rather than discovered during a restore.

`docs/RUNBOOK.md` documents recovery, written before it is needed: reflash Trixie, apply the
provisioning steps, `apt install`, `scp` the binary, `restic restore`. Config and content
live in git; data lives in B2. Target recovery time is under an hour.

## Testing

Table-driven Go tests. No browser automation.

| Package | Approach |
|---|---|
| `content` | Fixture YAML and markdown in `testdata/`; assert parsed structs. Includes a malformed file asserting last-good-version fallback. |
| `qr` | Assert exact payload strings — WiFi with `;` `,` `:` `\` in the password and the `hidden` flag, plus URL payloads. Assert payloads, not pixels. |
| `guestbook` | In-memory SQLite; create, length caps, ordering, `hidden` exclusion, photo linkage. |
| `photos` | Fake `imaging` implementation. Content-type sniffing including a `.jpg`-named text file, size limit, disk cap rejection, partial-batch reporting. |
| `imaging` | Integration-tagged, skipped when ImageMagick is absent. Asserts HEIC converts, EXIF GPS is gone from output, thumbnail dimensions are correct. |
| `spotify` | `httptest.Server` with canned JSON for now-playing/search/queue; token refresh on expiry; the failure-table rows above each produce the right sentinel. |
| `web` | `httptest` requests; assert fragments contain expected content, that all ten routes render, and that `/spotify/*` rejects any non-loopback caller. |

## Local development

The site is built and reviewed **entirely on macOS** before it ever reaches the Pi. Nothing
about the design should require the Pi to see a page — the Pi is a deployment target, not a
development environment.

```
brew install imagemagick libheif
make dev     # templ generate --watch + go run, live reload
open http://localhost:8080
```

Requirements this places on the design:

- **`listen` is configurable and defaults to `:8080` locally.** Binding `:80` needs `sudo`
  on macOS; only the Pi's systemd unit grants `CAP_NET_BIND_SERVICE`.
- **`config.local.yaml`** with relative `./content`, `./data`, `./uploads` paths, all
  gitignored except `content/`.
- **Seed content is committed** — real menu items, two cat profiles with photos, a few guest
  book entries. The site must look finished on first run, not like an empty shell, or design
  review is guesswork.
- **ImageMagick is a Homebrew dependency on the Mac**, matching the `apt` package on the Pi.
  The `imaging` package shells out identically on both.
- Spotify's OAuth setup runs the same way locally as on the Pi, except the loopback
  redirect needs no SSH tunnel at all — `http://127.0.0.1:8080/spotify/login` is already
  reachable directly on the Mac serving on `:8080`. One real authorization is needed per
  development machine to exercise now-playing/search/queue against the real API.

**Design review happens on a real phone, not a desktop browser.** The Mac serves on its LAN
address, so `http://<mac-ip>:8080` opens on the actual device the site is designed for.
DevTools device emulation is fine for fast iteration but misrepresents tap targets, real
viewport height with browser chrome, font rendering, and — critically for the two QR pages
— whether a code actually scans off a real screen. Those pages cannot be validated in an
emulator at all.

Only once the site looks right on a phone does anything get deployed.

## Deployment

`make deploy` runs: `templ generate` → `GOOS=linux GOARCH=arm64 go build` → `scp` the binary
→ `ssh systemctl restart homesite`.

```ini
# deploy/homesite.service
[Unit]
Description=Brivin Household site
After=network-online.target
Wants=network-online.target

[Service]
User=homesite
Group=homesite
WorkingDirectory=/srv/homesite
ExecStart=/srv/homesite/homesite -config /srv/homesite/config.yaml
Restart=always
RestartSec=2
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/srv/homesite/data /srv/homesite/uploads

[Install]
WantedBy=multi-user.target
```

`ProtectSystem=strict` makes the filesystem read-only except `ReadWritePaths`, so only
`data/` and `uploads/` are writable by the service. `content/` is group-writable by the
host's own user for SSH editing; the service never writes to it.

## Configuration

```yaml
# /srv/homesite/config.yaml — mode 0600, owner homesite
site:
  title: "Brivin Household"
  listen: ":80"
  url: "http://home.arpa"              # encoded into the /share QR
  fallback_url: "http://192.168.1.50"  # small text on /share
wifi:
  ssid: "..."
  password: "..."
  auth: WPA
  hidden: false
spotify:
  client_id: "..."
  client_secret: "..."
  redirect_uri: "http://127.0.0.1:8080/spotify/callback"
  now_playing_cache_seconds: 10
photos:
  max_file_bytes: 26214400             # 25MB
  max_total_bytes: 85899345920         # 80GB cap
  warn_at_percent: 85
  thumb_long_edge: 400
limits:
  guestbook_per_window: 2
  photo_uploads_per_window: 30
  song_requests_per_window: 3
  window_minutes: 15
content_dir: "/srv/homesite/content"
data_dir: "/srv/homesite/data"
uploads_dir: "/srv/homesite/uploads"
```

Secrets live only in this file. It is never committed; the repo carries
`config.example.yaml`.

## Security posture

The site is unauthenticated by design. Anyone on the WiFi can read every page, sign the
guest book, upload photos, and queue a song directly — which is the intent. The controls
that matter:

- No inbound path from the internet: no port forward, no tunnel, no public DNS record
- **EXIF stripped from every upload**, so guest photos cannot leak location or device data
  to other guests browsing the gallery
- Uploads identified by content sniffing, never by filename or client MIME type; stored
  under generated IDs, never guest-supplied names
- `/uploads/*` serves only from the uploads directory, with path traversal rejected
- **The Spotify refresh token is the one piece of guest-adjacent secret state this design
  stores** — it lives in `oauth_tokens`, on the same filesystem as everything else, with no
  extra encryption at rest (matching the rest of the data model; the threat model is a LAN
  party, not a hostile host). `/spotify/login` and `/spotify/callback` reject any request
  whose `RemoteAddr` is not loopback, so a guest on the WiFi cannot reach the authorization
  flow even though it shares the site's port. The token's blast radius if leaked is queue
  control and playback visibility on the host's Spotify account — not the account password.
- WiFi password and Spotify client credentials confined to a `0600` config file
- Service runs as a non-privileged user under systemd hardening, with exactly two writable
  directories
- Rate limits on every write endpoint, including song requests — the one limit that matters
  most, since queueing is unmoderated
- Output escaped by Templ throughout; guest input never rendered as raw HTML
