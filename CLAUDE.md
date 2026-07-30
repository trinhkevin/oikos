# Brivin Household — homesite

A Go + Templ + HTMX site for a Raspberry Pi 4, shared with party guests over
the household's WiFi (QR code on the Welcome/Wi-Fi page). Ten-ish pages:
Welcome, Wi-Fi join, Drinks (cocktails + beer/seltzer + coffee, folded
together), Food, Entertainment (jackbox.tv link), Music Requests (Spotify),
Upload Photos, Guest Book, Meet the Cats, Share.

**This file is the authoritative "what's actually true right now" reference.**
`docs/superpowers/specs/2026-07-27-homesite-design.md` and
`docs/superpowers/plans/2026-07-27-homesite-implementation.md` describe the
*original* v1 design and the 19-task plan that built it — both are historical
planning artifacts from before the redesign pass this file documents, and no
longer fully match reality (visual design, page structure, and the Music
page's interaction model have all changed materially since). Trust this file
over them for anything currently deployed.

## Stack

Go, `github.com/a-h/templ` (compile-time HTML templates), HTMX (fragment
swaps, no JS build step), `modernc.org/sqlite` (pure Go, no CGO — this is
what makes cross-compiling from macOS to linux/arm64 painless), ImageMagick 7
(`magick`, not `convert`) for HEIC decoding and EXIF stripping.

## Local dev

```
make dev    # templ generate --watch + go run, config.local.yaml, :8080
make test   # go test ./...
```

## Deployment (live, real hardware)

- **Pi**: SSH as `apollo@apollo` (mDNS hostname `apollo` — not `apollo.local`;
  the two are separate `known_hosts` entries and only `apollo` has a trusted
  key from setup, so keep using the bare form for `ssh`/`scp`/`Makefile`).
  Currently at `192.168.1.186` on wifi (`wlan0`), MAC `dc:a6:32:2b:e6:cf`,
  Debian 13 (Trixie), arm64. Passwordless SSH key auth is set up
  (`~/.ssh/id_ed25519`) — no password needed for normal SSH.
- **Service user**: `homesite` (system user, runs the site via systemd, home
  `/srv/homesite`). Your login user `apollo` is in the `homesite` group so it
  can write `/srv/homesite` (0775 root:homesite) without sudo.
- **sudo**: `apollo`'s sudo needs a password *except* for a narrow allowlist
  in `/etc/sudoers.d/homesite-deploy` (exact-argument match, so e.g. adding
  `--no-pager` to a whitelisted command will still prompt): `systemctl
  restart|status homesite`, `systemctl daemon-reload`, `systemctl enable --now
  homesite`, `systemctl enable --now|restart homesite-backup.timer`,
  `systemctl status homesite-backup.timer`. Anything else needs the password
  (which the user has — ask them to run it, or use `echo password | sudo -S
  ...` if they've shared it in-session; don't assume it's still `password`
  indefinitely, it's a very weak default they were told to change).
- **Deploy**: `make deploy` from the Mac — builds `GOOS=linux GOARCH=arm64`,
  `scp`s to `/srv/homesite/homesite.new`, then `ssh`-`mv`s it over the real
  path. **Don't `scp` directly to `/srv/homesite/homesite`** — it's the
  currently-executing binary and a direct overwrite fails with "text file
  busy" (ETXTBSY); the temp-file-then-rename is why the Makefile does it in
  two steps.
- **Config**: `/srv/homesite/config.yaml` on the Pi (gitignored, has the real
  WiFi password and Spotify client secret — not in git, don't try to
  reconstruct it from the repo). `listen: ":80"`, `url: "http://home.local"`,
  `fallback_url: "http://192.168.1.186"`.
- **Domain (`home.local`)**: resolves two ways, both needed —
  1. `deploy/homesite-mdns.service` (installed at
     `/etc/systemd/system/homesite-mdns.service`, enabled, `Restart=always`)
     runs `avahi-publish -a -R home.local 192.168.1.186` — true mDNS, for
     Apple devices (iPhone/Mac only resolve `.local` over multicast DNS, and
     often never fall back to a router's regular DNS for that TLD).
  2. A UniFi **Local DNS Record** (`home.local` → `192.168.1.186`) for
     everything else. **Status as of this writing: instructions were given
     to the user but not confirmed done** — verify before assuming it works
     on non-Apple devices.
  3. Also depends on a UniFi **DHCP reservation** for the Pi's MAC so
     `192.168.1.186` never changes — **also not confirmed done.** If the Pi's
     IP ever changes, the mDNS service's hardcoded IP in
     `homesite-mdns.service` needs updating (redeploy that unit + restart),
     and `config.yaml`'s `fallback_url` too.
- **Spotify**: real app created, credentials already deployed and working —
  the one-time OAuth (`ssh -L 8080:localhost:80 apollo@apollo`, then visit
  `http://127.0.0.1:8080/spotify/login` locally) is done and the refresh
  token is stored server-side. Only needs redoing if revoked from Spotify's
  connected-apps settings. The host's Spotify account needs **Premium** —
  queue-add and other playback-modification endpoints 403 without it
  (mapped to `spotify.ErrPremiumRequired` → "Queueing needs Spotify Premium
  on the host's account").
- **Backups**: `deploy/homesite-backup.service`/`.timer` exist but are **not
  installed/enabled** — needs `/srv/homesite/backup.env` with restic + B2
  credentials first, which hasn't been set up. `GET /healthz` will show
  `"last_backup": "never"` until this is done.
- **Live data**: as of 2026-07-28, all test photos and guest book entries
  were deliberately deleted from the Pi (`guestbook_entries` and `photos`
  tables emptied, uploaded files removed) — the site is clean and ready for
  real party use, this wasn't an accident.

## What changed in the post-launch redesign

The original spec/plan (see note above) describe the site as first built.
Since then, in response to iterative user feedback, the following changed
materially:

**Visual design** — light theme only (dark mode fully removed, no
`prefers-color-scheme` anywhere). Palette: `--ground: #fbf8f6`, `--ink:
#151515`, `--accent: #06402b` (a deep MCM bottle green — the header bar's
background, not just an accent color). Display face is **Cormorant
Garamond** (600), not Fraunces — Fraunces read as too heavy/"thick" per
feedback and was fully replaced (font files removed, `@font-face` deleted).
Body stays Lora; Space Grotesk is still the "structural/UI" face (nav,
labels, buttons, captions, tags). The site went through several signature-
motif iterations (a ball-terminal dot before text, a left-border stripe, a
corner medallion) — all were explicitly rejected by the user as looking
"like a bullet list" or not matching the desired MCM feel; nav-cards are
now **plain bordered cards with no accent device**. Header: hamburger menu
on the left, a small white/cream circular favicon badge centered on the
green bar (`static/brand/header-icon.png`); the expanded nav overlay shows
the full "Brivin" wordmark top-left. All photos (cats, guest book, gallery)
share one CSS filter (`saturate(0.88) contrast(1.06) brightness(1.01)
sepia(0.06)`) plus a faint SVG-noise grain overlay (`.photo-frame::after`)
for a consistent "shot on one roll" look. Photos open in a pure-CSS
checkbox-toggle lightbox (`views/lightbox.templ`, `PhotoLightbox` component)
on both Upload Photos and Guest Book — no JS, and it's a real `<img>` so
long-press-to-save still works on mobile.

**Content/IA** — `content/coffee.yaml` was deleted; its items are now a
"Coffee" section inside `content/drinks.yaml` (renamed from
`cocktails.yaml`). `content/refreshments.yaml` was renamed to `food.yaml`.
The `/coffee` route is gone (404s); `/cocktails` and `/refreshments` routes
are unchanged but now serve "Drinks" and "Food" respectively (nav labels
and page titles updated, URLs kept stable). A new Entertainment page
(`/entertainment`, `views/entertainment.templ`) was added between Food and
Music Requests — just a "jackbox.tv" link button. **Cat detail pages are
gone** — `/cats/{slug}` 404s now; `internal/content.Cat` gained a
`Birthday` field (was `Adopted`, renamed) and a `PetSpots` field, and every
cat's full bio/birthday/likes/dislikes/pet-spots renders inline on `/cats`
in one scrollable page, no more click-through.

**Music page** — substantially rebuilt. Album art now shows everywhere
(now-playing, search results, queue) via `spotify.Track.AlbumArtURL` and a
shared `trackItem`/`toTrack()` parser in `internal/spotify/spotify.go`. A
CSS equalizer-bar animation runs next to the now-playing track. The
now-playing card polls every 6s (`GET /music/now-playing`, its own fragment,
**not** including the queue — polling the queue too would silently kick a
guest out of an in-progress search). The queue and search results now share
one panel (`#queue-panel`/`GET /music/queue` for the default queue view,
`GET /music/search` swaps in search results with a sticky ✕-close header);
typing in the search box swaps the panel's content, closing or successfully
queueing a track swaps it back to the queue view (the latter via an HTMX
out-of-band swap alongside a toast). The queue list is uncapped and the
panel is sized to most of the viewport (`min-height: 45vh; max-height:
58vh`) with internal scroll. Successfully queueing shows a floating,
self-dismissing CSS-only toast (`@keyframes`, ~2.6s, with a static
non-animated fallback under `prefers-reduced-motion`) instead of a message
that used to permanently replace the search results. The old "Recently
added" section (our own DB-tracked request history, distinct from Spotify's
live queue) was removed from the UI as redundant — `spotify.RequestStore.
Recent()` and its store-level test are untouched and still work, just
unused by any page now. `spotify.Client.QueueTrack` now distinguishes a 403
into `ErrPremiumRequired` vs `ErrPlaybackRejected` by inspecting the
response body, and logs that body — this is what was missing when a queue
failure first showed up as an opaque "failed" with nothing actionable in
the logs.

**Uploads** — both Upload Photos and Guest Book replaced the native file-
input chrome with a big circular "+" tap target. Upload Photos auto-submits
the moment a photo is picked (`hx-trigger="change"` on the form — no
separate Upload button). Guest Book's photo field does *not* auto-submit
(it's one field among Name/Message, submitted together via "Sign the guest
book"). Both show a CSS spinner during the request via HTMX's standard
`htmx-indicator`/`htmx-request` class convention. Guest Book entries render
in a 1–2 column responsive grid, photo-led, message styled as an italic
quote — no borders between entries, grid gap only.

## Recipes (household-only, not a party-guest feature)

`/recipes` (index, with client-side search/tag filtering) and
`/recipes/{slug}` (detail) exist. Content lives in `content/recipes/*.md`
(YAML frontmatter + markdown body), loaded via `internal/content.RecipeLoader`
— mirrors the `Cat`/`CatLoader` pattern. This is **deliberately not** in the
guest-facing hamburger menu (`views.NavItems` in `views/nav.templ`) — it's a
page for the household, not party guests, and its absence from nav is
intentional, not an oversight (`TestRecipesNotInGuestNav` locks this in).
There's no in-app scraper or admin UI for adding recipes: the workflow is
Kevin hands Claude a URL, and Claude fetches/cleans/writes the `.md` content
file directly.

## Known gaps / pending

- UniFi DHCP reservation + Local DNS Record for `home.local` — instructions
  given, not confirmed done.
- Backups (`homesite-backup.timer`) — not configured, needs
  `backup.env` with restic/B2 credentials.
- The weak `apollo` login password — user was told to change it
  (`passwd`), not confirmed done.
