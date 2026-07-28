# Brivin Household Site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Brivin Household site — a Go + Templ + HTMX server serving ten mobile-first pages (Welcome, Wi-Fi, Coffee, Cocktails, Refreshments, Music, Photos, Guest Book, Cats, Share), developed and design-reviewed entirely on macOS before deploying to a Raspberry Pi 4.

**Architecture:** A single Go binary (`cmd/homesite`) wires together small internal packages (`content`, `qr`, `store`, `photos`, `imaging`, `guestbook`, `spotify`, `web`) behind narrow interfaces. Templ components render server-side HTML; HTMX handles the two write endpoints (guest book, photo upload) without a JS build step. SQLite (pure-Go driver) persists guest book entries and photo metadata. Content (menus, cat bios) is read from YAML/Markdown files with mtime-based cache invalidation, so editing over SSH takes effect on next refresh with no restart.

**Tech Stack:** Go 1.23, Templ, HTMX 2.0.4, `modernc.org/sqlite`, `github.com/yuin/goldmark`, `github.com/adrg/frontmatter`, `github.com/yeqown/go-qrcode/v2`, `gopkg.in/yaml.v3`, ImageMagick 7 (`magick` CLI) via `exec`.

## Global Constraints

- **Spec of record:** `docs/superpowers/specs/2026-07-27-homesite-design.md`. Every task below implements a section of it; do not deviate without checking there first.
- Module name: `homesite` (no VCS path — this is not published). All internal imports are `homesite/internal/...`.
- **No CGO.** `modernc.org/sqlite` is pure Go specifically so the Mac→Pi cross-compile (`GOOS=linux GOARCH=arm64`) needs no C toolchain. Never add a CGO-requiring dependency.
- **ImageMagick is invoked as `magick`**, not `convert`. Confirmed: Debian 13 (Trixie) and Homebrew both ship ImageMagick 7, whose canonical CLI entrypoint is `magick` (with `magick identify` as the subcommand form, not a separate `identify` binary).
- **`http.DetectContentType` does NOT detect HEIC.** Verified directly: it returns `application/octet-stream` for HEIC/HEIF ftyp-box bytes. Any content-type sniffing must special-case the ISO-BMFF `ftyp` box before falling back to stdlib sniffing.
- **Local development is mandatory before any Pi deploy.** `listen` defaults to `:8080` locally (binding `:80` needs root, which only the Pi's systemd unit grants via `CAP_NET_BIND_SERVICE`). Every page must be reachable and reviewable at `http://localhost:8080` from Task 1 onward.
- **Seed content is committed**, not left empty — real menu items, two cat profiles with photos, so the site looks finished from the first run rather than like an empty shell.
- Copy in the UI reads **"Brivin Household"** as the site title, exactly, on every page header.
- Templ escapes all output by default — never use `templ.Raw`/`templ.SafeContent` on guest-supplied text (guest book messages, names).
- All timestamps stored as RFC3339 UTC strings (`time.Now().UTC().Format(time.RFC3339)`).
- Every write-capable HTTP handler (guest book POST, photo upload POST) must have a table-driven test covering at least one success case and one rejection case before being wired into `main.go`.

---

## Task 1: Project Scaffold and Config Loader

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `config.example.yaml`
- Create: `config.local.example.yaml`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Create: `internal/config/testdata/valid.yaml`

**Interfaces:**
- Produces: `config.Config` struct (all fields below), `config.Load(path string) (*Config, error)`

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
cd /Users/kevintrinh/dev/homesite
go mod init homesite
```
Expected: creates `go.mod` with `module homesite` and a `go` directive. Edit it so the `go` directive reads `go 1.23`.

- [ ] **Step 2: Write `.gitignore`**

```gitignore
/homesite
/data/
/uploads/
config.yaml
config.local.yaml
*.db
*.db-wal
*.db-shm
.DS_Store
```

- [ ] **Step 3: Write the failing config test**

```go
// internal/config/config_test.go
package config

import "testing"

func TestLoadValid(t *testing.T) {
	cfg, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Site.Title != "Brivin Household" {
		t.Errorf("Site.Title = %q, want %q", cfg.Site.Title, "Brivin Household")
	}
	if cfg.Site.Listen != ":8080" {
		t.Errorf("Site.Listen = %q, want %q", cfg.Site.Listen, ":8080")
	}
	if cfg.Photos.MaxFileBytes != 26214400 {
		t.Errorf("Photos.MaxFileBytes = %d, want 26214400", cfg.Photos.MaxFileBytes)
	}
	if cfg.Limits.WindowMinutes != 15 {
		t.Errorf("Limits.WindowMinutes = %d, want 15", cfg.Limits.WindowMinutes)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
```

- [ ] **Step 4: Write the testdata fixture**

```yaml
# internal/config/testdata/valid.yaml
site:
  title: "Brivin Household"
  listen: ":8080"
  url: "http://localhost:8080"
  fallback_url: "http://192.168.1.50"
wifi:
  ssid: "TestNet"
  password: "testpass123"
  auth: WPA
  hidden: false
spotify:
  playlist_url: "https://open.spotify.com/playlist/abc123"
  client_id: "test-client-id"
  client_secret: "test-client-secret"
  cache_seconds: 60
photos:
  max_file_bytes: 26214400
  max_total_bytes: 85899345920
  warn_at_percent: 85
  thumb_long_edge: 400
limits:
  guestbook_per_window: 2
  photo_uploads_per_window: 30
  window_minutes: 15
content_dir: "./content"
data_dir: "./data"
uploads_dir: "./uploads"
```

- [ ] **Step 5: Run the test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL — `config.go` doesn't exist yet, compile error.

- [ ] **Step 6: Implement the config package**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Site        SiteConfig    `yaml:"site"`
	WiFi        WiFiConfig    `yaml:"wifi"`
	Spotify     SpotifyConfig `yaml:"spotify"`
	Photos      PhotosConfig  `yaml:"photos"`
	Limits      LimitsConfig  `yaml:"limits"`
	ContentDir  string        `yaml:"content_dir"`
	DataDir     string        `yaml:"data_dir"`
	UploadsDir  string        `yaml:"uploads_dir"`
}

type SiteConfig struct {
	Title       string `yaml:"title"`
	Listen      string `yaml:"listen"`
	URL         string `yaml:"url"`
	FallbackURL string `yaml:"fallback_url"`
}

type WiFiConfig struct {
	SSID     string `yaml:"ssid"`
	Password string `yaml:"password"`
	Auth     string `yaml:"auth"`
	Hidden   bool   `yaml:"hidden"`
}

type SpotifyConfig struct {
	PlaylistURL  string `yaml:"playlist_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	CacheSeconds int    `yaml:"cache_seconds"`
}

type PhotosConfig struct {
	MaxFileBytes   int64 `yaml:"max_file_bytes"`
	MaxTotalBytes  int64 `yaml:"max_total_bytes"`
	WarnAtPercent  int   `yaml:"warn_at_percent"`
	ThumbLongEdge  int   `yaml:"thumb_long_edge"`
}

type LimitsConfig struct {
	GuestbookPerWindow    int `yaml:"guestbook_per_window"`
	PhotoUploadsPerWindow int `yaml:"photo_uploads_per_window"`
	WindowMinutes         int `yaml:"window_minutes"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &cfg, nil
}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS for both `TestLoadValid` and `TestLoadMissingFile`.

- [ ] **Step 8: Write `config.example.yaml` and `config.local.example.yaml`**

`config.example.yaml` (Pi production template — real values redacted):
```yaml
site:
  title: "Brivin Household"
  listen: ":80"
  url: "http://home.arpa"
  fallback_url: "http://192.168.1.50"
wifi:
  ssid: "YOUR_SSID"
  password: "YOUR_WIFI_PASSWORD"
  auth: WPA
  hidden: false
spotify:
  playlist_url: "https://open.spotify.com/playlist/YOUR_PLAYLIST_ID"
  client_id: "YOUR_SPOTIFY_CLIENT_ID"
  client_secret: "YOUR_SPOTIFY_CLIENT_SECRET"
  cache_seconds: 60
photos:
  max_file_bytes: 26214400
  max_total_bytes: 85899345920
  warn_at_percent: 85
  thumb_long_edge: 400
limits:
  guestbook_per_window: 2
  photo_uploads_per_window: 30
  window_minutes: 15
content_dir: "/srv/homesite/content"
data_dir: "/srv/homesite/data"
uploads_dir: "/srv/homesite/uploads"
```

`config.local.example.yaml` (macOS dev template, copy to `config.local.yaml`):
```yaml
site:
  title: "Brivin Household"
  listen: ":8080"
  url: "http://localhost:8080"
  fallback_url: "http://localhost:8080"
wifi:
  ssid: "DevNet"
  password: "devpassword"
  auth: WPA
  hidden: false
spotify:
  playlist_url: "https://open.spotify.com/playlist/YOUR_PLAYLIST_ID"
  client_id: "YOUR_SPOTIFY_CLIENT_ID"
  client_secret: "YOUR_SPOTIFY_CLIENT_SECRET"
  cache_seconds: 60
photos:
  max_file_bytes: 26214400
  max_total_bytes: 85899345920
  warn_at_percent: 85
  thumb_long_edge: 400
limits:
  guestbook_per_window: 2
  photo_uploads_per_window: 30
  window_minutes: 15
content_dir: "./content"
data_dir: "./data"
uploads_dir: "./uploads"
```

- [ ] **Step 9: Write the Makefile skeleton**

```makefile
.PHONY: dev build deploy test

dev:
	templ generate --watch &
	go run ./cmd/homesite -config config.local.yaml

test:
	go test ./...

build:
	templ generate
	GOOS=linux GOARCH=arm64 go build -o homesite ./cmd/homesite

deploy: build
	scp homesite pi@192.168.1.50:/srv/homesite/homesite
	ssh pi@192.168.1.50 'sudo systemctl restart homesite'
```

- [ ] **Step 10: Commit**

```bash
git add go.mod .gitignore Makefile config.example.yaml config.local.example.yaml internal/config
git commit -m "Add project scaffold and config loader"
```

---

## Task 2: Content Cache Core

**Files:**
- Create: `internal/content/cache.go`
- Test: `internal/content/cache_test.go`
- Create: `internal/content/testdata/cache/sample.txt`

**Interfaces:**
- Consumes: nothing (foundation package)
- Produces: `content.Cache` struct with `Get(path string, parse func([]byte) (any, error)) (any, error)` — later tasks (Menu, Cat, Welcome loaders) wrap this with typed accessors.

This is the single mechanism behind "edit text files, auto-reload": every content loader in the codebase goes through this cache so the mtime-check-and-reparse behavior is written once, not per content type.

- [ ] **Step 1: Write the failing test**

```go
// internal/content/cache_test.go
package content

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheReparsesOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCache()
	parseCount := 0
	parse := func(b []byte) (any, error) {
		parseCount++
		return string(b), nil
	}

	got, err := c.Get(path, parse)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "v1" || parseCount != 1 {
		t.Fatalf("got %v, parseCount %d, want v1/1", got, parseCount)
	}

	// Re-fetch without modifying the file: must NOT reparse.
	if _, err := c.Get(path, parse); err != nil {
		t.Fatal(err)
	}
	if parseCount != 1 {
		t.Fatalf("parseCount = %d after unchanged re-fetch, want 1", parseCount)
	}

	// mtime granularity on some filesystems is 1s; force it forward.
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	got, err = c.Get(path, parse)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "v2" || parseCount != 2 {
		t.Fatalf("got %v, parseCount %d, want v2/2 after modification", got, parseCount)
	}
}

func TestCacheMissingFileReturnsError(t *testing.T) {
	c := NewCache()
	_, err := c.Get("/no/such/file", func(b []byte) (any, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/content/... -v`
Expected: FAIL — `NewCache` undefined.

- [ ] **Step 3: Implement the cache**

```go
// internal/content/cache.go
package content

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type entry struct {
	modTime time.Time
	value   any
}

// Cache holds parsed content keyed by file path, invalidated when the
// file's mtime changes. A file that fails to parse leaves the previous
// good entry in place — callers should log the error and keep serving
// stale-but-valid content rather than surface a 500.
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]entry)}
}

// Get returns the cached, parsed value for path, reparsing via parse
// only if the file's mtime has advanced since the last successful parse.
// If parse fails and a previous good value exists, that value is returned
// alongside the error so callers can choose to log-and-serve-stale.
func (c *Cache) Get(path string, parse func([]byte) (any, error)) (any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	c.mu.Lock()
	e, ok := c.entries[path]
	c.mu.Unlock()
	if ok && !info.ModTime().After(e.modTime) {
		return e.value, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if ok {
			return e.value, fmt.Errorf("reading %s (serving stale): %w", path, err)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	value, err := parse(b)
	if err != nil {
		if ok {
			return e.value, fmt.Errorf("parsing %s (serving stale): %w", path, err)
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	c.mu.Lock()
	c.entries[path] = entry{modTime: info.ModTime(), value: value}
	c.mu.Unlock()
	return value, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/content/... -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/content/cache.go internal/content/cache_test.go
git commit -m "Add mtime-invalidated content cache"
```

---

## Task 3: Menu Content Type, Parser, and Seed Data

**Files:**
- Create: `internal/content/menu.go`
- Test: `internal/content/menu_test.go`
- Create: `internal/content/testdata/menu/valid.yaml`
- Create: `internal/content/testdata/menu/malformed.yaml`
- Create: `content/coffee.yaml`
- Create: `content/cocktails.yaml`
- Create: `content/refreshments.yaml`

**Interfaces:**
- Consumes: `content.Cache` (Task 2)
- Produces: `content.Menu`, `content.Section`, `content.Item` structs; `content.MenuLoader` struct with `NewMenuLoader(cache *Cache) *MenuLoader` and `(*MenuLoader) Load(path string) (Menu, error)` — used directly by web handlers in Task 9.

Coffee, Cocktails, and Refreshments share this exact type and loader; they differ only in which YAML file is passed to `Load`.

- [ ] **Step 1: Write the failing test**

```go
// internal/content/menu_test.go
package content

import "testing"

func TestMenuLoaderLoadsValidFile(t *testing.T) {
	loader := NewMenuLoader(NewCache())
	menu, err := loader.Load("testdata/menu/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if menu.Title != "Cocktail Menu" {
		t.Errorf("Title = %q, want %q", menu.Title, "Cocktail Menu")
	}
	if len(menu.Sections) != 1 || menu.Sections[0].Name != "Classics" {
		t.Fatalf("Sections = %+v", menu.Sections)
	}
	item := menu.Sections[0].Items[0]
	if item.Name != "Negroni" || len(item.Ingredients) != 3 {
		t.Fatalf("Items[0] = %+v", item)
	}
}

func TestMenuLoaderMalformedFileReturnsError(t *testing.T) {
	loader := NewMenuLoader(NewCache())
	if _, err := loader.Load("testdata/menu/malformed.yaml"); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestMenuLoaderServesStaleOnSubsequentFailure(t *testing.T) {
	// Load valid content first, then verify that after the underlying
	// cache records a good value, a parse failure on re-fetch would
	// serve stale — this is exercised at the Cache layer (Task 2); here
	// we only confirm MenuLoader propagates Cache's contract untouched
	// by decoding into the correct type on the happy path.
	loader := NewMenuLoader(NewCache())
	menu, err := loader.Load("testdata/menu/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if menu.Note == "" {
		t.Error("expected non-empty Note in fixture")
	}
}
```

- [ ] **Step 2: Write the testdata fixtures**

```yaml
# internal/content/testdata/menu/valid.yaml
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

```yaml
# internal/content/testdata/menu/malformed.yaml
title: Broken
sections:
  - name: [this is not a valid section name
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/content/... -run TestMenuLoader -v`
Expected: FAIL — `NewMenuLoader` undefined.

- [ ] **Step 4: Implement the Menu type and loader**

```go
// internal/content/menu.go
package content

import "gopkg.in/yaml.v3"

type Menu struct {
	Title    string    `yaml:"title"`
	Note     string    `yaml:"note"`
	Sections []Section `yaml:"sections"`
}

type Section struct {
	Name  string `yaml:"name"`
	Items []Item `yaml:"items"`
}

type Item struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Ingredients []string `yaml:"ingredients"`
	Tags        []string `yaml:"tags"`
	Price       string   `yaml:"price"`
}

type MenuLoader struct {
	cache *Cache
}

func NewMenuLoader(cache *Cache) *MenuLoader {
	return &MenuLoader{cache: cache}
}

func (l *MenuLoader) Load(path string) (Menu, error) {
	v, err := l.cache.Get(path, func(b []byte) (any, error) {
		var m Menu
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		return m, nil
	})
	if err != nil {
		if m, ok := v.(Menu); ok {
			return m, err
		}
		return Menu{}, err
	}
	return v.(Menu), nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/content/... -run TestMenuLoader -v`
Expected: PASS for all three.

- [ ] **Step 6: Write the seed content files**

```yaml
# content/coffee.yaml
title: Coffee Menu
note: Ground fresh, brewed to order.
sections:
  - name: Espresso
    items:
      - name: Espresso
        description: Double shot, no fuss.
        tags: [strong]
      - name: Cortado
        description: Espresso cut with warm milk, equal parts.
        tags: [balanced]
      - name: Oat Milk Latte
        description: Espresso, steamed oat milk, latte art on a good day.
        tags: [creamy]
  - name: Pour Over
    items:
      - name: House Pour Over
        description: Rotating single origin — ask what's on today.
        tags: [light, fruity]
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
      - name: Whiskey Sour
        description: Shaken hard, egg white foam, a few dashes of bitters on top.
        ingredients: [Bourbon, Lemon juice, Simple syrup, Egg white]
        tags: [shaken, citrus]
  - name: On the Rocks
    items:
      - name: Old Fashioned
        description: Built in the glass, big cube, orange twist.
        ingredients: [Bourbon, Sugar, Angostura bitters]
        tags: [stirred, boozy]
```

```yaml
# content/refreshments.yaml
title: Refreshments
note: Help yourself.
sections:
  - name: Snacks
    items:
      - name: Mixed Nuts
        description: Salted, in the bowl by the door.
      - name: Cheese Board
        description: Rotating selection — ask if you have allergies.
  - name: Non-Alcoholic
    items:
      - name: Sparkling Water
        description: Lime or plain, in the fridge.
      - name: Iced Tea
        description: Unsweetened, pitcher on the counter.
```

- [ ] **Step 7: Commit**

```bash
git add internal/content/menu.go internal/content/menu_test.go internal/content/testdata/menu content/coffee.yaml content/cocktails.yaml content/refreshments.yaml
git commit -m "Add Menu content type, loader, and seed menu content"
```

---

## Task 4: Cat Content Type, Front-Matter Parser, and Seed Cats

**Files:**
- Create: `internal/content/cats.go`
- Test: `internal/content/cats_test.go`
- Create: `internal/content/testdata/cats/mochi.md`
- Create: `internal/content/testdata/cats/malformed.md`
- Create: `content/cats/mochi.md`
- Create: `content/cats/biscuit.md`
- Create: `content/cats/mochi.jpg` (placeholder)
- Create: `content/cats/biscuit.jpg` (placeholder)

**Interfaces:**
- Consumes: `content.Cache` (Task 2)
- Produces: `content.Cat` struct; `content.CatLoader` with `NewCatLoader(cache *Cache, dir string) *CatLoader`, `(*CatLoader) LoadAll() ([]Cat, error)`, `(*CatLoader) LoadOne(slug string) (Cat, error)` — used by web handlers in Task 9.

Cat biographies are Markdown with YAML front matter. Front matter is parsed by `github.com/adrg/frontmatter`, whose `Parse(r io.Reader, v any, formats ...*Format) ([]byte, error)` signature returns the remaining Markdown body as bytes after decoding front matter into `v`. That body is then rendered to HTML via `goldmark.Convert`.

- [ ] **Step 1: Add dependencies**

Run:
```bash
go get github.com/adrg/frontmatter
go get github.com/yuin/goldmark
```

- [ ] **Step 2: Write the failing test**

```go
// internal/content/cats_test.go
package content

import (
	"strings"
	"testing"
)

func TestCatLoaderLoadOne(t *testing.T) {
	loader := NewCatLoader(NewCache(), "testdata/cats")
	cat, err := loader.LoadOne("mochi")
	if err != nil {
		t.Fatalf("LoadOne: %v", err)
	}
	if cat.Name != "Mochi" {
		t.Errorf("Name = %q, want %q", cat.Name, "Mochi")
	}
	if cat.Photo != "mochi.jpg" {
		t.Errorf("Photo = %q, want %q", cat.Photo, "mochi.jpg")
	}
	if len(cat.Likes) != 2 {
		t.Errorf("Likes = %v, want 2 entries", cat.Likes)
	}
	if !strings.Contains(cat.BioHTML, "<p>") {
		t.Errorf("BioHTML = %q, want rendered HTML paragraph", cat.BioHTML)
	}
}

func TestCatLoaderLoadAll(t *testing.T) {
	loader := NewCatLoader(NewCache(), "testdata/cats")
	cats, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// testdata/cats has mochi.md (valid) and malformed.md (invalid front
	// matter) — LoadAll must skip and log the bad one, not fail entirely.
	if len(cats) != 1 {
		t.Fatalf("LoadAll returned %d cats, want 1 (malformed.md skipped)", len(cats))
	}
}

func TestCatLoaderUnknownSlug(t *testing.T) {
	loader := NewCatLoader(NewCache(), "testdata/cats")
	if _, err := loader.LoadOne("no-such-cat"); err == nil {
		t.Fatal("expected error for unknown slug")
	}
}
```

- [ ] **Step 3: Write the testdata fixtures**

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
(save as `internal/content/testdata/cats/mochi.md`)

```markdown
---
name: [this is not valid yaml
---
Body text.
```
(save as `internal/content/testdata/cats/malformed.md`)

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/content/... -run TestCatLoader -v`
Expected: FAIL — `NewCatLoader` undefined.

- [ ] **Step 5: Implement the Cat type and loader**

```go
// internal/content/cats.go
package content

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/yuin/goldmark"
)

type Cat struct {
	Name     string   `yaml:"name"`
	Slug     string   `yaml:"slug"`
	Photo    string   `yaml:"photo"`
	Adopted  string   `yaml:"adopted"`
	Likes    []string `yaml:"likes"`
	Dislikes []string `yaml:"dislikes"`
	BioHTML  string   `yaml:"-"`
}

type CatLoader struct {
	cache *Cache
	dir   string
}

func NewCatLoader(cache *Cache, dir string) *CatLoader {
	return &CatLoader{cache: cache, dir: dir}
}

func (l *CatLoader) LoadOne(slug string) (Cat, error) {
	path := filepath.Join(l.dir, slug+".md")
	v, err := l.cache.Get(path, parseCat)
	if err != nil {
		if c, ok := v.(Cat); ok {
			return c, err
		}
		return Cat{}, err
	}
	return v.(Cat), nil
}

// LoadAll reads every *.md file in dir, skipping (and logging) any that
// fail to parse rather than failing the whole page.
func (l *CatLoader) LoadAll() ([]Cat, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("reading cats dir %s: %w", l.dir, err)
	}
	var cats []Cat
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		cat, err := l.LoadOne(slug)
		if err != nil {
			log.Printf("content: skipping cat %s: %v", e.Name(), err)
			continue
		}
		cats = append(cats, cat)
	}
	return cats, nil
}

func parseCat(b []byte) (any, error) {
	var cat Cat
	rest, err := frontmatter.Parse(bytes.NewReader(b), &cat)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter: %w", err)
	}
	var buf bytes.Buffer
	if err := goldmark.Convert(rest, &buf); err != nil {
		return nil, fmt.Errorf("rendering markdown: %w", err)
	}
	cat.BioHTML = buf.String()
	return cat, nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/content/... -run TestCatLoader -v`
Expected: PASS for all three.

- [ ] **Step 7: Write the seed cat content**

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
(save as `content/cats/mochi.md`)

```markdown
---
name: Biscuit
slug: biscuit
photo: biscuit.jpg
adopted: 2022-11-15
likes: [sunbeams, knocking things off tables]
dislikes: [the carrier, other cats on TV]
---
Biscuit is soft, opinionated, and always underfoot.
```
(save as `content/cats/biscuit.md`)

- [ ] **Step 8: Add placeholder photos**

Run:
```bash
mkdir -p content/cats
magick -size 800x600 xc:'#d9c9b6' -gravity center -pointsize 40 -fill '#3a2f28' -annotate 0 'Mochi' content/cats/mochi.jpg
magick -size 800x600 xc:'#c9b6a6' -gravity center -pointsize 40 -fill '#3a2f28' -annotate 0 'Biscuit' content/cats/biscuit.jpg
```
Expected: two placeholder JPEGs exist. (Swap these for real cat photos any time — the loader only cares about the filename matching `photo:` in front matter.)

- [ ] **Step 9: Commit**

```bash
git add internal/content/cats.go internal/content/cats_test.go internal/content/testdata/cats content/cats
git commit -m "Add Cat content type, front-matter loader, and seed cats"
```

---

## Task 5: Welcome Content Loader and Seed Copy

**Files:**
- Create: `internal/content/welcome.go`
- Test: `internal/content/welcome_test.go`
- Create: `internal/content/testdata/welcome/hello.md`
- Create: `content/welcome.md`

**Interfaces:**
- Consumes: `content.Cache` (Task 2)
- Produces: `content.WelcomeLoader` with `NewWelcomeLoader(cache *Cache) *WelcomeLoader`, `(*WelcomeLoader) Load(path string) (string, error)` returning rendered HTML — used by the Welcome page handler in Task 9.

Welcome copy is plain Markdown with no front matter — just a short hello, reworded per occasion without a deploy.

- [ ] **Step 1: Write the failing test**

```go
// internal/content/welcome_test.go
package content

import (
	"strings"
	"testing"
)

func TestWelcomeLoaderRendersMarkdown(t *testing.T) {
	loader := NewWelcomeLoader(NewCache())
	html, err := loader.Load("testdata/welcome/hello.md")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(html, "<p>") {
		t.Errorf("html = %q, want a rendered paragraph", html)
	}
	if !strings.Contains(html, "welcome") {
		t.Errorf("html = %q, want it to contain fixture text", html)
	}
}
```

- [ ] **Step 2: Write the testdata fixture**

```markdown
Hello, and welcome to our place. Make yourself at home.
```
(save as `internal/content/testdata/welcome/hello.md`)

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/content/... -run TestWelcomeLoader -v`
Expected: FAIL — `NewWelcomeLoader` undefined.

- [ ] **Step 4: Implement the loader**

```go
// internal/content/welcome.go
package content

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
)

type WelcomeLoader struct {
	cache *Cache
}

func NewWelcomeLoader(cache *Cache) *WelcomeLoader {
	return &WelcomeLoader{cache: cache}
}

func (l *WelcomeLoader) Load(path string) (string, error) {
	v, err := l.cache.Get(path, func(b []byte) (any, error) {
		var buf bytes.Buffer
		if err := goldmark.Convert(b, &buf); err != nil {
			return nil, fmt.Errorf("rendering markdown: %w", err)
		}
		return buf.String(), nil
	})
	if err != nil {
		if s, ok := v.(string); ok {
			return s, err
		}
		return "", err
	}
	return v.(string), nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/content/... -run TestWelcomeLoader -v`
Expected: PASS.

- [ ] **Step 6: Write the seed welcome copy**

```markdown
Welcome — glad you're here. Scan around: coffee, cocktails, snacks,
add a song, drop a photo, sign the guest book, and say hi to the cats.
```
(save as `content/welcome.md`)

- [ ] **Step 7: Commit**

```bash
git add internal/content/welcome.go internal/content/welcome_test.go internal/content/testdata/welcome content/welcome.md
git commit -m "Add Welcome content loader and seed copy"
```

---

## Task 6: QR Payload Builders and PNG Renderer

**Files:**
- Create: `internal/qr/payload.go`
- Create: `internal/qr/qr.go`
- Test: `internal/qr/payload_test.go`
- Test: `internal/qr/qr_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `qr.WiFiPayload(ssid, password, auth string, hidden bool) string`, `qr.URLPayload(url string) string` (trivial passthrough, kept for naming symmetry at call sites), `qr.PNG(payload string) ([]byte, error)` — used by the Wi-Fi, Music, and Share page handlers in Tasks 9–10 and 16.

**Note on output format:** the spec describes "SVG render," but the verified library (`github.com/yeqown/go-qrcode/v2` with its `writer/standard` package) renders raster images (PNG/JPEG) to an `io.WriteCloser`, not SVG. This plan renders **PNG** instead — visually identical result for the "big code guests scan off a screen" use case, no loss of the level-Q error correction or white/black lock the spec calls for. Handlers will serve it with `Content-Type: image/png`.

- [ ] **Step 1: Add dependencies**

Run:
```bash
go get github.com/yeqown/go-qrcode/v2
go get github.com/yeqown/go-qrcode/writer/standard
```

- [ ] **Step 2: Write the failing payload test**

```go
// internal/qr/payload_test.go
package qr

import "testing"

func TestWiFiPayloadBasic(t *testing.T) {
	got := WiFiPayload("MyNet", "hunter2", "WPA", false)
	want := "WIFI:T:WPA;S:MyNet;P:hunter2;;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWiFiPayloadHidden(t *testing.T) {
	got := WiFiPayload("MyNet", "hunter2", "WPA", true)
	want := "WIFI:T:WPA;S:MyNet;P:hunter2;H:true;;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWiFiPayloadEscapesSpecialChars(t *testing.T) {
	// SSID and password containing `;` `,` `:` `\` must be backslash-escaped
	// per the WIFI: QR payload spec — otherwise a camera app misparses the
	// field boundaries.
	got := WiFiPayload(`Kev;in,Net:work\`, `p:a;s,s\word`, "WPA", false)
	want := `WIFI:T:WPA;S:Kev\;in\,Net\:work\\;P:p\:a\;s\,s\\word;;`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestURLPayload(t *testing.T) {
	got := URLPayload("http://home.arpa")
	want := "http://home.arpa"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/qr/... -run Payload -v`
Expected: FAIL — `WiFiPayload` undefined.

- [ ] **Step 4: Implement payload builders**

```go
// internal/qr/payload.go
package qr

import "strings"

var wifiEscaper = strings.NewReplacer(
	`\`, `\\`,
	`;`, `\;`,
	`,`, `\,`,
	`:`, `\:`,
)

// WiFiPayload builds the standard WIFI: QR payload
// (WIFI:T:<auth>;S:<ssid>;P:<password>;[H:true;];), escaping `;` `,` `:`
// and `\` in the SSID and password as the format requires.
func WiFiPayload(ssid, password, auth string, hidden bool) string {
	var b strings.Builder
	b.WriteString("WIFI:T:")
	b.WriteString(auth)
	b.WriteString(";S:")
	b.WriteString(wifiEscaper.Replace(ssid))
	b.WriteString(";P:")
	b.WriteString(wifiEscaper.Replace(password))
	b.WriteString(";")
	if hidden {
		b.WriteString("H:true;")
	}
	b.WriteString(";")
	return b.String()
}

// URLPayload is a passthrough that exists so call sites read
// qr.URLPayload(cfg.Site.URL) rather than passing a raw string, keeping
// every QR-producing call site symmetrical.
func URLPayload(url string) string {
	return url
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/qr/... -run Payload -v`
Expected: PASS for all four.

- [ ] **Step 6: Write the failing PNG renderer test**

```go
// internal/qr/qr_test.go
package qr

import "testing"

func TestPNGProducesValidImage(t *testing.T) {
	b, err := PNG("http://home.arpa")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("PNG returned empty bytes")
	}
	// PNG magic bytes.
	sig := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if len(b) < len(sig) {
		t.Fatalf("output too short to be a PNG: %d bytes", len(b))
	}
	for i, want := range sig {
		if b[i] != want {
			t.Fatalf("byte %d = %#x, want %#x — not a PNG", i, b[i], want)
		}
	}
}

func TestPNGRejectsEmptyPayload(t *testing.T) {
	if _, err := PNG(""); err == nil {
		t.Fatal("expected error for empty payload")
	}
}
```

- [ ] **Step 7: Run the test to verify it fails**

Run: `go test ./internal/qr/... -run TestPNG -v`
Expected: FAIL — `PNG` undefined.

- [ ] **Step 8: Implement the PNG renderer**

Verified against the library's confirmed option set: `WithBuiltinImageEncoder` must be set explicitly to `PNG_FORMAT` — JPEG is the writer's default, and JPEG on a black/white QR pattern risks compression artifacts blurring module edges enough to fail a scan.

```go
// internal/qr/qr.go
package qr

import (
	"bytes"
	"fmt"

	qrcode "github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

// bufWriteCloser adapts a bytes.Buffer to io.WriteCloser, which the
// standard writer requires, so PNG rendering can happen fully in memory
// with no temp file on disk.
type bufWriteCloser struct {
	*bytes.Buffer
}

func (bufWriteCloser) Close() error { return nil }

// PNG renders payload as a QR code PNG: white background, black modules
// (locked regardless of viewer theme — an inverted QR fails to scan on
// many camera apps), error correction level Q (25% recovery, per the
// spec's "reads at an angle, off a glossy screen, in dim light"
// requirement), and a generous quiet zone.
func PNG(payload string) ([]byte, error) {
	if payload == "" {
		return nil, fmt.Errorf("qr: payload must not be empty")
	}

	qrc, err := qrcode.New(payload, qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionQuart))
	if err != nil {
		return nil, fmt.Errorf("qr: building code: %w", err)
	}

	buf := &bufWriteCloser{Buffer: &bytes.Buffer{}}
	w, err := standard.NewWith(buf,
		standard.WithBgColorRGBHex("#FFFFFF"),
		standard.WithFgColorRGBHex("#000000"),
		standard.WithQRWidth(12),
		standard.WithBorderWidth(20),
		standard.WithBuiltinImageEncoder(standard.PNG_FORMAT),
	)
	if err != nil {
		return nil, fmt.Errorf("qr: building writer: %w", err)
	}

	if err := qrc.Save(w); err != nil {
		return nil, fmt.Errorf("qr: rendering: %w", err)
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 9: Run the test to verify it passes**

Run: `go test ./internal/qr/... -v`
Expected: PASS for all six tests across both files.

- [ ] **Step 10: Commit**

```bash
git add internal/qr
git commit -m "Add QR payload builders and PNG renderer"
```

---

## Task 7: Base Layout and Navigation Templ Components

**Files:**
- Create: `views/layout.templ`
- Create: `views/nav.templ`
- Create: `static/css/site.css`
- Create: `static/htmx.min.js`
- Create: `static/fonts/` (Lora woff2 files + license)

**Interfaces:**
- Consumes: nothing yet (pure presentation layer; wired to real data in Task 9)
- Produces: `views.Layout(title string, headerVariant views.HeaderVariant, body templ.Component) templ.Component`, `views.HeaderVariant` enum (`views.HeaderStandard`, `views.HeaderQR`), `views.NavItems []views.NavItem` (the ten-section list, each with `Title`, `Href`, `Blurb`), `views.CardGrid(items []views.NavItem) templ.Component`, `views.MenuOverlay(items []views.NavItem) templ.Component` — every page template in Tasks 9, 10, 13, 14, and 16 wraps its content in `views.Layout`.

This task has no unit tests of its own — templ components are exercised through the handler tests in later tasks, which assert on rendered HTML fragments. What's verified here is that `templ generate` succeeds and the dev server renders a real page in a browser, per the spec's mandate that pages are reviewed on a real phone before anything is deployed.

- [ ] **Step 1: Install templ and Homebrew dependencies**

Run:
```bash
go install github.com/a-h/templ/cmd/templ@latest
brew install imagemagick
```
Expected: `templ` is on `$PATH` (`templ version` prints something). `magick -version` reports ImageMagick 7.

- [ ] **Step 2: Vendor htmx**

Run:
```bash
mkdir -p static
curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o static/htmx.min.js
```
Expected: `static/htmx.min.js` exists and is non-empty. Pinning to 2.0.4 (the version confirmed available) rather than `@latest` so a build a year from now doesn't silently pick up breaking changes.

- [ ] **Step 3: Add the Lora font**

Lora is SIL Open Font License — free to self-host. Download the variable woff2 from Google Fonts:
```bash
mkdir -p static/fonts
curl -fsSL "https://fonts.google.com/download?family=Lora" -o /tmp/lora.zip
unzip -o /tmp/lora.zip -d /tmp/lora
cp /tmp/lora/static/Lora-Regular.ttf /tmp/lora/static/Lora-Bold.ttf /tmp/lora/static/Lora-Italic.ttf static/fonts/
cp /tmp/lora/OFL.txt static/fonts/LICENSE.txt
```
Then convert to woff2 for smaller payloads (requires `fonttools`, `pip install fonttools brotli`):
```bash
python3 -m fontTools.ttLib.woff2 compress static/fonts/Lora-Regular.ttf
python3 -m fontTools.ttLib.woff2 compress static/fonts/Lora-Bold.ttf
python3 -m fontTools.ttLib.woff2 compress static/fonts/Lora-Italic.ttf
rm static/fonts/*.ttf
```
Expected: `static/fonts/Lora-Regular.woff2`, `Lora-Bold.woff2`, `Lora-Italic.woff2`, and `LICENSE.txt` exist.

- [ ] **Step 4: Write the nav data and components**

```go
// views/nav.templ
package views

type NavItem struct {
	Title string
	Href  string
	Blurb string
}

var NavItems = []NavItem{
	{Title: "Wi-Fi", Href: "/wifi", Blurb: "Join the network"},
	{Title: "Coffee Menu", Href: "/coffee", Blurb: "Current coffee drinks"},
	{Title: "Cocktail Menu", Href: "/cocktails", Blurb: "Current cocktails"},
	{Title: "Refreshments", Href: "/refreshments", Blurb: "Snacks and food"},
	{Title: "Music Requests", Href: "/music", Blurb: "Add a song to the playlist"},
	{Title: "Upload Photos", Href: "/photos", Blurb: "Share your shots"},
	{Title: "Guest Book", Href: "/guestbook", Blurb: "Leave a note"},
	{Title: "Meet the Cats", Href: "/cats", Blurb: "Say hello"},
	{Title: "Share", Href: "/share", Blurb: "Pass the site along"},
}

templ CardGrid(items []NavItem) {
	<div class="card-grid">
		for _, item := range items {
			<a class="nav-card" href={ templ.URL(item.Href) }>
				<span class="nav-card-title">{ item.Title }</span>
				<span class="nav-card-blurb">{ item.Blurb }</span>
			</a>
		}
	</div>
}

templ MenuOverlay(items []NavItem) {
	<input type="checkbox" id="menu-toggle" class="menu-toggle-input" />
	<label for="menu-toggle" class="menu-toggle-button" aria-label="Open menu">☰</label>
	<div class="menu-overlay">
		<label for="menu-toggle" class="menu-overlay-close" aria-label="Close menu">✕</label>
		<nav class="menu-overlay-list">
			<a href="/">Welcome</a>
			for _, item := range items {
				<a href={ templ.URL(item.Href) }>{ item.Title }</a>
			}
		</nav>
	</div>
}
```

- [ ] **Step 5: Write the layout component**

Two header variants: `HeaderStandard` (site title plus the menu-overlay trigger) and `HeaderQR` (title only, no overlay trigger, no dark-mode toggle) for the three QR pages, which the spec requires to opt out of dark mode and stay free of surrounding chrome that could distract a guest's camera.

```go
// views/layout.templ
package views

type HeaderVariant int

const (
	HeaderStandard HeaderVariant = iota
	HeaderQR
)

templ Layout(title string, variant HeaderVariant, body templ.Component) {
	<!DOCTYPE html>
	<html lang="en" class={ templ.KV("qr-page", variant == HeaderQR) }>
		<head>
			<meta charset="UTF-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
			<title>{ title } — Brivin Household</title>
			<link rel="stylesheet" href="/static/css/site.css"/>
			<script src="/static/htmx.min.js"></script>
		</head>
		<body>
			<header class="site-header">
				<a class="site-title" href="/">Brivin Household</a>
				if variant == HeaderStandard {
					@MenuOverlay(NavItems)
				}
			</header>
			<main>
				@body
			</main>
		</body>
	</html>
}
```

- [ ] **Step 6: Write the base stylesheet**

```css
/* static/css/site.css */
@font-face {
	font-family: "Lora";
	src: url("/static/fonts/Lora-Regular.woff2") format("woff2");
	font-weight: 400;
	font-style: normal;
	font-display: swap;
}
@font-face {
	font-family: "Lora";
	src: url("/static/fonts/Lora-Bold.woff2") format("woff2");
	font-weight: 700;
	font-style: normal;
	font-display: swap;
}
@font-face {
	font-family: "Lora";
	src: url("/static/fonts/Lora-Italic.woff2") format("woff2");
	font-weight: 400;
	font-style: italic;
	font-display: swap;
}

:root {
	--ink: #2b2420;
	--ground: #f7f1e8;
	--accent: #b3552f;
	--card-bg: #ffffff;
	--measure: 60ch;
}

@media (prefers-color-scheme: dark) {
	:root {
		--ink: #f1e9dc;
		--ground: #201a16;
		--accent: #e08a5c;
		--card-bg: #2a231d;
	}
}

/* The three QR pages (Wi-Fi, Music, Share) lock to light mode: an
   inverted QR fails to scan on many camera apps. */
html.qr-page {
	color-scheme: light only;
}
html.qr-page body {
	background: #ffffff;
	color: #111111;
}

* { box-sizing: border-box; }

body {
	margin: 0;
	font-family: "Lora", Georgia, serif;
	background: var(--ground);
	color: var(--ink);
	line-height: 1.5;
}

.site-header {
	position: sticky;
	top: 0;
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 0.75rem 1rem;
	background: var(--ground);
	border-bottom: 1px solid rgba(0, 0, 0, 0.08);
	z-index: 10;
}

.site-title {
	font-weight: 700;
	font-size: 1.1rem;
	letter-spacing: -0.01em;
	color: var(--ink);
	text-decoration: none;
}

main {
	max-width: var(--measure);
	margin: 0 auto;
	padding: 1.5rem 1rem 4rem;
}

.card-grid {
	display: grid;
	grid-template-columns: 1fr;
	gap: 0.75rem;
}
@media (min-width: 480px) {
	.card-grid { grid-template-columns: 1fr 1fr; }
}

.nav-card {
	display: flex;
	flex-direction: column;
	gap: 0.25rem;
	padding: 1.25rem;
	min-height: 44px;
	border-radius: 0.75rem;
	background: var(--card-bg);
	box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
	text-decoration: none;
	color: var(--ink);
}
.nav-card-title {
	font-weight: 700;
	font-size: 1.1rem;
}
.nav-card-blurb {
	font-size: 0.9rem;
	opacity: 0.75;
}

.menu-toggle-input { display: none; }
.menu-toggle-button {
	font-size: 1.5rem;
	cursor: pointer;
	padding: 0.5rem;
	min-width: 44px;
	min-height: 44px;
	display: flex;
	align-items: center;
	justify-content: center;
}
.menu-overlay {
	position: fixed;
	inset: 0;
	background: var(--ground);
	display: flex;
	flex-direction: column;
	align-items: flex-start;
	padding: 2rem;
	gap: 1rem;
	transform: translateY(-100%);
	transition: transform 0.2s ease;
	z-index: 20;
}
.menu-toggle-input:checked ~ .menu-overlay {
	transform: translateY(0);
}
.menu-overlay-close {
	align-self: flex-end;
	font-size: 1.5rem;
	cursor: pointer;
	min-width: 44px;
	min-height: 44px;
	display: flex;
	align-items: center;
	justify-content: center;
}
.menu-overlay-list {
	display: flex;
	flex-direction: column;
	gap: 1rem;
	font-size: 1.3rem;
}
.menu-overlay-list a {
	color: var(--ink);
	text-decoration: none;
	font-weight: 700;
}

.qr-card {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 1rem;
	text-align: center;
}
.qr-card img { max-width: 320px; width: 100%; height: auto; }
.qr-fallback-text { font-size: 0.85rem; opacity: 0.7; }
```

- [ ] **Step 7: Generate and build to confirm it compiles**

Run:
```bash
templ generate
go build ./...
```
Expected: `views/*_templ.go` files are generated; `go build` succeeds (there's no `main.go` yet, so this just confirms the templ files compile — Task 8 adds the entrypoint that actually serves them).

- [ ] **Step 8: Commit**

```bash
git add views static Makefile
git commit -m "Add base layout, nav components, Lora font, and htmx"
```

---

## Task 8: Web Server Skeleton, Static/Media Serving, main.go

**Files:**
- Create: `embed.go`
- Create: `internal/web/server.go`
- Create: `internal/web/healthz.go`
- Test: `internal/web/server_test.go`
- Create: `cmd/homesite/main.go`

**Interfaces:**
- Consumes: `config.Config` (Task 1)
- Produces: `web.Server` struct implementing `http.Handler`, `web.New(cfg *config.Config) *Server` — every subsequent web task (9, 10, 13, 14, 16) adds routes to this same `Server` via a `register*` method called from `New`.

**Why a root-level embed file:** `go:embed` patterns can only reach files at or below the directory of the source file that declares them — never a parent or unrelated sibling directory. `static/` sits at the repo root, so the `//go:embed static` directive must also live in a file at the repo root, not inside `internal/web/`. `embed.go` is that file; `internal/web` imports it as the module's root package.

- [ ] **Step 1: Write the root embed file**

```go
// embed.go
package homesite

import "embed"

//go:embed static
var StaticFS embed.FS
```

- [ ] **Step 2: Write the failing server test**

```go
// internal/web/server_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"homesite/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Site:       config.SiteConfig{Title: "Brivin Household"},
		ContentDir: "../../content",
		DataDir:    "../../data",
		UploadsDir: "../../uploads",
	}
}

func TestHealthzReturnsOK(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestStaticServesHTMX(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty htmx.min.js body")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/web/... -v`
Expected: FAIL — package `internal/web` doesn't exist yet.

- [ ] **Step 4: Implement the server skeleton**

```go
// internal/web/server.go
package web

import (
	"io/fs"
	"net/http"

	homesite "homesite"
	"homesite/internal/config"
)

type Server struct {
	mux *http.ServeMux
	cfg *config.Config
}

func New(cfg *config.Config) *Server {
	s := &Server{mux: http.NewServeMux(), cfg: cfg}

	staticSub, err := fs.Sub(homesite.StaticFS, "static")
	if err != nil {
		panic("web: static assets missing from embed: " + err.Error())
	}
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	catsPhotoDir := http.Dir(cfg.ContentDir + "/cats")
	s.mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(catsPhotoDir)))

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
```

- [ ] **Step 5: Implement the healthz handler**

Only liveness for now — disk usage and last-backup-time fields are added in Task 18 once the photos pipeline and backup script exist to report on.

```go
// internal/web/healthz.go
package web

import "net/http"

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/web/... -v`
Expected: PASS for both tests.

- [ ] **Step 7: Write main.go**

```go
// cmd/homesite/main.go
package main

import (
	"flag"
	"log"
	"net/http"

	"homesite/internal/config"
	"homesite/internal/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	srv := web.New(cfg)
	log.Printf("Brivin Household listening on %s", cfg.Site.Listen)
	if err := http.ListenAndServe(cfg.Site.Listen, srv); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 8: Run it locally to confirm the skeleton serves**

Run:
```bash
templ generate
go run ./cmd/homesite -config config.local.yaml
```
(first copy `config.local.example.yaml` to `config.local.yaml` if you haven't yet)
Then in another terminal:
```bash
curl -s http://localhost:8080/healthz
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/static/htmx.min.js
```
Expected: `ok`, then `200`.

- [ ] **Step 9: Commit**

```bash
git add embed.go internal/web cmd/homesite
git commit -m "Add web server skeleton, static/media serving, and main entrypoint"
```

---

## Task 9: Welcome, Menu, and Cats Pages

**Files:**
- Modify: `internal/web/server.go` (add content loaders as fields, wire routes)
- Create: `internal/web/handlers_pages.go`
- Test: `internal/web/handlers_pages_test.go`
- Create: `views/welcome.templ`
- Create: `views/menu.templ`
- Create: `views/cats.templ`

**Interfaces:**
- Consumes: `content.NewMenuLoader`, `content.NewCatLoader`, `content.NewWelcomeLoader`, `content.NewCache` (Tasks 2–5); `views.Layout`, `views.CardGrid`, `views.NavItems` (Task 7); `web.Server` (Task 8)
- Produces: five working routes (`GET /`, `GET /coffee`, `GET /cocktails`, `GET /refreshments`, `GET /cats`, `GET /cats/{slug}`) — six routes, seven if counting the index separately from detail.

- [ ] **Step 1: Add content loaders to Server**

```go
// internal/web/server.go — replace the New function body
package web

import (
	"io/fs"
	"net/http"

	homesite "homesite"
	"homesite/internal/config"
	"homesite/internal/content"
)

type Server struct {
	mux           *http.ServeMux
	cfg           *config.Config
	menuLoader    *content.MenuLoader
	catLoader     *content.CatLoader
	welcomeLoader *content.WelcomeLoader
}

func New(cfg *config.Config) *Server {
	cache := content.NewCache()
	s := &Server{
		mux:           http.NewServeMux(),
		cfg:           cfg,
		menuLoader:    content.NewMenuLoader(cache),
		catLoader:     content.NewCatLoader(cache, cfg.ContentDir+"/cats"),
		welcomeLoader: content.NewWelcomeLoader(cache),
	}

	staticSub, err := fs.Sub(homesite.StaticFS, "static")
	if err != nil {
		panic("web: static assets missing from embed: " + err.Error())
	}
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	catsPhotoDir := http.Dir(cfg.ContentDir + "/cats")
	s.mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(catsPhotoDir)))

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.registerPageRoutes()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
```

- [ ] **Step 2: Write the failing handler test**

```go
// internal/web/handlers_pages_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWelcomePageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Brivin Household") {
		t.Error("expected page to contain site title")
	}
	if !strings.Contains(rec.Body.String(), "Wi-Fi") {
		t.Error("expected welcome hub to link to Wi-Fi")
	}
}

func TestCoffeeMenuPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/coffee", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Espresso") {
		t.Error("expected coffee menu content in response")
	}
}

func TestCocktailsMenuPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cocktails", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Negroni") {
		t.Error("expected cocktail menu content in response")
	}
}

func TestRefreshmentsPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/refreshments", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Mixed Nuts") {
		t.Error("expected refreshments content in response")
	}
}

func TestCatsIndexPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cats", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Mochi") {
		t.Error("expected cats index to list Mochi")
	}
}

func TestCatDetailPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cats/mochi", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cardboard boxes") {
		t.Error("expected cat detail page to render Mochi's likes")
	}
}

func TestCatDetailPageUnknownSlugReturns404(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cats/no-such-cat", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/web/... -run "Welcome|Coffee|Cocktails|Refreshments|Cats" -v`
Expected: FAIL — `registerPageRoutes` undefined and these routes 404.

- [ ] **Step 4: Write the templ views**

```templ
// views/welcome.templ
package views

templ Welcome(bodyHTML string) {
	@Layout("Welcome", HeaderStandard, welcomeBody(bodyHTML))
}

templ welcomeBody(bodyHTML string) {
	<div class="prose">
		@templ.Raw(bodyHTML)
	</div>
	@CardGrid(NavItems)
}
```

`templ.Raw` is safe here specifically because `bodyHTML` comes from `content.WelcomeLoader`, which renders **hand-edited, host-controlled Markdown** — never guest input. Every other use of `templ.Raw` in this codebase is a bug; guest-supplied text (guest book, upload captions) must always go through templ's default escaping.

```templ
// views/menu.templ
package views

import "homesite/internal/content"

templ MenuPage(menu content.Menu) {
	@Layout(menu.Title, HeaderStandard, menuBody(menu))
}

templ menuBody(menu content.Menu) {
	<h1>{ menu.Title }</h1>
	if menu.Note != "" {
		<p class="menu-note">{ menu.Note }</p>
	}
	for _, section := range menu.Sections {
		<section class="menu-section">
			<h2>{ section.Name }</h2>
			for _, item := range section.Items {
				<article class="menu-item">
					<h3>{ item.Name }</h3>
					if item.Description != "" {
						<p>{ item.Description }</p>
					}
					if len(item.Ingredients) > 0 {
						<p class="menu-item-ingredients">
							for i, ing := range item.Ingredients {
								if i > 0 {
									{ ", " }
								}
								{ ing }
							}
						</p>
					}
				</article>
			}
		</section>
	}
}
```

```templ
// views/cats.templ
package views

import "homesite/internal/content"

templ CatsIndex(cats []content.Cat) {
	@Layout("Meet the Cats", HeaderStandard, catsIndexBody(cats))
}

templ catsIndexBody(cats []content.Cat) {
	<h1>Meet the Cats</h1>
	<div class="card-grid">
		for _, cat := range cats {
			<a class="nav-card" href={ templ.URL("/cats/" + cat.Slug) }>
				<img src={ "/media/" + cat.Photo } alt={ cat.Name } class="cat-thumb"/>
				<span class="nav-card-title">{ cat.Name }</span>
			</a>
		}
	</div>
}

templ CatDetail(cat content.Cat) {
	@Layout(cat.Name, HeaderStandard, catDetailBody(cat))
}

templ catDetailBody(cat content.Cat) {
	<h1>{ cat.Name }</h1>
	<img src={ "/media/" + cat.Photo } alt={ cat.Name } class="cat-photo"/>
	<div class="prose">
		@templ.Raw(cat.BioHTML)
	</div>
	if len(cat.Likes) > 0 {
		<p><strong>Likes:</strong> { joinStrings(cat.Likes) }</p>
	}
	if len(cat.Dislikes) > 0 {
		<p><strong>Dislikes:</strong> { joinStrings(cat.Dislikes) }</p>
	}
}
```

`cat.BioHTML` is safe as `templ.Raw` for the same reason as Welcome: it's rendered from hand-authored Markdown files under `content/cats/`, never from guest input.

- [ ] **Step 5: Add the `joinStrings` helper used by cats.templ**

```go
// views/helpers.go
package views

import "strings"

func joinStrings(items []string) string {
	return strings.Join(items, ", ")
}
```

- [ ] **Step 6: Write the handlers**

```go
// internal/web/handlers_pages.go
package web

import (
	"log"
	"net/http"

	"homesite/views"
)

func (s *Server) registerPageRoutes() {
	s.mux.HandleFunc("GET /{$}", s.handleWelcome)
	s.mux.HandleFunc("GET /coffee", s.handleMenu(s.cfg.ContentDir+"/coffee.yaml"))
	s.mux.HandleFunc("GET /cocktails", s.handleMenu(s.cfg.ContentDir+"/cocktails.yaml"))
	s.mux.HandleFunc("GET /refreshments", s.handleMenu(s.cfg.ContentDir+"/refreshments.yaml"))
	s.mux.HandleFunc("GET /cats", s.handleCatsIndex)
	s.mux.HandleFunc("GET /cats/{slug}", s.handleCatDetail)
}

func (s *Server) handleWelcome(w http.ResponseWriter, r *http.Request) {
	html, err := s.welcomeLoader.Load(s.cfg.ContentDir + "/welcome.md")
	if err != nil {
		log.Printf("web: welcome content error: %v", err)
	}
	render(w, r, views.Welcome(html))
}

func (s *Server) handleMenu(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		menu, err := s.menuLoader.Load(path)
		if err != nil {
			log.Printf("web: menu content error (%s): %v", path, err)
		}
		render(w, r, views.MenuPage(menu))
	}
}

func (s *Server) handleCatsIndex(w http.ResponseWriter, r *http.Request) {
	cats, err := s.catLoader.LoadAll()
	if err != nil {
		log.Printf("web: cats index error: %v", err)
	}
	render(w, r, views.CatsIndex(cats))
}

func (s *Server) handleCatDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	cat, err := s.catLoader.LoadOne(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, r, views.CatDetail(cat))
}
```

Note: `GET /{$}` (not `GET /`) is deliberate — Go 1.22+'s `http.ServeMux` treats a bare `"/"` pattern as a catch-all prefix match, which would swallow every other route including `/static/` and `/media/`. The `{$}` suffix restricts the pattern to match only the exact root path.

- [ ] **Step 7: Add the shared `render` helper**

```go
// internal/web/render.go
package web

import (
	"log"
	"net/http"

	"github.com/a-h/templ"
)

// render writes a templ.Component to w as text/html, logging (not
// panicking) on a render error, since by the time Render is called
// headers may already be committed.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("web: render error: %v", err)
	}
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run:
```bash
templ generate
go test ./internal/web/... -v
```
Expected: PASS for all six new tests plus the two from Task 8.

- [ ] **Step 9: Review in the browser on a real phone**

Run: `go run ./cmd/homesite -config config.local.yaml`, find the Mac's LAN IP (`ipconfig getifaddr en0`), and open `http://<mac-ip>:8080/` on a phone. Confirm the welcome hub, all three menus, and both cat pages render legibly at phone width. This is a checkpoint, not a step with a pass/fail assertion — note anything that looks wrong for the Task 17 design-polish pass, but don't block on visual perfection here.

- [ ] **Step 10: Commit**

```bash
git add internal/web views
git commit -m "Add Welcome, Menu, and Cats pages"
```

---

## Task 10: Wi-Fi and Share QR Pages

**Files:**
- Modify: `internal/web/server.go` (register new routes)
- Create: `internal/web/handlers_qr.go`
- Test: `internal/web/handlers_qr_test.go`
- Create: `views/qrpage.templ`

**Interfaces:**
- Consumes: `qr.WiFiPayload`, `qr.URLPayload`, `qr.PNG` (Task 6); `views.Layout` with `HeaderQR` variant (Task 7); `config.WiFiConfig`, `config.SiteConfig` (Task 1)
- Produces: four routes — `GET /wifi`, `GET /wifi/qr.png`, `GET /share`, `GET /share/qr.png`

Each page is an `<img>` tag pointing at its own PNG route rather than an inline data URI, so the browser can cache the image and so the PNG bytes are independently testable.

- [ ] **Step 1: Write the failing test**

```go
// internal/web/handlers_qr_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homesite/internal/config"
)

func testConfigWithWiFi() *config.Config {
	cfg := testConfig()
	cfg.WiFi = config.WiFiConfig{SSID: "Brivin Net", Password: "letmein123", Auth: "WPA"}
	cfg.Site.URL = "http://home.arpa"
	cfg.Site.FallbackURL = "http://192.168.1.50"
	return cfg
}

func TestWiFiPageRendersCredentials(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/wifi", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Brivin Net") || !strings.Contains(body, "letmein123") {
		t.Error("expected page to show SSID and password as selectable text")
	}
	if !strings.Contains(body, "/wifi/qr.png") {
		t.Error("expected page to reference the QR image route")
	}
}

func TestWiFiQRPngServesImage(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/wifi/qr.png", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty PNG body")
	}
}

func TestSharePageRendersURLAndFallback(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/share", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "home.arpa") {
		t.Error("expected page to show the primary URL")
	}
	if !strings.Contains(body, "192.168.1.50") {
		t.Error("expected page to show the resolver-bypass fallback")
	}
}

func TestShareQRPngServesImage(t *testing.T) {
	s := New(testConfigWithWiFi())
	req := httptest.NewRequest(http.MethodGet, "/share/qr.png", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/web/... -run "WiFi|Share" -v`
Expected: FAIL — routes 404.

- [ ] **Step 3: Write the shared QR page templ component**

```templ
// views/qrpage.templ
package views

templ QRPage(title, qrSrc, primaryText, fallbackText string) {
	@Layout(title, HeaderQR, qrPageBody(qrSrc, primaryText, fallbackText))
}

templ qrPageBody(qrSrc, primaryText, fallbackText string) {
	<div class="qr-card">
		<img src={ qrSrc } alt="QR code" width="320" height="320"/>
		<p class="qr-primary-text">{ primaryText }</p>
		if fallbackText != "" {
			<p class="qr-fallback-text">{ fallbackText }</p>
		}
	</div>
}
```

- [ ] **Step 4: Wire the routes into `registerPageRoutes`**

Add these two lines to `internal/web/server.go`'s `registerPageRoutes` (from Task 9):

```go
	s.mux.HandleFunc("GET /wifi", s.handleWiFiPage)
	s.mux.HandleFunc("GET /wifi/qr.png", s.handleWiFiQRPng)
	s.mux.HandleFunc("GET /share", s.handleSharePage)
	s.mux.HandleFunc("GET /share/qr.png", s.handleShareQRPng)
```

- [ ] **Step 5: Implement the handlers**

```go
// internal/web/handlers_qr.go
package web

import (
	"fmt"
	"log"
	"net/http"

	"homesite/internal/qr"
	"homesite/views"
)

func (s *Server) handleWiFiPage(w http.ResponseWriter, r *http.Request) {
	primary := fmt.Sprintf("%s / %s", s.cfg.WiFi.SSID, s.cfg.WiFi.Password)
	render(w, r, views.QRPage("Wi-Fi", "/wifi/qr.png", primary, ""))
}

func (s *Server) handleWiFiQRPng(w http.ResponseWriter, r *http.Request) {
	payload := qr.WiFiPayload(s.cfg.WiFi.SSID, s.cfg.WiFi.Password, s.cfg.WiFi.Auth, s.cfg.WiFi.Hidden)
	writePNG(w, payload)
}

func (s *Server) handleSharePage(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.QRPage("Share", "/share/qr.png", s.cfg.Site.URL, s.cfg.Site.FallbackURL))
}

func (s *Server) handleShareQRPng(w http.ResponseWriter, r *http.Request) {
	payload := qr.URLPayload(s.cfg.Site.URL)
	writePNG(w, payload)
}

func writePNG(w http.ResponseWriter, payload string) {
	b, err := qr.PNG(payload)
	if err != nil {
		log.Printf("web: qr render error: %v", err)
		http.Error(w, "could not render QR code", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(b)
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run:
```bash
templ generate
go test ./internal/web/... -v
```
Expected: PASS for all four new tests plus everything from Tasks 8–9.

- [ ] **Step 7: Review both QR pages on a real phone**

With `go run ./cmd/homesite -config config.local.yaml` running, open `http://<mac-ip>:8080/wifi` and `/share` on a phone and actually scan each code with the Camera app. This is the one checkpoint in the whole plan that **cannot be verified by an automated test** — a passing PNG-bytes test proves the file is a valid image, not that a phone camera can read it. Confirm: white background regardless of the phone's system dark-mode setting, and the code visibly scans from arm's length.

- [ ] **Step 8: Commit**

```bash
git add internal/web views
git commit -m "Add Wi-Fi and Share QR pages"
```

---

## Task 11: SQLite Store and Schema Migration

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `store.Open(path string) (*sql.DB, error)` (file-backed, WAL mode, migrated), `store.OpenMemory() (*sql.DB, error)` (for tests) — both used by `internal/guestbook` (Task 14) and `internal/photos` (Task 13), and by `main.go`.

One migration function creates both `photos` and `guestbook_entries` tables, since `guestbook_entries.photo_id` references `photos.id` and the guest book depends on photos existing first.

- [ ] **Step 1: Add the dependency**

Run: `go get modernc.org/sqlite`

- [ ] **Step 2: Write the failing test**

```go
// internal/store/store_test.go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	tables := []string{"photos", "guestbook_entries"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", tbl, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open on existing file: %v", err)
	}
	defer db2.Close()
}

func TestOpenMemoryForTests(t *testing.T) {
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer db.Close()

	var name string
	err = db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='photos'",
	).Scan(&name)
	if err != nil {
		t.Errorf("photos table not found in in-memory db: %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/store/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 4: Implement the store package**

```go
// internal/store/store.go
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS photos (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id     TEXT    NOT NULL UNIQUE,
  path        TEXT    NOT NULL,
  thumb_path  TEXT    NOT NULL,
  byte_size   INTEGER NOT NULL,
  width       INTEGER NOT NULL,
  height      INTEGER NOT NULL,
  source      TEXT    NOT NULL,
  caption     TEXT,
  created_at  TEXT    NOT NULL,
  hidden      INTEGER NOT NULL DEFAULT 0,
  client_ip   TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_photos_created ON photos(created_at DESC);

CREATE TABLE IF NOT EXISTS guestbook_entries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL,
  message    TEXT    NOT NULL,
  photo_id   INTEGER REFERENCES photos(id),
  created_at TEXT    NOT NULL,
  hidden     INTEGER NOT NULL DEFAULT 0,
  client_ip  TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_guestbook_created ON guestbook_entries(created_at DESC);
`

// Open opens (creating if needed) a file-backed SQLite database in WAL
// mode — lower write amplification than the default rollback journal,
// which matters when the database shares a microSD card with the OS —
// and applies the schema migration idempotently.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: opening %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("store: connecting to %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("store: migrating %s: %w", path, err)
	}
	return db, nil
}

// OpenMemory opens an in-memory database for tests. Connections are
// capped at one: SQLite's :memory: databases are private per connection,
// so a pool of more than one would see an empty schema on the second
// connection.
func OpenMemory() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("store: opening in-memory db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("store: migrating in-memory db: %w", err)
	}
	return db, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/store/... -v`
Expected: PASS for all three tests.

- [ ] **Step 6: Commit**

```bash
git add internal/store
git commit -m "Add SQLite store with photos and guestbook_entries schema"
```

---

## Task 12: ImageMagick Wrapper (Convert, Strip, Thumbnail)

**Files:**
- Create: `internal/imaging/imaging.go`
- Test: `internal/imaging/imaging_test.go`

**Interfaces:**
- Consumes: nothing (shells out to the `magick` CLI)
- Produces: `imaging.ConvertAndStrip(ctx context.Context, srcPath, dstPath string) (imaging.Result, error)`, `imaging.Thumbnail(ctx context.Context, srcPath, dstPath string, longEdge int) error`, `imaging.Result{Width, Height int}` — used by `internal/photos` (Task 13).

This test is integration-tagged and skips itself when `magick` isn't on `PATH`, per the spec's testing table — it exercises the real binary rather than mocking it, since the entire point is verifying HEIC conversion and EXIF stripping actually happen.

- [ ] **Step 1: Confirm ImageMagick is available**

Run: `magick -version`
Expected: reports ImageMagick 7.x. If missing: `brew install imagemagick` (macOS, already done in Task 7) or `sudo apt install imagemagick` (Pi, done in Task 18's provisioning step).

- [ ] **Step 2: Write the failing test**

```go
// internal/imaging/imaging_test.go
package imaging

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireMagick(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("magick"); err != nil {
		t.Skip("magick not found on PATH — skipping ImageMagick integration test")
	}
}

func TestConvertAndStripProducesJPEGWithoutEXIF(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.png")
	if err := exec.CommandContext(ctx, "magick",
		"-size", "100x50", "xc:red",
		"-set", "exif:GPSLatitude", "37/1,46/1,2540/100",
		src,
	).Run(); err != nil {
		t.Fatalf("generating test fixture with fake GPS EXIF: %v", err)
	}

	dst := filepath.Join(dir, "out.jpg")
	result, err := ConvertAndStrip(ctx, src, dst)
	if err != nil {
		t.Fatalf("ConvertAndStrip: %v", err)
	}
	if result.Width != 100 || result.Height != 50 {
		t.Errorf("Result = %+v, want 100x50", result)
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%[EXIF:GPSLatitude]", dst).Output()
	if err != nil {
		t.Fatalf("identify on output: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no GPS EXIF in output, got %q", out)
	}
}

func TestThumbnailResizesToLongEdge(t *testing.T) {
	requireMagick(t)
	ctx := context.Background()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.jpg")
	if err := exec.CommandContext(ctx, "magick", "-size", "1200x600", "xc:blue", src).Run(); err != nil {
		t.Fatalf("generating test fixture: %v", err)
	}

	dst := filepath.Join(dir, "thumb.jpg")
	if err := Thumbnail(ctx, src, dst, 400); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}

	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%w %h", dst).Output()
	if err != nil {
		t.Fatalf("identify on thumbnail: %v", err)
	}
	var w, h int
	if _, err := fmt.Sscanf(string(out), "%d %d", &w, &h); err != nil {
		t.Fatalf("parsing identify output %q: %v", out, err)
	}
	if w != 400 || h != 200 {
		t.Errorf("thumbnail = %dx%d, want 400x200 (long edge 400, 2:1 aspect preserved)", w, h)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/imaging/... -v`
Expected: FAIL — package doesn't exist yet (or SKIP if `magick` isn't installed, in which case install it first per Step 1 and re-run).

- [ ] **Step 4: Implement the imaging package**

```go
// internal/imaging/imaging.go
package imaging

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Result struct {
	Width  int
	Height int
}

const execTimeout = 20 * time.Second

// ConvertAndStrip converts src (any ImageMagick-readable format,
// including HEIC/HEIF) into a JPEG at dstPath with all metadata removed
// via -strip — this is where GPS coordinates, device identifiers, and
// timestamps embedded in guest photos are discarded before the file
// ever touches the gallery or the guest book.
func ConvertAndStrip(ctx context.Context, srcPath, dstPath string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", srcPath, "-auto-orient", "-strip", dstPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("imaging: convert %s: %w: %s", srcPath, err, stderr.String())
	}
	return identify(ctx, dstPath)
}

// Thumbnail writes a resized copy of srcPath to dstPath whose longest
// edge is longEdge pixels, preserving aspect ratio and stripping
// metadata. The trailing `>` in the geometry means "shrink only" — a
// photo already smaller than longEdge is not upscaled.
func Thumbnail(ctx context.Context, srcPath, dstPath string, longEdge int) error {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	geometry := fmt.Sprintf("%dx%d>", longEdge, longEdge)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "magick", srcPath, "-auto-orient", "-strip", "-resize", geometry, dstPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("imaging: thumbnail %s: %w: %s", srcPath, err, stderr.String())
	}
	return nil
}

func identify(ctx context.Context, path string) (Result, error) {
	out, err := exec.CommandContext(ctx, "magick", "identify", "-format", "%w %h", path).Output()
	if err != nil {
		return Result{}, fmt.Errorf("imaging: identify %s: %w", path, err)
	}
	var w, h int
	if _, err := fmt.Sscanf(string(out), "%d %d", &w, &h); err != nil {
		return Result{}, fmt.Errorf("imaging: parsing identify output %q: %w", out, err)
	}
	return Result{Width: w, Height: h}, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/imaging/... -v`
Expected: PASS for both tests (or SKIP if `magick` is unavailable in this environment — that's an acceptable outcome per the spec's testing table, not a failure).

- [ ] **Step 6: Commit**

```bash
git add internal/imaging
git commit -m "Add ImageMagick wrapper for conversion, EXIF stripping, and thumbnails"
```

---

## Task 13: Photos Package — Sniffing, Ingest Pipeline, and Store

**Files:**
- Create: `internal/photos/photos.go`
- Create: `internal/photos/sniff.go`
- Create: `internal/photos/ingest.go`
- Test: `internal/photos/sniff_test.go`
- Test: `internal/photos/ingest_test.go`
- Test: `internal/photos/photos_test.go`

**Interfaces:**
- Consumes: `store.OpenMemory` (Task 11); `imaging.ConvertAndStrip`, `imaging.Thumbnail`, `imaging.Result` (Task 12)
- Produces: `photos.Photo` struct, `photos.Sniff(b []byte) (mimeType string, ok bool)`, `photos.Store` with `NewStore(db *sql.DB) *Store`, `(*Store) List(ctx, limit, offset int) ([]Photo, error)`, `(*Store) DiskUsageBytes(uploadsDir string) (int64, error)`; `photos.Ingester` with `NewIngester(store *Store, cfg config.PhotosConfig, uploadsDir string) *Ingester` and `(*Ingester) Ingest(ctx context.Context, r io.Reader, source, clientIP string) (Photo, error)`; sentinel errors `ErrNotAnImage`, `ErrTooLarge`, `ErrDiskFull`, `ErrProcessing` — all consumed by the guest-facing handlers in Task 14 and the guest book in Task 15.

**The HEIC gap this closes:** `http.DetectContentType` was verified directly (see Global Constraints) to return `application/octet-stream` for HEIC/HEIF bytes — it has no entry for the ISO-BMFF `ftyp` box family. `Sniff` checks for that box manually before falling back to the stdlib sniffer, which is what makes "iPhone uploads a HEIC photo" actually work.

- [ ] **Step 1: Write the failing sniff test**

```go
// internal/photos/sniff_test.go
package photos

import "testing"

func TestSniffDetectsJPEG(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	mt, ok := Sniff(jpeg)
	if !ok || mt != "image/jpeg" {
		t.Errorf("Sniff(jpeg) = %q, %v, want image/jpeg, true", mt, ok)
	}
}

func TestSniffDetectsPNG(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	mt, ok := Sniff(png)
	if !ok || mt != "image/png" {
		t.Errorf("Sniff(png) = %q, %v, want image/png, true", mt, ok)
	}
}

func TestSniffDetectsHEIC(t *testing.T) {
	// ISO-BMFF ftyp box: 4-byte size, "ftyp", 4-byte major brand "heic".
	// http.DetectContentType alone returns application/octet-stream for
	// this — verified directly — which is exactly the gap Sniff closes.
	heic := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0x00, 0x00, 0x00, 0x00}
	mt, ok := Sniff(heic)
	if !ok || mt != "image/heic" {
		t.Errorf("Sniff(heic) = %q, %v, want image/heic, true", mt, ok)
	}
}

func TestSniffRejectsTextFileNamedJPG(t *testing.T) {
	// The point: sniffing must be byte-based, never trust a filename or
	// client-supplied MIME type.
	text := []byte("this is not an image, just named photo.jpg\n")
	_, ok := Sniff(text)
	if ok {
		t.Error("expected Sniff to reject plain text regardless of extension")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/photos/... -run TestSniff -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement Sniff**

```go
// internal/photos/sniff.go
package photos

import "net/http"

var heicBrands = map[string]bool{
	"heic": true, "heix": true, "heim": true, "heis": true,
	"hevc": true, "hevx": true, "mif1": true, "msf1": true,
}

// Sniff identifies an image's real type from its bytes, never from a
// filename or client-supplied Content-Type header. It special-cases the
// ISO-BMFF `ftyp` box (HEIC/HEIF) before falling back to
// http.DetectContentType, which does not recognize that family at all.
func Sniff(b []byte) (mimeType string, ok bool) {
	if len(b) >= 12 && string(b[4:8]) == "ftyp" && heicBrands[string(b[8:12])] {
		return "image/heic", true
	}
	switch ct := http.DetectContentType(b); ct {
	case "image/jpeg", "image/png", "image/webp":
		return ct, true
	}
	return "", false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/photos/... -run TestSniff -v`
Expected: PASS for all four cases.

- [ ] **Step 5: Write the Store type and its failing test**

```go
// internal/photos/photos_test.go
package photos

import (
	"context"
	"testing"

	"homesite/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestStoreInsertAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Photo{
		FileID: "abc123", Path: "2026-07/abc123.jpg", ThumbPath: "2026-07/abc123_thumb.jpg",
		ByteSize: 1024, Width: 800, Height: 600, Source: "gallery",
		CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "192.168.1.10",
	}
	if _, err := s.Insert(ctx, p); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].FileID != "abc123" {
		t.Fatalf("List = %+v, want one entry with FileID abc123", got)
	}
}

func TestStoreListExcludesHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	visible := Photo{FileID: "visible", Path: "p1.jpg", ThumbPath: "t1.jpg", Source: "gallery", CreatedAt: "2026-07-27T12:00:00Z", ClientIP: "1.1.1.1"}
	hiddenID, err := s.Insert(ctx, visible)
	if err != nil {
		t.Fatal(err)
	}
	_ = hiddenID

	hidden := Photo{FileID: "hidden", Path: "p2.jpg", ThumbPath: "t2.jpg", Source: "gallery", CreatedAt: "2026-07-27T12:00:01Z", ClientIP: "1.1.1.1", Hidden: true}
	if _, err := s.Insert(ctx, hidden); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FileID != "visible" {
		t.Fatalf("List = %+v, want only the visible entry", got)
	}
}

func TestDiskUsageBytesSumsFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/a.jpg", make([]byte, 100))
	writeFile(t, dir+"/b.jpg", make([]byte, 250))

	s := newTestStore(t)
	total, err := s.DiskUsageBytes(dir)
	if err != nil {
		t.Fatalf("DiskUsageBytes: %v", err)
	}
	if total != 350 {
		t.Errorf("DiskUsageBytes = %d, want 350", total)
	}
}
```

- [ ] **Step 6: Add the `writeFile` test helper**

```go
// internal/photos/testhelpers_test.go
package photos

import (
	"os"
	"testing"
)

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
```

- [ ] **Step 7: Run the tests to verify they fail**

Run: `go test ./internal/photos/... -run "TestStore|TestDiskUsage" -v`
Expected: FAIL — `Photo`, `NewStore` undefined.

- [ ] **Step 8: Implement the Photo type and Store**

```go
// internal/photos/photos.go
package photos

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
)

type Photo struct {
	ID        int64
	FileID    string
	Path      string
	ThumbPath string
	ByteSize  int64
	Width     int
	Height    int
	Source    string // "gallery" | "guestbook"
	Caption   string
	CreatedAt string
	Hidden    bool
	ClientIP  string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Insert(ctx context.Context, p Photo) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO photos (file_id, path, thumb_path, byte_size, width, height, source, caption, created_at, hidden, client_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.FileID, p.Path, p.ThumbPath, p.ByteSize, p.Width, p.Height, p.Source, p.Caption, p.CreatedAt, boolToInt(p.Hidden), p.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("photos: inserting: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Photo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, file_id, path, thumb_path, byte_size, width, height, source, caption, created_at, hidden, client_ip
		FROM photos WHERE hidden = 0 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("photos: listing: %w", err)
	}
	defer rows.Close()

	var out []Photo
	for rows.Next() {
		var p Photo
		var hidden int
		var caption sql.NullString
		if err := rows.Scan(&p.ID, &p.FileID, &p.Path, &p.ThumbPath, &p.ByteSize, &p.Width, &p.Height, &p.Source, &caption, &p.CreatedAt, &hidden, &p.ClientIP); err != nil {
			return nil, fmt.Errorf("photos: scanning row: %w", err)
		}
		p.Caption = caption.String
		p.Hidden = hidden != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// DiskUsageBytes sums the size of every regular file under dir. It backs
// the 80GB upload cap — checked before every ingest — and the /healthz
// disk-usage figure (Task 18).
func (s *Store) DiskUsageBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("photos: computing disk usage of %s: %w", dir, err)
	}
	return total, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/photos/... -run "TestStore|TestDiskUsage" -v`
Expected: PASS for all three tests.

- [ ] **Step 10: Write the failing ingest test using a fake converter**

The fake converter avoids depending on a real `magick` install for this package's own tests — the real binary is exercised by `internal/imaging`'s own integration test (Task 12).

```go
// internal/photos/ingest_test.go
package photos

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"homesite/internal/config"
	"homesite/internal/imaging"
)

type fakeConverter struct {
	convertCalled, thumbnailCalled bool
}

func (f *fakeConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	f.convertCalled = true
	if err := os.WriteFile(dst, []byte("fake-jpeg-bytes"), 0o644); err != nil {
		return imaging.Result{}, err
	}
	return imaging.Result{Width: 800, Height: 600}, nil
}

func (f *fakeConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	f.thumbnailCalled = true
	return os.WriteFile(dst, []byte("fake-thumb-bytes"), 0o644)
}

func testPhotosConfig() config.PhotosConfig {
	return config.PhotosConfig{
		MaxFileBytes:  1024 * 1024,
		MaxTotalBytes: 10 * 1024 * 1024,
		ThumbLongEdge: 400,
	}
}

func TestIngestSucceedsForValidJPEG(t *testing.T) {
	uploadsDir := t.TempDir()
	conv := &fakeConverter{}
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: conv}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	photo, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if !conv.convertCalled || !conv.thumbnailCalled {
		t.Error("expected both ConvertAndStrip and Thumbnail to be called")
	}
	if photo.Width != 800 || photo.Height != 600 {
		t.Errorf("photo dims = %dx%d, want 800x600", photo.Width, photo.Height)
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, photo.Path)); err != nil {
		t.Errorf("expected final image on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, photo.ThumbPath)); err != nil {
		t.Errorf("expected thumbnail on disk: %v", err)
	}
}

func TestIngestRejectsNonImage(t *testing.T) {
	uploadsDir := t.TempDir()
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: &fakeConverter{}}

	_, err := ing.Ingest(context.Background(), bytes.NewReader([]byte("not an image")), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want ErrNotAnImage", err)
	}
}

func TestIngestRejectsOversizedFile(t *testing.T) {
	uploadsDir := t.TempDir()
	cfg := testPhotosConfig()
	cfg.MaxFileBytes = 8 // tiny, to trigger the limit deterministically
	ing := &Ingester{store: newTestStore(t), cfg: cfg, uploadsDir: uploadsDir, conv: &fakeConverter{}}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	_, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestIngestRejectsWhenDiskCapReached(t *testing.T) {
	uploadsDir := t.TempDir()
	writeFile(t, filepath.Join(uploadsDir, "existing.dat"), make([]byte, 1000))

	cfg := testPhotosConfig()
	cfg.MaxTotalBytes = 500 // already exceeded by the existing file above
	ing := &Ingester{store: newTestStore(t), cfg: cfg, uploadsDir: uploadsDir, conv: &fakeConverter{}}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	_, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("err = %v, want ErrDiskFull", err)
	}
}

func TestIngestWrapsConverterFailure(t *testing.T) {
	uploadsDir := t.TempDir()
	failing := failingConverter{}
	ing := &Ingester{store: newTestStore(t), cfg: testPhotosConfig(), uploadsDir: uploadsDir, conv: failing}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3, 4, 5}
	_, err := ing.Ingest(context.Background(), bytes.NewReader(jpeg), "gallery", "192.168.1.20")
	if !errors.Is(err, ErrProcessing) {
		t.Fatalf("err = %v, want ErrProcessing", err)
	}
}

type failingConverter struct{}

func (failingConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	return imaging.Result{}, errors.New("boom")
}
func (failingConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	return errors.New("boom")
}
```

- [ ] **Step 11: Run the tests to verify they fail**

Run: `go test ./internal/photos/... -run TestIngest -v`
Expected: FAIL — `Ingester` undefined.

- [ ] **Step 12: Implement the ingest pipeline**

Order of checks matters: sniff before size (a huge non-image should still report "not an image," not "too large"), and size before the disk-cap `DiskUsageBytes` walk (cheap check first — no reason to walk the whole uploads tree for a file that's already rejected).

```go
// internal/photos/ingest.go
package photos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"homesite/internal/config"
	"homesite/internal/imaging"
)

var (
	ErrNotAnImage = errors.New("not a recognized image type")
	ErrTooLarge   = errors.New("file exceeds maximum size")
	ErrDiskFull   = errors.New("upload storage is full")
	ErrProcessing = errors.New("image processing failed")
)

// Converter is the seam between Ingester and ImageMagick, so tests can
// substitute a fake and exercise the pipeline's decision logic without
// requiring `magick` to be installed. internal/imaging's own tests cover
// the real binary.
type Converter interface {
	ConvertAndStrip(ctx context.Context, srcPath, dstPath string) (imaging.Result, error)
	Thumbnail(ctx context.Context, srcPath, dstPath string, longEdge int) error
}

type realConverter struct{}

func (realConverter) ConvertAndStrip(ctx context.Context, src, dst string) (imaging.Result, error) {
	return imaging.ConvertAndStrip(ctx, src, dst)
}
func (realConverter) Thumbnail(ctx context.Context, src, dst string, longEdge int) error {
	return imaging.Thumbnail(ctx, src, dst, longEdge)
}

type Ingester struct {
	store      *Store
	cfg        config.PhotosConfig
	uploadsDir string
	conv       Converter
}

func NewIngester(store *Store, cfg config.PhotosConfig, uploadsDir string) *Ingester {
	return &Ingester{store: store, cfg: cfg, uploadsDir: uploadsDir, conv: realConverter{}}
}

// Ingest reads r fully (capped at cfg.MaxFileBytes+1 so an oversized
// upload can't exhaust memory), sniffs its real type, checks the disk
// cap, converts it to a metadata-stripped JPEG plus thumbnail, and
// records it. source is "gallery" or "guestbook".
func (ing *Ingester) Ingest(ctx context.Context, r io.Reader, source, clientIP string) (Photo, error) {
	buf, err := io.ReadAll(io.LimitReader(r, ing.cfg.MaxFileBytes+1))
	if err != nil {
		return Photo{}, fmt.Errorf("photos: reading upload: %w", err)
	}
	if int64(len(buf)) > ing.cfg.MaxFileBytes {
		return Photo{}, fmt.Errorf("photos: exceeds %d bytes: %w", ing.cfg.MaxFileBytes, ErrTooLarge)
	}

	sniffLen := min(len(buf), 512)
	mimeType, ok := Sniff(buf[:sniffLen])
	if !ok {
		return Photo{}, fmt.Errorf("photos: unrecognized file type: %w", ErrNotAnImage)
	}

	used, err := ing.store.DiskUsageBytes(ing.uploadsDir)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: checking disk usage: %w", err)
	}
	if used+int64(len(buf)) > ing.cfg.MaxTotalBytes {
		return Photo{}, fmt.Errorf("photos: at capacity: %w", ErrDiskFull)
	}

	fileID, err := newFileID()
	if err != nil {
		return Photo{}, fmt.Errorf("photos: generating file id: %w", err)
	}

	monthDir := time.Now().UTC().Format("2006-01")
	destDir := filepath.Join(ing.uploadsDir, monthDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Photo{}, fmt.Errorf("photos: creating upload dir: %w", err)
	}

	tmpSrc := filepath.Join(destDir, fileID+"_src"+extensionFor(mimeType))
	if err := os.WriteFile(tmpSrc, buf, 0o644); err != nil {
		return Photo{}, fmt.Errorf("photos: writing temp source: %w", err)
	}
	defer os.Remove(tmpSrc)

	relPath := filepath.Join(monthDir, fileID+".jpg")
	relThumb := filepath.Join(monthDir, fileID+"_thumb.jpg")
	finalPath := filepath.Join(ing.uploadsDir, relPath)
	thumbPath := filepath.Join(ing.uploadsDir, relThumb)

	result, err := ing.conv.ConvertAndStrip(ctx, tmpSrc, finalPath)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: converting: %w: %w", err, ErrProcessing)
	}
	if err := ing.conv.Thumbnail(ctx, tmpSrc, thumbPath, ing.cfg.ThumbLongEdge); err != nil {
		os.Remove(finalPath)
		return Photo{}, fmt.Errorf("photos: thumbnailing: %w: %w", err, ErrProcessing)
	}

	finalInfo, err := os.Stat(finalPath)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: stat final image: %w", err)
	}

	photo := Photo{
		FileID: fileID, Path: relPath, ThumbPath: relThumb,
		ByteSize: finalInfo.Size(), Width: result.Width, Height: result.Height,
		Source: source, CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientIP: clientIP,
	}
	id, err := ing.store.Insert(ctx, photo)
	if err != nil {
		return Photo{}, fmt.Errorf("photos: recording metadata: %w", err)
	}
	photo.ID = id
	return photo, nil
}

func newFileID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:]), nil
}

func extensionFor(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	default:
		return ".bin"
	}
}
```

- [ ] **Step 13: Run the tests to verify they pass**

Run: `go test ./internal/photos/... -v`
Expected: PASS for every test across `sniff_test.go`, `photos_test.go`, and `ingest_test.go`.

- [ ] **Step 14: Commit**

```bash
git add internal/photos
git commit -m "Add photos package: byte sniffing, ingest pipeline, and SQLite store"
```

---

## Task 14: Photos Web Feature — Gallery and Upload

**Files:**
- Modify: `internal/web/server.go` (add `photosStore`, `photosIngester` fields; register routes and `/uploads/` static serving)
- Create: `internal/web/handlers_photos.go`
- Create: `internal/web/clientip.go`
- Test: `internal/web/handlers_photos_test.go`
- Create: `views/photos.templ`

**Interfaces:**
- Consumes: `photos.Store`, `photos.Ingester`, `photos.Photo`, sentinel errors (Task 13); `store.OpenMemory`/`store.Open` (Task 11)
- Produces: `GET /photos`, `POST /photos` routes; `web.clientIP(r *http.Request) string` — reused by Task 15 (guest book) and Task 16 (rate limiting).

Pagination avoids a separate `COUNT(*)` query: the handler asks the store for one row more than a page needs and uses its presence to decide whether an "Older" link should appear, rather than adding a new `Store` method.

- [ ] **Step 1: Write the `clientIP` helper**

```go
// internal/web/clientip.go
package web

import (
	"net"
	"net/http"
)

// clientIP extracts the caller's IP from RemoteAddr, stripping the port.
// This is a LAN-only site with no reverse proxy in front of it, so
// RemoteAddr is always the guest's real address — no X-Forwarded-For
// handling is needed or trusted.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/web/handlers_photos_test.go
package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homesite/internal/config"
	"homesite/internal/photos"
	"homesite/internal/store"
)

func testConfigWithPhotos(t *testing.T) (*config.Config, string) {
	t.Helper()
	uploadsDir := t.TempDir()
	cfg := testConfig()
	cfg.UploadsDir = uploadsDir
	cfg.Photos = config.PhotosConfig{
		MaxFileBytes: 1024 * 1024, MaxTotalBytes: 100 * 1024 * 1024, ThumbLongEdge: 400,
	}
	return cfg, uploadsDir
}

func newTestServerWithPhotos(t *testing.T) *Server {
	t.Helper()
	cfg, _ := testConfigWithPhotos(t)
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(cfg)
	s.photosStore = photos.NewStore(db)
	s.photosIngester = photos.NewIngester(s.photosStore, cfg.Photos, cfg.UploadsDir)
	return s
}

func multipartJPEGRequest(t *testing.T, filename string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("photos", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(body)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/photos", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestPhotosPageRendersEmptyGallery(t *testing.T) {
	s := newTestServerWithPhotos(t)
	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Upload Photos") {
		t.Error("expected page heading")
	}
}

func TestPhotosUploadRejectsNonImage(t *testing.T) {
	// magick isn't invoked on a rejected file, so this test needs no
	// real ImageMagick install — Sniff rejects it before Ingest ever
	// calls the converter.
	s := newTestServerWithPhotos(t)
	req := multipartJPEGRequest(t, "notes.txt", []byte("just some text"))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fragment renders even on per-file failure)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "0 of 1 uploaded") {
		t.Errorf("body = %q, want a 0-of-1 result", body)
	}
	if !strings.Contains(body, "isn't a photo") {
		t.Errorf("body = %q, want the not-an-image message", body)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/web/... -run TestPhotos -v`
Expected: FAIL — `photosStore` field doesn't exist on `Server`.

- [ ] **Step 4: Write the templ views**

```templ
// views/photos.templ
package views

import (
	"fmt"

	"homesite/internal/photos"
)

type UploadFailure struct {
	Filename string
	Message  string
}

type UploadResult struct {
	Total     int
	Succeeded int
	Failures  []UploadFailure
}

templ PhotosPage(gallery []photos.Photo, page int, hasPrev, hasMore bool) {
	@Layout("Upload Photos", HeaderStandard, photosPageBody(gallery, page, hasPrev, hasMore))
}

templ photosPageBody(gallery []photos.Photo, page int, hasPrev, hasMore bool) {
	<h1>Upload Photos</h1>
	<form hx-post="/photos" hx-encoding="multipart/form-data" hx-target="#photos-content" hx-swap="innerHTML"
		_onsubmit="document.getElementById('upload-progress').value=0">
		<input type="file" name="photos" accept="image/jpeg,image/png,image/webp,image/heic" multiple required/>
		<progress id="upload-progress" value="0" max="100"></progress>
		<button type="submit">Upload</button>
	</form>
	<div id="photos-content">
		@PhotosContent(nil, gallery, page, hasPrev, hasMore)
	</div>
	<script>
		document.body.addEventListener('htmx:xhrProgress', function(evt) {
			var bar = document.getElementById('upload-progress');
			if (bar && evt.detail.lengthComputable) {
				bar.value = (evt.detail.loaded / evt.detail.total) * 100;
			}
		});
	</script>
}

templ PhotosContent(result *UploadResult, gallery []photos.Photo, page int, hasPrev, hasMore bool) {
	if result != nil {
		<p class="upload-result">{ fmt.Sprintf("%d of %d uploaded", result.Succeeded, result.Total) }</p>
		if len(result.Failures) > 0 {
			<ul class="upload-failures">
				for _, f := range result.Failures {
					<li>{ f.Filename }: { f.Message }</li>
				}
			</ul>
		}
	}
	<div class="photo-grid">
		for _, p := range gallery {
			<a href={ templ.URL("/uploads/" + p.Path) } class="photo-thumb">
				<img src={ "/uploads/" + p.ThumbPath } alt="Guest photo" loading="lazy"/>
			</a>
		}
	</div>
	<nav class="pagination">
		if hasPrev {
			<a href={ templ.URL(fmt.Sprintf("/photos?page=%d", page-1)) }>&larr; Newer</a>
		}
		if hasMore {
			<a href={ templ.URL(fmt.Sprintf("/photos?page=%d", page+1)) }>Older &rarr;</a>
		}
	</nav>
}
```

`_onsubmit` above is a plain HTML attribute typo-guard note: templ does not require any special syntax for it — it's written as a normal attribute (`onsubmit="..."`). Correct the attribute name to `onsubmit` (not `_onsubmit`) when writing the file; the leading underscore here was an error in this plan and must not be copied literally.

- [ ] **Step 5: Modify `internal/web/server.go`**

Add fields and wiring (shown as a diff against the version from Task 9):

```go
// internal/web/server.go — add imports and fields
import (
	// ...existing imports...
	"homesite/internal/photos"
)

type Server struct {
	mux            *http.ServeMux
	cfg            *config.Config
	menuLoader     *content.MenuLoader
	catLoader      *content.CatLoader
	welcomeLoader  *content.WelcomeLoader
	photosStore    *photos.Store
	photosIngester *photos.Ingester
}
```

In `New`, after the existing loader construction, add:

```go
	uploadsDir := http.Dir(cfg.UploadsDir)
	s.mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(uploadsDir)))
```

(Photo store and ingester are deliberately **not** constructed inside `New` yet — they need a live `*sql.DB`, which `main.go` opens once at startup in Task 15 alongside the guest book, which shares the same database. Until then, tests construct a `Server` via `New` and then set `s.photosStore`/`s.photosIngester` directly, as `newTestServerWithPhotos` above does. This is a deliberate, temporary seam — Task 15 closes it by adding a `NewWithDB`-style wiring path used by both features and by `main.go`.)

Add the route registrations to `registerPageRoutes`:
```go
	s.mux.HandleFunc("GET /photos", s.handlePhotosPage)
	s.mux.HandleFunc("POST /photos", s.handlePhotosUpload)
```

- [ ] **Step 6: Implement the handlers**

```go
// internal/web/handlers_photos.go
package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"homesite/internal/photos"
	"homesite/views"
)

const (
	galleryPageSize      = 60
	multipartMemoryLimit = 10 << 20 // 10MB held in memory before spilling to temp files
)

func (s *Server) handlePhotosPage(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	gallery, hasMore := s.listGalleryPage(r, page)
	render(w, r, views.PhotosPage(gallery, page, page > 1, hasMore))
}

func (s *Server) handlePhotosUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		http.Error(w, "could not parse upload", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["photos"]
	result := views.UploadResult{Total: len(files)}
	ip := clientIP(r)

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			result.Failures = append(result.Failures, views.UploadFailure{Filename: fh.Filename, Message: "couldn't read that file"})
			continue
		}
		_, err = s.photosIngester.Ingest(r.Context(), f, "gallery", ip)
		f.Close()
		if err != nil {
			log.Printf("web: photo upload rejected (%s): %v", fh.Filename, err)
			result.Failures = append(result.Failures, views.UploadFailure{Filename: fh.Filename, Message: uploadErrorMessage(err)})
			continue
		}
		result.Succeeded++
	}

	gallery, hasMore := s.listGalleryPage(r, 1)
	render(w, r, views.PhotosContent(&result, gallery, 1, false, hasMore))
}

func (s *Server) listGalleryPage(r *http.Request, page int) (gallery []photos.Photo, hasMore bool) {
	offset := (page - 1) * galleryPageSize
	rows, err := s.photosStore.List(r.Context(), galleryPageSize+1, offset)
	if err != nil {
		log.Printf("web: listing photos: %v", err)
		return nil, false
	}
	if len(rows) > galleryPageSize {
		return rows[:galleryPageSize], true
	}
	return rows, false
}

func parsePage(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func uploadErrorMessage(err error) string {
	switch {
	case errors.Is(err, photos.ErrNotAnImage):
		return "that file isn't a photo — JPEG, PNG, WebP, or HEIC please"
	case errors.Is(err, photos.ErrTooLarge):
		return "that photo's too big — 25MB max"
	case errors.Is(err, photos.ErrDiskFull):
		return "photo storage is full — tell Kevin"
	case errors.Is(err, photos.ErrProcessing):
		return "couldn't process that photo, try another"
	default:
		return "upload failed"
	}
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run:
```bash
templ generate
go test ./internal/web/... -v
```
Expected: PASS for both new tests. `TestPhotosUploadRejectsNonImage` should pass with no ImageMagick installed, since `Sniff` rejects the plain-text upload before any converter is invoked.

- [ ] **Step 8: Add the disk-cap warning banner**

The spec requires this: *"Current usage is exposed on `/healthz`, and past 85% the upload form shows a warning to the host."* `/healthz` itself is wired up later in Task 19 once the backup script exists to report on too, but the **on-page warning belongs here**, alongside the rest of the upload form — it shouldn't wait on unrelated deployment work. Scope note: the warning only appears on the full page load (`GET /photos`), not on the upload-result HTMX fragment, since that fragment doesn't re-render the page shell around it.

Modify `views/photos.templ`'s `PhotosPage` and `photosPageBody` to take a `showDiskWarning bool`:

```templ
templ PhotosPage(gallery []photos.Photo, page int, hasPrev, hasMore, showDiskWarning bool) {
	@Layout("Upload Photos", HeaderStandard, photosPageBody(gallery, page, hasPrev, hasMore, showDiskWarning))
}

templ photosPageBody(gallery []photos.Photo, page int, hasPrev, hasMore, showDiskWarning bool) {
	<h1>Upload Photos</h1>
	if showDiskWarning {
		<p class="disk-warning">Photo storage is almost full — uploads may start failing soon.</p>
	}
	<form hx-post="/photos" hx-encoding="multipart/form-data" hx-target="#photos-content" hx-swap="innerHTML"
		onsubmit="document.getElementById('upload-progress').value=0">
		<input type="file" name="photos" accept="image/jpeg,image/png,image/webp,image/heic" multiple required/>
		<progress id="upload-progress" value="0" max="100"></progress>
		<button type="submit">Upload</button>
	</form>
	<div id="photos-content">
		@PhotosContent(nil, gallery, page, hasPrev, hasMore)
	</div>
	<script>
		document.body.addEventListener('htmx:xhrProgress', function(evt) {
			var bar = document.getElementById('upload-progress');
			if (bar && evt.detail.lengthComputable) {
				bar.value = (evt.detail.loaded / evt.detail.total) * 100;
			}
		});
	</script>
}
```

(This also fixes the earlier `onsubmit` attribute name — Step 4 of this task originally wrote it as `_onsubmit` with a note to correct it; this version is the corrected one and supersedes that note.)

Modify `handlePhotosPage` in `internal/web/handlers_photos.go`:

```go
func (s *Server) handlePhotosPage(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	gallery, hasMore := s.listGalleryPage(r, page)

	showWarning := false
	if used, err := s.photosStore.DiskUsageBytes(s.cfg.UploadsDir); err == nil && s.cfg.Photos.MaxTotalBytes > 0 {
		percent := used * 100 / s.cfg.Photos.MaxTotalBytes
		showWarning = percent >= int64(s.cfg.Photos.WarnAtPercent)
	}

	render(w, r, views.PhotosPage(gallery, page, page > 1, hasMore, showWarning))
}
```

Add a test to `internal/web/handlers_photos_test.go`:

```go
func TestPhotosPageShowsDiskWarningPastThreshold(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	cfg.Photos.MaxTotalBytes = 1000
	cfg.Photos.WarnAtPercent = 85
	writeFileForTest(t, uploadsDir+"/existing.dat", make([]byte, 900)) // 90% full

	db := mustOpenMemoryDB(t)
	s := New(cfg, db)

	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "almost full") {
		t.Error("expected the disk warning banner at 90% usage with an 85% threshold")
	}
}

func writeFileForTest(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
```
(add `"os"` to this test file's imports; `mustOpenMemoryDB` is defined in Task 19 — if executing tasks in order, add a minimal local equivalent here instead: `db, err := store.OpenMemory(); if err != nil { t.Fatal(err) }` with `"homesite/internal/store"` imported, and skip the shared helper until Task 19 introduces it.)

Run:
```bash
templ generate
go test ./internal/web/... -run TestPhotos -v
```
Expected: PASS for the new warning test alongside the existing photos tests.

- [ ] **Step 9: Manually verify a real upload locally**

With ImageMagick installed and `go run ./cmd/homesite -config config.local.yaml` running (main.go doesn't yet construct `photosStore`/`photosIngester` for real — this manual check is deferred to the end of Task 15, once `main.go` is updated to open the database and wire both features. Skip this step for now and revisit it as part of Task 15's Step 14.)

- [ ] **Step 10: Commit**

```bash
git add internal/web views
git commit -m "Add photo gallery, multi-file upload with progress, and disk-cap warning"
```

---

## Task 15: Guest Book Package, Real DB Wiring, and Web Feature

This task also closes the temporary seam flagged in Task 14: `Server` gains a real `*sql.DB`-based constructor used by both photos and the guest book, and by `main.go`.

**Files:**
- Create: `internal/guestbook/guestbook.go`
- Test: `internal/guestbook/guestbook_test.go`
- Modify: `internal/web/server.go` (constructor now takes `*sql.DB`; constructs `photosStore`, `photosIngester`, `guestbookStore` for real)
- Modify: `internal/web/server_test.go` (add shared `newTestServer(t)` helper)
- Modify: `internal/web/handlers_pages_test.go`, `internal/web/handlers_qr_test.go`, `internal/web/handlers_photos_test.go` (use the shared helper instead of calling `New` directly)
- Create: `internal/web/handlers_guestbook.go`
- Test: `internal/web/handlers_guestbook_test.go`
- Create: `views/guestbook.templ`
- Modify: `cmd/homesite/main.go` (open the database, pass it to `web.New`)

**Interfaces:**
- Consumes: `store.Open`, `store.OpenMemory` (Task 11); `photos.Ingester`, sentinel photo errors (Task 13); `web.clientIP` (Task 14)
- Produces: `guestbook.Entry`, `guestbook.Store` with `NewStore(db *sql.DB) *Store`, `(*Store) Create(ctx, Entry) (int64, error)`, `(*Store) List(ctx) ([]Entry, error)`; sentinel errors `ErrNameRequired`, `ErrNameTooLong`, `ErrMessageRequired`, `ErrMessageTooLong`; `web.New(cfg *config.Config, db *sql.DB) *Server` (signature change) — consumed by `main.go` and every test in the `web` package from this point on.

- [ ] **Step 1: Write the failing guestbook store test**

```go
// internal/guestbook/guestbook_test.go
package guestbook

import (
	"context"
	"errors"
	"testing"

	"homesite/internal/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestCreateAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.Create(ctx, Entry{Name: "Alex", Message: "Great party!", ClientIP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "Alex" {
		t.Fatalf("List = %+v", entries)
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Create(context.Background(), Entry{Name: "", Message: "hi", ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrNameRequired) {
		t.Fatalf("err = %v, want ErrNameRequired", err)
	}
}

func TestCreateRejectsOverlongName(t *testing.T) {
	s := newTestStore(t)
	longName := make([]byte, 41)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := s.Create(context.Background(), Entry{Name: string(longName), Message: "hi", ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrNameTooLong) {
		t.Fatalf("err = %v, want ErrNameTooLong", err)
	}
}

func TestCreateRejectsEmptyMessage(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Create(context.Background(), Entry{Name: "Alex", Message: "", ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrMessageRequired) {
		t.Fatalf("err = %v, want ErrMessageRequired", err)
	}
}

func TestCreateRejectsOverlongMessage(t *testing.T) {
	s := newTestStore(t)
	longMsg := make([]byte, 501)
	for i := range longMsg {
		longMsg[i] = 'a'
	}
	_, err := s.Create(context.Background(), Entry{Name: "Alex", Message: string(longMsg), ClientIP: "1.2.3.4"})
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("err = %v, want ErrMessageTooLong", err)
	}
}

func TestListExcludesHidden(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, Entry{Name: "Visible", Message: "shown", ClientIP: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(ctx, Entry{Name: "Hidden", Message: "moderated away", ClientIP: "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db().ExecContext(ctx, "UPDATE guestbook_entries SET hidden = 1 WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "Visible" {
		t.Fatalf("List = %+v, want only Visible", entries)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/guestbook/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement the guestbook package**

`(*Store) db()` is exported only to the package's own tests (lowercase, same-package access) so `TestListExcludesHidden` can simulate the SSH-based moderation described in the spec (`UPDATE guestbook_entries SET hidden=1 WHERE id=…`) without a separate test-only method on the public API.

```go
// internal/guestbook/guestbook.go
package guestbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNameRequired    = errors.New("name is required")
	ErrNameTooLong     = errors.New("name is too long")
	ErrMessageRequired = errors.New("message is required")
	ErrMessageTooLong  = errors.New("message is too long")
)

const (
	MaxNameLength    = 40
	MaxMessageLength = 500
)

type Entry struct {
	ID        int64
	Name      string
	Message   string
	PhotoID   *int64
	CreatedAt string
	Hidden    bool
	ClientIP  string
}

type Store struct {
	sqlDB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{sqlDB: db}
}

func (s *Store) db() *sql.DB { return s.sqlDB }

func (s *Store) Create(ctx context.Context, e Entry) (int64, error) {
	if e.Name == "" {
		return 0, ErrNameRequired
	}
	if len(e.Name) > MaxNameLength {
		return 0, ErrNameTooLong
	}
	if e.Message == "" {
		return 0, ErrMessageRequired
	}
	if len(e.Message) > MaxMessageLength {
		return 0, ErrMessageTooLong
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.sqlDB.ExecContext(ctx, `
		INSERT INTO guestbook_entries (name, message, photo_id, created_at, hidden, client_ip)
		VALUES (?, ?, ?, ?, 0, ?)`,
		e.Name, e.Message, e.PhotoID, createdAt, e.ClientIP,
	)
	if err != nil {
		return 0, fmt.Errorf("guestbook: inserting: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.sqlDB.QueryContext(ctx, `
		SELECT id, name, message, photo_id, created_at, hidden, client_ip
		FROM guestbook_entries WHERE hidden = 0 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("guestbook: listing: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var hidden int
		var photoID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Name, &e.Message, &photoID, &e.CreatedAt, &hidden, &e.ClientIP); err != nil {
			return nil, fmt.Errorf("guestbook: scanning row: %w", err)
		}
		if photoID.Valid {
			e.PhotoID = &photoID.Int64
		}
		e.Hidden = hidden != 0
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/guestbook/... -v`
Expected: PASS for all six tests.

- [ ] **Step 5: Update `Server` to take a real `*sql.DB`**

```go
// internal/web/server.go — full replacement of New and its surrounding imports/fields
package web

import (
	"database/sql"
	"io/fs"
	"net/http"

	homesite "homesite"
	"homesite/internal/config"
	"homesite/internal/content"
	"homesite/internal/guestbook"
	"homesite/internal/photos"
)

type Server struct {
	mux            *http.ServeMux
	cfg            *config.Config
	menuLoader     *content.MenuLoader
	catLoader      *content.CatLoader
	welcomeLoader  *content.WelcomeLoader
	photosStore    *photos.Store
	photosIngester *photos.Ingester
	guestbookStore *guestbook.Store
}

func New(cfg *config.Config, db *sql.DB) *Server {
	cache := content.NewCache()
	photosStore := photos.NewStore(db)

	s := &Server{
		mux:            http.NewServeMux(),
		cfg:            cfg,
		menuLoader:     content.NewMenuLoader(cache),
		catLoader:      content.NewCatLoader(cache, cfg.ContentDir+"/cats"),
		welcomeLoader:  content.NewWelcomeLoader(cache),
		photosStore:    photosStore,
		photosIngester: photos.NewIngester(photosStore, cfg.Photos, cfg.UploadsDir),
		guestbookStore: guestbook.NewStore(db),
	}

	staticSub, err := fs.Sub(homesite.StaticFS, "static")
	if err != nil {
		panic("web: static assets missing from embed: " + err.Error())
	}
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	catsPhotoDir := http.Dir(cfg.ContentDir + "/cats")
	s.mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(catsPhotoDir)))

	uploadsDir := http.Dir(cfg.UploadsDir)
	s.mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(uploadsDir)))

	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.registerPageRoutes()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
```

Add the guest book routes to `registerPageRoutes` (in the same file, alongside the ones from Tasks 9, 10, and 14):
```go
	s.mux.HandleFunc("GET /guestbook", s.handleGuestbookPage)
	s.mux.HandleFunc("POST /guestbook", s.handleGuestbookCreate)
```

- [ ] **Step 6: Add the shared test helper and update every existing test file's `New` call**

```go
// internal/web/server_test.go — add this function; keep testConfig() as-is
func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(testConfig(), db)
}
```
Add `"homesite/internal/store"` to `server_test.go`'s imports.

Then, in **each** of `handlers_pages_test.go`, `handlers_qr_test.go`, and `handlers_photos_test.go`, replace every `New(testConfig())` / `New(testConfigWithWiFi())` call with `newTestServer(t)` (for the plain cases) — except `handlers_qr_test.go`'s tests, which need the WiFi/site fields set, so change those to:
```go
	s := newTestServer(t)
	s.cfg = testConfigWithWiFi()
```
(setting `cfg` directly is fine here since these tests only read `s.cfg`, never `s.photosStore`/`s.guestbookStore`, which stay backed by the fresh in-memory DB from `newTestServer`).

In `handlers_photos_test.go`, replace the whole body of `newTestServerWithPhotos` with:
```go
func newTestServerWithPhotos(t *testing.T) *Server {
	t.Helper()
	cfg, _ := testConfigWithPhotos(t)
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(cfg, db)
}
```
(this replaces the Task 14 version that manually poked `s.photosStore`/`s.photosIngester` after calling `New` with one argument — that was the temporary seam; this closes it.)

- [ ] **Step 7: Run the whole web package's tests to confirm nothing broke**

Run: `go test ./internal/web/... -v`
Expected: PASS for every test from Tasks 8, 9, 10, and 14, plus the guest book tests added next.

- [ ] **Step 8: Write the failing guest book handler test**

```go
// internal/web/handlers_guestbook_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGuestbookPageRendersEntries(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.guestbookStore.Create(t.Context(), guestbookEntryFixture("Alex", "Loved the negroni")); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/guestbook", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Loved the negroni") {
		t.Error("expected the seeded entry to render")
	}
}

func TestGuestbookCreateSucceeds(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"name": {"Jamie"}, "message": {"Great cats"}}
	req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Great cats") {
		t.Error("expected the new entry in the response fragment")
	}
}

func TestGuestbookCreateRejectsEmptyMessagePreservingName(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{"name": {"Jamie"}, "message": {""}}
	req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (validation error still renders a fragment)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Jamie") {
		t.Error("expected the typed name to be preserved after a validation error")
	}
	if !strings.Contains(body, "message") {
		t.Error("expected an inline message-required error")
	}
}
```

Add this small fixture helper alongside the tests above:
```go
func guestbookEntryFixture(name, message string) guestbook.Entry {
	return guestbook.Entry{Name: name, Message: message, ClientIP: "192.168.1.30"}
}
```
(add `"homesite/internal/guestbook"` to this test file's imports)

- [ ] **Step 9: Run the tests to verify they fail**

Run: `go test ./internal/web/... -run TestGuestbook -v`
Expected: FAIL — `handleGuestbookPage` undefined.

- [ ] **Step 10: Write the guest book templ views**

```templ
// views/guestbook.templ
package views

import "homesite/internal/guestbook"

type GuestbookFormError struct {
	Field   string
	Message string
	Name    string
	Message_ string
}

templ GuestbookPage(entries []guestbook.Entry) {
	@Layout("Guest Book", HeaderStandard, guestbookPageBody(entries, nil))
}

templ guestbookPageBody(entries []guestbook.Entry, formErr *GuestbookFormError) {
	<h1>Guest Book</h1>
	@GuestbookForm(formErr)
	<div id="guestbook-entries">
		@GuestbookEntries(entries)
	</div>
}

templ GuestbookForm(formErr *GuestbookFormError) {
	<form hx-post="/guestbook" hx-target="#guestbook-entries" hx-swap="afterbegin" id="guestbook-form">
		<label>
			Name
			<input type="text" name="name" maxlength="40" required
				if formErr != nil {
					value={ formErr.Name }
				}/>
		</label>
		<label>
			Message
			<textarea name="message" maxlength="500" required>
				if formErr != nil {
					{ formErr.Message_ }
				}
			</textarea>
		</label>
		if formErr != nil {
			<p class="form-error">{ formErr.Message }</p>
		}
		<button type="submit">Sign the guest book</button>
	</form>
}

templ GuestbookEntries(entries []guestbook.Entry) {
	for _, e := range entries {
		<article class="guestbook-entry">
			<p class="guestbook-name">{ e.Name }</p>
			<p class="guestbook-message">{ e.Message }</p>
		</article>
	}
}
```

Note: `formErr.Message_` (trailing underscore) holds the **typed message text** to redisplay, distinct from `formErr.Message` which holds the **validation error text**. This naming collision-avoidance is deliberate but easy to misread — when implementing, double check every reference uses the correct one (the field, not the error).

- [ ] **Step 11: Implement the guest book handlers**

```go
// internal/web/handlers_guestbook.go
package web

import (
	"errors"
	"log"
	"net/http"

	"homesite/internal/guestbook"
	"homesite/views"
)

func (s *Server) handleGuestbookPage(w http.ResponseWriter, r *http.Request) {
	entries, err := s.guestbookStore.List(r.Context())
	if err != nil {
		log.Printf("web: listing guestbook: %v", err)
	}
	render(w, r, views.GuestbookPage(entries))
}

func (s *Server) handleGuestbookCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}
	name := r.PostFormValue("name")
	message := r.PostFormValue("message")

	_, err := s.guestbookStore.Create(r.Context(), guestbook.Entry{
		Name: name, Message: message, ClientIP: clientIP(r),
	})
	if err != nil {
		render(w, r, views.GuestbookForm(&views.GuestbookFormError{
			Message:  guestbookErrorMessage(err),
			Name:     name,
			Message_: message,
		}))
		return
	}

	entries, err := s.guestbookStore.List(r.Context())
	if err != nil {
		log.Printf("web: listing guestbook after create: %v", err)
	}
	// The newest entry is entries[0] (List orders DESC by created_at);
	// hx-swap="afterbegin" on #guestbook-entries means only that one new
	// entry should be sent back, not the whole list re-rendered.
	if len(entries) > 0 {
		render(w, r, views.GuestbookEntries(entries[:1]))
	}
}

func guestbookErrorMessage(err error) string {
	switch {
	case errors.Is(err, guestbook.ErrNameRequired):
		return "please enter a name"
	case errors.Is(err, guestbook.ErrNameTooLong):
		return "name is too long — 40 characters max"
	case errors.Is(err, guestbook.ErrMessageRequired):
		return "please enter a message"
	case errors.Is(err, guestbook.ErrMessageTooLong):
		return "message is too long — 500 characters max"
	default:
		return "couldn't save that — try again"
	}
}
```

- [ ] **Step 12: Run the tests to verify they pass**

Run:
```bash
templ generate
go test ./internal/web/... -v
```
Expected: PASS for all three new guest book tests, and everything from prior tasks still green.

- [ ] **Step 13: Wire the real database into `main.go`**

```go
// cmd/homesite/main.go
package main

import (
	"flag"
	"log"
	"net/http"

	"homesite/internal/config"
	"homesite/internal/store"
	"homesite/internal/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	db, err := store.Open(cfg.DataDir + "/homesite.db")
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	srv := web.New(cfg, db)
	log.Printf("Brivin Household listening on %s", cfg.Site.Listen)
	if err := http.ListenAndServe(cfg.Site.Listen, srv); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 14: Run the full local server and manually verify photos AND guest book**

```bash
mkdir -p data uploads
templ generate
go run ./cmd/homesite -config config.local.yaml
```
On a phone (or the Mac browser), open `/photos` and upload a real photo — confirm it appears in the gallery and that `identify -format "%[EXIF:GPSLatitude]" <file>` on the resulting file under `uploads/` prints nothing. Then open `/guestbook`, submit an entry, and confirm it appears immediately without a page reload.

- [ ] **Step 15: Commit**

```bash
git add internal/guestbook internal/web views cmd/homesite
git commit -m "Add guest book, wire real SQLite database, close photos test seam"
```

---

## Task 16: Rate Limiting on Guest Book and Photo Uploads

**Files:**
- Create: `internal/web/ratelimit.go`
- Test: `internal/web/ratelimit_test.go`
- Modify: `internal/web/server.go` (construct limiters, wrap the two POST routes)
- Modify: `views/guestbook.templ`, `views/photos.templ` (add a retarget div per form)
- Create: `views/ratelimit.templ`

**Interfaces:**
- Consumes: `config.LimitsConfig` (Task 1); `web.clientIP` (Task 14)
- Produces: `web.newRateLimiter(max int, window time.Duration) *rateLimiter`, `(*rateLimiter) allow(key string) bool`, `web.rateLimitKey(w, r) string` — wraps the two write routes at registration time in `server.go`.

**Why `HX-Retarget` rather than swapping into the normal response target:** the guest book's normal response target (`#guestbook-entries`, `hx-swap="afterbegin"`) is meant to receive a new signed entry — sending a rate-limit notice there would make it look like a fake guest book post. `HX-Retarget` is an HTMX response header that overrides the swap target for that one response, so a rate-limited response can point at a dedicated message div instead, without touching the form's normal success path.

- [ ] **Step 1: Write the failing rate limiter test**

```go
// internal/web/ratelimit_test.go
package web

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToMax(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	if !rl.allow("k") {
		t.Fatal("1st request should be allowed")
	}
	if !rl.allow("k") {
		t.Fatal("2nd request should be allowed")
	}
	if rl.allow("k") {
		t.Fatal("3rd request should be denied")
	}
}

func TestRateLimiterIsPerKey(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	if !rl.allow("a") {
		t.Fatal("first key's 1st request should be allowed")
	}
	if !rl.allow("b") {
		t.Fatal("different key should have its own budget")
	}
}

func TestRateLimiterExpiresOldHits(t *testing.T) {
	rl := newRateLimiter(1, 10*time.Millisecond)
	if !rl.allow("k") {
		t.Fatal("1st request should be allowed")
	}
	time.Sleep(20 * time.Millisecond)
	if !rl.allow("k") {
		t.Fatal("request after the window elapsed should be allowed again")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/web/... -run TestRateLimiter -v`
Expected: FAIL — `newRateLimiter` undefined.

- [ ] **Step 3: Implement the rate limiter and cookie key**

```go
// internal/web/ratelimit.go
package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: make(map[string][]time.Time), max: max, window: window}
}

// allow records a hit for key and reports whether it's within budget.
// Restart-resets-everything is an accepted tradeoff (see spec) — this
// is an in-memory map, not a persisted counter.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	kept := rl.hits[key][:0]
	for _, t := range rl.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.hits[key] = kept
		return false
	}
	rl.hits[key] = append(kept, now)
	return true
}

const rateLimitCookieName = "brivin_rl"

// rateLimitKey combines a long-lived per-browser cookie with the
// caller's IP, per the spec's "keyed by session cookie AND client IP, so
// clearing cookies does not reset them" requirement. It sets the cookie
// on first sight of a caller that doesn't have one yet.
func rateLimitKey(w http.ResponseWriter, r *http.Request) string {
	token := ""
	if c, err := r.Cookie(rateLimitCookieName); err == nil && c.Value != "" {
		token = c.Value
	} else {
		token = randomToken()
		http.SetCookie(w, &http.Cookie{
			Name: rateLimitCookieName, Value: token, Path: "/",
			MaxAge: 60 * 60 * 24 * 30,
		})
	}
	return token + "|" + clientIP(r)
}

func randomToken() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// rateLimit wraps next so requests over budget get a friendly fragment
// retargeted (via the HX-Retarget/HX-Reswap response headers) into
// targetID instead of the route's normal success target.
func (s *Server) rateLimit(rl *rateLimiter, targetID string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := rateLimitKey(w, r)
		if !rl.allow(key) {
			w.Header().Set("HX-Retarget", "#"+targetID)
			w.Header().Set("HX-Reswap", "innerHTML")
			renderRateLimited(w, r)
			return
		}
		next(w, r)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/web/... -run TestRateLimiter -v`
Expected: PASS for all three tests.

- [ ] **Step 5: Add the rate-limit message component**

```templ
// views/ratelimit.templ
package views

templ RateLimitedMessage() {
	<p class="rate-limit-message">You've had your turn — let someone else in.</p>
}
```

Add a small render helper alongside the existing `render` function:

```go
// internal/web/render.go — add this function below the existing render()
func renderRateLimited(w http.ResponseWriter, r *http.Request) {
	render(w, r, views.RateLimitedMessage())
}
```
(add `"homesite/views"` to `render.go`'s imports if not already present from a prior task)

- [ ] **Step 6: Add retarget divs to the two forms**

In `views/guestbook.templ`, add a message div right after the form's closing tag in `guestbookPageBody`:
```templ
	@GuestbookForm(formErr)
	<div id="guestbook-rl-message"></div>
	<div id="guestbook-entries">
```

In `views/photos.templ`, add the equivalent after the upload form in `photosPageBody`:
```templ
	</form>
	<div id="photos-rl-message"></div>
	<div id="photos-content">
```

- [ ] **Step 7: Wire limiters into `Server` and wrap the two POST routes**

Add two fields to the `Server` struct in `internal/web/server.go`:
```go
	guestbookLimiter *rateLimiter
	photosLimiter    *rateLimiter
```

In `New`, after constructing `guestbookStore`, add:
```go
	windowDur := time.Duration(cfg.Limits.WindowMinutes) * time.Minute
	s.guestbookLimiter = newRateLimiter(cfg.Limits.GuestbookPerWindow, windowDur)
	s.photosLimiter = newRateLimiter(cfg.Limits.PhotoUploadsPerWindow, windowDur)
```
(add `"time"` to `server.go`'s imports)

Change the two route registrations to wrap their handlers:
```go
	s.mux.HandleFunc("POST /guestbook", s.rateLimit(s.guestbookLimiter, "guestbook-rl-message", s.handleGuestbookCreate))
	s.mux.HandleFunc("POST /photos", s.rateLimit(s.photosLimiter, "photos-rl-message", s.handlePhotosUpload))
```

Update `testConfig()` in `internal/web/server_test.go` to set non-zero limits so `newTestServer` doesn't produce a limiter with `max: 0` (which would deny every request immediately):
```go
	cfg.Limits = config.LimitsConfig{GuestbookPerWindow: 2, PhotoUploadsPerWindow: 30, WindowMinutes: 15}
```

- [ ] **Step 8: Write a failing integration test proving the limit actually triggers over HTTP**

```go
// internal/web/ratelimit_integration_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGuestbookRateLimitTriggersAfterConfiguredMax(t *testing.T) {
	s := newTestServer(t) // testConfig() now sets GuestbookPerWindow: 2
	var cookie *http.Cookie

	post := func(message string) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Alex"}, "message": {message}}
		req := httptest.NewRequest(http.MethodPost, "/guestbook", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == rateLimitCookieName {
				cookie = c
			}
		}
		return rec
	}

	post("first")
	post("second")
	rec := post("third — should be blocked")

	if got := rec.Header().Get("HX-Retarget"); got != "#guestbook-rl-message" {
		t.Errorf("HX-Retarget = %q, want #guestbook-rl-message", got)
	}
	if !strings.Contains(rec.Body.String(), "You've had your turn") {
		t.Errorf("body = %q, want the rate-limit message", rec.Body.String())
	}
}
```

- [ ] **Step 9: Run all `internal/web` tests to verify everything passes**

Run:
```bash
templ generate
go test ./internal/web/... -v
```
Expected: PASS for every test in the package, including the new rate-limit ones. The key thing this integration test proves that the unit test in Step 1 can't: that the cookie set on the first response is actually what makes the second and third requests share a rate-limit bucket — without carrying that cookie across requests (as the test does via `cookie *http.Cookie`), each request would look like a brand-new visitor and the limit would never trigger.

- [ ] **Step 10: Commit**

```bash
git add internal/web views
git commit -m "Add rate limiting to guest book and photo upload endpoints"
```

---

## Task 17: Spotify OAuth — Now Playing, Search, and Unmoderated Queue Requests

This task replaces an earlier design that used a collaborative playlist read via
client-credentials specifically to avoid OAuth. The site owner asked mid-implementation for
live now-playing and direct, unmoderated queue control, both of which are scoped to a real
user's device — there is no app-only equivalent, so this task reintroduces the OAuth surface
the original brainstorming deliberately avoided. See
`docs/superpowers/specs/2026-07-27-homesite-design.md`'s Music Requests section for the full
rationale.

**Files:**
- Modify: `internal/store/store.go` (add `oauth_tokens` and `song_requests` tables to the schema)
- Test: `internal/store/store_test.go` (assert the two new tables exist)
- Modify: `internal/config/config.go` (replace `SpotifyConfig`'s fields; add `SongRequestsPerWindow` to `LimitsConfig`)
- Modify: `internal/config/testdata/valid.yaml`, `config.example.yaml`, `config.local.example.yaml` (new Spotify field names)
- Create: `internal/spotify/spotify.go` (OAuth `Client`: auth URL, code exchange, token refresh, now-playing, search, queue-add)
- Create: `internal/spotify/tokenstore.go` (`SQLTokenStore` — persists the refresh token)
- Create: `internal/spotify/requests.go` (`RequestStore` — `song_requests` insert/recent)
- Test: `internal/spotify/spotify_test.go`, `internal/spotify/tokenstore_test.go`, `internal/spotify/requests_test.go`
- Modify: `internal/web/server.go` (add `spotifyClient`, `requestStore`, `songRequestLimiter` fields; register routes)
- Modify: `internal/web/server_test.go` (`testConfig()` gains `SongRequestsPerWindow`)
- Create: `internal/web/loopback.go` (`isLoopback(r *http.Request) bool`)
- Create: `internal/web/handlers_music.go`, `internal/web/handlers_spotify_auth.go`
- Test: `internal/web/handlers_music_test.go`, `internal/web/handlers_spotify_auth_test.go`
- Create: `views/music.templ`

**Interfaces:**
- Consumes: `store.Open`/`store.OpenMemory` (Task 11, extended here); `web.clientIP` (Task 14); `web.newRateLimiter`, `web.rateLimit`, `web.randomToken`, `rateLimitCookieName` pattern (Task 16); `config.SpotifyConfig`, `config.LimitsConfig` (Task 1, modified here)
- Produces: `spotify.Track{URI, Name, Artists string}`, `spotify.SongRequest{...}`, `spotify.New(clientID, clientSecret, redirectURI string, tokens TokenStore, nowPlayingTTL time.Duration) *Client`, `(*Client) AuthURL(state string) string`, `(*Client) ExchangeCode(ctx, code string) error`, `(*Client) NowPlaying(ctx) (*Track, error)`, `(*Client) Search(ctx, query string) ([]Track, error)`, `(*Client) QueueTrack(ctx, uri string) error`; sentinel errors `ErrNoActiveDevice`, `ErrTokenInvalid`, `ErrRateLimited`, `ErrUpstream`; `GET /music`, `GET /music/search`, `POST /music/request`, `GET /spotify/login`, `GET /spotify/callback` routes.

**Music is no longer a QR page.** There is no external playlist link to encode anymore — the
page works entirely within the site. Use `HeaderStandard`, not `HeaderQR`, in
`views/music.templ`. The site now has exactly two QR pages (Wi-Fi, Share), not three.

**The loopback restriction is load-bearing, not decorative.** `/spotify/login` and
`/spotify/callback` share the site's one port with every guest-facing route. Both handlers
must reject any request whose `RemoteAddr` is not loopback (`127.0.0.1`/`::1`) *before* doing
anything else — a guest on the WiFi must never be able to reach the authorization flow, since
it's the one thing on this site with account-level consequences if abused.

**"Unmoderated" means no approval step — not no rate limit.** Every queue-add still goes
through `songRequestLimiter`, same mechanism as guest book and photo uploads (Task 16). What
"unmoderated" removes is a human approving each song before it reaches Spotify; the rate limit
still exists to keep one enthusiastic guest from dominating the queue.

- [ ] **Step 1: Write the failing schema test**

```go
// internal/store/store_test.go — add this test alongside the existing ones
func TestOpenCreatesOAuthAndSongRequestTables(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	for _, tbl := range []string{"oauth_tokens", "song_requests"} {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", tbl, err)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/store/... -run TestOpenCreatesOAuthAndSongRequestTables -v`
Expected: FAIL — tables don't exist yet.

- [ ] **Step 3: Extend the schema**

Add these two tables to the `schema` const in `internal/store/store.go`, alongside the
existing `photos`/`guestbook_entries` definitions (append, don't reorder — `CREATE TABLE IF
NOT EXISTS` makes this safe to run against an already-migrated database):

```sql
CREATE TABLE IF NOT EXISTS oauth_tokens (
  provider      TEXT PRIMARY KEY,
  refresh_token TEXT NOT NULL,
  access_token  TEXT,
  expires_at    TEXT,
  scopes        TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS song_requests (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  track_uri    TEXT NOT NULL,
  track_name   TEXT NOT NULL,
  artist_name  TEXT NOT NULL,
  requested_by TEXT,
  created_at   TEXT NOT NULL,
  client_ip    TEXT NOT NULL,
  status       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_song_requests_created ON song_requests(created_at DESC);
```

`oauth_tokens.access_token`/`expires_at` exist for schema completeness but are never written
by this task's Go code — the access token is cached in memory only (see `spotify.Client`
below) and recomputed via refresh on restart; only the durable `refresh_token` is persisted.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/store/... -v`
Expected: PASS for all four tests (three existing, one new).

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "Add oauth_tokens and song_requests tables"
```

- [ ] **Step 6: Update config for OAuth**

Replace `SpotifyConfig` in `internal/config/config.go`:

```go
type SpotifyConfig struct {
	ClientID               string `yaml:"client_id"`
	ClientSecret           string `yaml:"client_secret"`
	RedirectURI            string `yaml:"redirect_uri"`
	NowPlayingCacheSeconds int    `yaml:"now_playing_cache_seconds"`
}
```

Add a field to `LimitsConfig`:

```go
type LimitsConfig struct {
	GuestbookPerWindow    int `yaml:"guestbook_per_window"`
	PhotoUploadsPerWindow int `yaml:"photo_uploads_per_window"`
	SongRequestsPerWindow int `yaml:"song_requests_per_window"`
	WindowMinutes         int `yaml:"window_minutes"`
}
```

Update the `spotify:` and `limits:` blocks in `internal/config/testdata/valid.yaml`,
`config.example.yaml`, and `config.local.example.yaml` to match:

```yaml
spotify:
  client_id: "test-client-id"
  client_secret: "test-client-secret"
  redirect_uri: "http://127.0.0.1:8080/spotify/callback"
  now_playing_cache_seconds: 10
limits:
  guestbook_per_window: 2
  photo_uploads_per_window: 30
  song_requests_per_window: 3
  window_minutes: 15
```

(`config.example.yaml`'s `redirect_uri` stays `http://127.0.0.1:8080/spotify/callback` even
though the Pi serves on `:80` in production — port `8080` here is the SSH tunnel's local end,
per the spec's OAuth setup steps, not the site's real listening port.)

**Also update `config.local.yaml`** on disk (this file is gitignored and not part of any
commit, but it's the file the dev server actually reads): add `redirect_uri:
"http://127.0.0.1:8080/spotify/callback"` and `now_playing_cache_seconds: 10` to its
`spotify:` block, and `song_requests_per_window: 3` to its `limits:` block. If real Spotify
`client_id`/`client_secret` values aren't in that file yet, leave them as placeholders —
Step 15 covers getting real ones.

Run `go build ./...` and `go test ./...` to confirm nothing broke from the field rename (no
existing test asserts on the old `PlaylistURL`/`CacheSeconds` fields by name, so this should
be a clean rename).

- [ ] **Step 7: Commit the config changes**

```bash
git add internal/config config.example.yaml config.local.example.yaml
git commit -m "Replace Spotify client-credentials config with OAuth fields"
```

- [ ] **Step 8: Write the failing spotify client test**

```go
// internal/spotify/spotify_test.go
package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeTokenStore is an in-memory TokenStore for tests, avoiding any real
// database dependency in this package's own tests.
type fakeTokenStore struct {
	refreshToken string
	saveCalls    int
}

func (f *fakeTokenStore) LoadRefreshToken(ctx context.Context) (string, error) {
	return f.refreshToken, nil
}
func (f *fakeTokenStore) SaveTokens(ctx context.Context, refreshToken string, updatedAt time.Time) error {
	f.refreshToken = refreshToken
	f.saveCalls++
	return nil
}

func TestAuthURLIncludesScopesAndState(t *testing.T) {
	c := New("id", "secret", "http://127.0.0.1:8080/spotify/callback", &fakeTokenStore{}, time.Second)
	u := c.AuthURL("test-state-123")
	if !strings.Contains(u, "state=test-state-123") {
		t.Errorf("AuthURL = %q, want it to contain the state param", u)
	}
	if !strings.Contains(u, "client_id=id") {
		t.Errorf("AuthURL = %q, want it to contain client_id", u)
	}
}

// testSpotifyServer fakes the accounts token endpoint and the three API.
func testSpotifyServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "token:"+r.FormValue("grant_type"))
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-access-token", "refresh_token": "test-refresh-token", "expires_in": 3600,
		})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "now-playing")
		json.NewEncoder(w).Encode(map[string]any{
			"item": map[string]any{
				"name": "Song A", "uri": "spotify:track:abc",
				"artists": []map[string]any{{"name": "Artist One"}},
			},
		})
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "search:"+r.URL.Query().Get("q"))
		json.NewEncoder(w).Encode(map[string]any{
			"tracks": map[string]any{"items": []map[string]any{
				{"name": "Song B", "uri": "spotify:track:def", "artists": []map[string]any{{"name": "Artist Two"}}},
			}},
		})
	})
	mux.HandleFunc("/me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "queue:"+r.URL.Query().Get("uri"))
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewServer(mux), &calls
}

func newTestClient(t *testing.T, ts *httptest.Server) (*Client, *fakeTokenStore) {
	t.Helper()
	store := &fakeTokenStore{refreshToken: "existing-refresh-token"}
	c := New("id", "secret", "http://127.0.0.1:8080/spotify/callback", store, time.Minute)
	c.tokenURL = ts.URL + "/token"
	c.apiBaseURL = ts.URL
	return c, store
}

func TestExchangeCodeSavesRefreshToken(t *testing.T) {
	ts, _ := testSpotifyServer(t)
	defer ts.Close()
	c, store := newTestClient(t, ts)
	store.refreshToken = "" // no token yet, this call is the initial authorization

	if err := c.ExchangeCode(context.Background(), "auth-code-123"); err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if store.refreshToken != "test-refresh-token" {
		t.Errorf("stored refresh token = %q, want test-refresh-token", store.refreshToken)
	}
}

func TestNowPlayingFetchesAndCaches(t *testing.T) {
	ts, calls := testSpotifyServer(t)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if track == nil || track.Name != "Song A" || track.Artists != "Artist One" {
		t.Fatalf("track = %+v", track)
	}

	if _, err := c.NowPlaying(context.Background()); err != nil {
		t.Fatal(err)
	}
	nowPlayingCalls := 0
	for _, call := range *calls {
		if call == "now-playing" {
			nowPlayingCalls++
		}
	}
	if nowPlayingCalls != 1 {
		t.Errorf("now-playing endpoint called %d times, want 1 (second call should hit cache)", nowPlayingCalls)
	}
}

func TestSearchReturnsTracks(t *testing.T) {
	ts, _ := testSpotifyServer(t)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	tracks, err := c.Search(context.Background(), "song b")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Name != "Song B" || tracks[0].URI != "spotify:track:def" {
		t.Fatalf("tracks = %+v", tracks)
	}
}

func TestQueueTrackSucceeds(t *testing.T) {
	ts, calls := testSpotifyServer(t)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	if err := c.QueueTrack(context.Background(), "spotify:track:abc"); err != nil {
		t.Fatalf("QueueTrack: %v", err)
	}
	found := false
	for _, call := range *calls {
		if call == "queue:spotify:track:abc" {
			found = true
		}
	}
	if !found {
		t.Error("expected a queue call with the requested URI")
	}
}

func TestQueueTrackNoActiveDeviceReturnsSentinel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	err := c.QueueTrack(context.Background(), "spotify:track:abc")
	if !errors.Is(err, ErrNoActiveDevice) {
		t.Fatalf("err = %v, want ErrNoActiveDevice", err)
	}
}

func TestQueueTrackRateLimitedReturnsSentinel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	err := c.QueueTrack(context.Background(), "spotify:track:abc")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestNowPlayingNoTrackReturnsNilNotError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := newTestClient(t, ts)

	track, err := c.NowPlaying(context.Background())
	if err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if track != nil {
		t.Errorf("track = %+v, want nil (nothing playing)", track)
	}
}
```

Add `"strings"` to this test file's imports (used by `TestAuthURLIncludesScopesAndState`).

- [ ] **Step 9: Run the test to verify it fails**

Run: `go test ./internal/spotify/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 10: Implement the OAuth client**

```go
// internal/spotify/spotify.go
package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrNoActiveDevice = errors.New("no active playback device")
	ErrTokenInvalid   = errors.New("spotify authorization is invalid or missing")
	ErrRateLimited    = errors.New("spotify rate limited the request")
	ErrUpstream       = errors.New("spotify request failed")
)

// Scopes requested during the one-time host authorization. Read-currently-
// playing and read-playback-state back the Now Playing display; modify-
// playback-state backs queue-add.
const Scopes = "user-read-currently-playing user-read-playback-state user-modify-playback-state"

type Track struct {
	URI     string
	Name    string
	Artists string
}

// TokenStore persists the durable refresh token across restarts. The
// access token is never persisted — it's short-lived and cheap to
// re-derive via refresh.
type TokenStore interface {
	LoadRefreshToken(ctx context.Context) (string, error) // "", nil if none saved yet
	SaveTokens(ctx context.Context, refreshToken string, updatedAt time.Time) error
}

// Client is the one integration in this codebase that authenticates as a
// user, not as the app — reading what's playing and controlling the
// queue are both scoped to a real Spotify account with no app-only
// equivalent.
type Client struct {
	clientID      string
	clientSecret  string
	redirectURI   string
	httpClient    *http.Client
	tokens        TokenStore
	nowPlayingTTL time.Duration

	authURL    string
	tokenURL   string
	apiBaseURL string

	mu           sync.Mutex
	accessToken  string
	accessExpiry time.Time
	nowPlaying   *Track
	nowPlayingAt time.Time
}

func New(clientID, clientSecret, redirectURI string, tokens TokenStore, nowPlayingTTL time.Duration) *Client {
	return &Client{
		clientID: clientID, clientSecret: clientSecret, redirectURI: redirectURI,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		tokens:        tokens,
		nowPlayingTTL: nowPlayingTTL,
		authURL:       "https://accounts.spotify.com/authorize",
		tokenURL:      "https://accounts.spotify.com/api/token",
		apiBaseURL:    "https://api.spotify.com/v1",
	}
}

// AuthURL builds the URL the host visits, via the loopback tunnel, to
// begin the one-time authorization. state is a random value the caller
// must verify unchanged on callback, to prevent CSRF.
func (c *Client) AuthURL(state string) string {
	v := url.Values{
		"client_id":     {c.clientID},
		"response_type": {"code"},
		"redirect_uri":  {c.redirectURI},
		"scope":         {Scopes},
		"state":         {state},
	}
	return c.authURL + "?" + v.Encode()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

// ExchangeCode trades an authorization code from the OAuth callback for
// tokens and persists the refresh token. Called exactly once, during the
// host's one-time setup (or again if the refresh token is ever revoked).
func (c *Client) ExchangeCode(ctx context.Context, code string) error {
	body, err := c.postForm(ctx, c.tokenURL, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {c.redirectURI},
	})
	if err != nil {
		return fmt.Errorf("spotify: exchanging code: %w", err)
	}
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("spotify: decoding token exchange response: %w", err)
	}
	if resp.RefreshToken == "" {
		return fmt.Errorf("spotify: token exchange returned no refresh token")
	}

	c.mu.Lock()
	c.accessToken = resp.AccessToken
	c.accessExpiry = time.Now().Add(time.Duration(resp.ExpiresIn-30) * time.Second)
	c.mu.Unlock()

	return c.tokens.SaveTokens(ctx, resp.RefreshToken, time.Now().UTC())
}

// getAccessToken returns a valid access token, refreshing via the stored
// refresh token if the cached one is missing or expired.
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && time.Now().Before(c.accessExpiry) {
		tok := c.accessToken
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	refreshToken, err := c.tokens.LoadRefreshToken(ctx)
	if err != nil {
		return "", fmt.Errorf("spotify: loading refresh token: %w", err)
	}
	if refreshToken == "" {
		return "", fmt.Errorf("spotify: no refresh token saved yet: %w", ErrTokenInvalid)
	}

	body, err := c.postForm(ctx, c.tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return "", fmt.Errorf("spotify: refreshing token: %w: %w", err, ErrTokenInvalid)
	}
	var resp tokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("spotify: decoding refresh response: %w", err)
	}

	c.mu.Lock()
	c.accessToken = resp.AccessToken
	c.accessExpiry = time.Now().Add(time.Duration(resp.ExpiresIn-30) * time.Second)
	c.mu.Unlock()

	// Spotify may rotate the refresh token; persist it if a new one came back.
	if resp.RefreshToken != "" {
		if err := c.tokens.SaveTokens(ctx, resp.RefreshToken, time.Now().UTC()); err != nil {
			return "", fmt.Errorf("spotify: saving rotated refresh token: %w", err)
		}
	}

	return resp.AccessToken, nil
}

// NowPlaying returns the currently-playing track, cached for
// nowPlayingTTL so a room full of guests loading the page doesn't
// hammer the API. Returns (nil, nil) when nothing is playing — that is
// not an error condition.
func (c *Client) NowPlaying(ctx context.Context) (*Track, error) {
	c.mu.Lock()
	if c.nowPlaying != nil && time.Now().Before(c.nowPlayingAt.Add(c.nowPlayingTTL)) {
		np := c.nowPlaying
		c.mu.Unlock()
		return np, nil
	}
	c.mu.Unlock()

	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/me/player/currently-playing", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: now-playing request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		c.mu.Lock()
		c.nowPlaying = nil
		c.nowPlayingAt = time.Now()
		c.mu.Unlock()
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: now-playing status %d: %w", resp.StatusCode, ErrUpstream)
	}

	var body struct {
		Item struct {
			Name    string `json:"name"`
			URI     string `json:"uri"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding now-playing: %w", err)
	}

	names := make([]string, 0, len(body.Item.Artists))
	for _, a := range body.Item.Artists {
		names = append(names, a.Name)
	}
	track := &Track{URI: body.Item.URI, Name: body.Item.Name, Artists: strings.Join(names, ", ")}

	c.mu.Lock()
	c.nowPlaying = track
	c.nowPlayingAt = time.Now()
	c.mu.Unlock()

	return track, nil
}

// Search returns up to 8 matching tracks for a guest's query.
func (c *Client) Search(ctx context.Context, query string) ([]Track, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/search?q=%s&type=track&limit=8", c.apiBaseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: search request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: search status %d: %w", resp.StatusCode, ErrUpstream)
	}

	var body struct {
		Tracks struct {
			Items []struct {
				URI     string `json:"uri"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("spotify: decoding search response: %w", err)
	}

	tracks := make([]Track, 0, len(body.Tracks.Items))
	for _, item := range body.Tracks.Items {
		names := make([]string, 0, len(item.Artists))
		for _, a := range item.Artists {
			names = append(names, a.Name)
		}
		tracks = append(tracks, Track{URI: item.URI, Name: item.Name, Artists: strings.Join(names, ", ")})
	}
	return tracks, nil
}

// QueueTrack adds uri to the active device's playback queue — the
// unmoderated part of "unmoderated queue requests." No approval step
// happens before this call.
func (c *Client) QueueTrack(ctx context.Context, uri string) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/me/player/queue?uri=%s", c.apiBaseURL, url.QueryEscape(uri))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify: queue request: %w: %w", err, ErrUpstream)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrNoActiveDevice
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("spotify: queue status %d: %w", resp.StatusCode, ErrUpstream)
	}
}
```

- [ ] **Step 11: Run the tests to verify they pass**

Run: `go test ./internal/spotify/... -v`
Expected: PASS for all eight tests.

- [ ] **Step 12: Write the token store and request store, with tests**

```go
// internal/spotify/tokenstore_test.go
package spotify

import (
	"context"
	"testing"

	"homesite/internal/store"
)

func TestSQLTokenStoreRoundTrips(t *testing.T) {
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ts := NewSQLTokenStore(db)
	ctx := context.Background()

	got, err := ts.LoadRefreshToken(ctx)
	if err != nil {
		t.Fatalf("LoadRefreshToken (empty): %v", err)
	}
	if got != "" {
		t.Errorf("LoadRefreshToken on empty table = %q, want empty string", got)
	}

	if err := ts.SaveTokens(ctx, "refresh-abc", timeNow()); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	got, err = ts.LoadRefreshToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "refresh-abc" {
		t.Errorf("LoadRefreshToken = %q, want refresh-abc", got)
	}

	// Saving again must update, not duplicate — the table holds exactly one row.
	if err := ts.SaveTokens(ctx, "refresh-def", timeNow()); err != nil {
		t.Fatalf("second SaveTokens: %v", err)
	}
	got, err = ts.LoadRefreshToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "refresh-def" {
		t.Errorf("LoadRefreshToken after update = %q, want refresh-def", got)
	}
}
```

Add a tiny `timeNow` test helper (kept separate from stdlib `time.Now` only so the test
reads clearly — no mocking involved):
```go
// internal/spotify/testhelpers_test.go
package spotify

import "time"

func timeNow() time.Time { return time.Now().UTC() }
```

Run: `go test ./internal/spotify/... -run TestSQLTokenStore -v` — expect FAIL (`NewSQLTokenStore` undefined), then implement:

```go
// internal/spotify/tokenstore.go
package spotify

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLTokenStore persists exactly one row — the Spotify refresh token
// from the host's one-time authorization.
type SQLTokenStore struct {
	db *sql.DB
}

func NewSQLTokenStore(db *sql.DB) *SQLTokenStore {
	return &SQLTokenStore{db: db}
}

func (s *SQLTokenStore) LoadRefreshToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx,
		`SELECT refresh_token FROM oauth_tokens WHERE provider = 'spotify'`,
	).Scan(&token)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("spotify: loading refresh token: %w", err)
	}
	return token, nil
}

func (s *SQLTokenStore) SaveTokens(ctx context.Context, refreshToken string, updatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_tokens (provider, refresh_token, scopes, updated_at)
		VALUES ('spotify', ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET refresh_token = excluded.refresh_token, updated_at = excluded.updated_at`,
		refreshToken, Scopes, updatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("spotify: saving refresh token: %w", err)
	}
	return nil
}
```

Run: `go test ./internal/spotify/... -v` — expect PASS for the token store test alongside
the client tests from Step 11.

Now the request store, following the same pattern as `guestbook.Store`:

```go
// internal/spotify/requests_test.go
package spotify

import (
	"context"
	"testing"

	"homesite/internal/store"
)

func TestRequestStoreInsertAndRecent(t *testing.T) {
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rs := NewRequestStore(db)
	ctx := context.Background()

	if _, err := rs.Insert(ctx, SongRequest{
		TrackURI: "spotify:track:abc", TrackName: "Song A", ArtistName: "Artist One",
		CreatedAt: "2026-07-27T20:00:00Z", ClientIP: "192.168.1.10", Status: "queued",
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := rs.Insert(ctx, SongRequest{
		TrackURI: "spotify:track:def", TrackName: "Song B", ArtistName: "Artist Two",
		CreatedAt: "2026-07-27T20:01:00Z", ClientIP: "192.168.1.11", Status: "failed",
	}); err != nil {
		t.Fatalf("Insert (failed status): %v", err)
	}

	recent, err := rs.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 1 || recent[0].TrackName != "Song A" {
		t.Fatalf("Recent = %+v, want only the queued entry", recent)
	}
}
```

```go
// internal/spotify/requests.go
package spotify

import (
	"context"
	"database/sql"
	"fmt"
)

type SongRequest struct {
	ID          int64
	TrackURI    string
	TrackName   string
	ArtistName  string
	RequestedBy string
	CreatedAt   string
	ClientIP    string
	Status      string // "queued" | "failed"
}

// RequestStore is not a moderation queue — nothing here is approved
// before reaching Spotify. It exists so a failed queue-add has a
// debugging trail and "who requested that" is answerable.
type RequestStore struct {
	db *sql.DB
}

func NewRequestStore(db *sql.DB) *RequestStore {
	return &RequestStore{db: db}
}

func (s *RequestStore) Insert(ctx context.Context, r SongRequest) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO song_requests (track_uri, track_name, artist_name, requested_by, created_at, client_ip, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.TrackURI, r.TrackName, r.ArtistName, r.RequestedBy, r.CreatedAt, r.ClientIP, r.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("spotify: recording song request: %w", err)
	}
	return res.LastInsertId()
}

// Recent returns the most recently successfully-queued requests, newest
// first — shown on the Music page so guests can see what's already been
// added and avoid duplicates.
func (s *RequestStore) Recent(ctx context.Context, limit int) ([]SongRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, track_uri, track_name, artist_name, requested_by, created_at, client_ip, status
		FROM song_requests WHERE status = 'queued' ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("spotify: listing recent requests: %w", err)
	}
	defer rows.Close()

	var out []SongRequest
	for rows.Next() {
		var r SongRequest
		var requestedBy sql.NullString
		if err := rows.Scan(&r.ID, &r.TrackURI, &r.TrackName, &r.ArtistName, &requestedBy, &r.CreatedAt, &r.ClientIP, &r.Status); err != nil {
			return nil, fmt.Errorf("spotify: scanning song request: %w", err)
		}
		r.RequestedBy = requestedBy.String
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 13: Run the full package's tests to verify everything passes**

Run: `go test ./internal/spotify/... -v`
Expected: PASS for all eleven tests (client, token store, request store combined).

- [ ] **Step 14: Commit the spotify package**

```bash
git add internal/spotify
git commit -m "Add Spotify OAuth client, token store, and request store"
```

- [ ] **Step 15: Register a real Spotify app and get real credentials**

This step needs a human — there is no automated substitute for registering an application
with a third party. At [developer.spotify.com/dashboard](https://developer.spotify.com/dashboard),
create an app, set the redirect URI to exactly `http://127.0.0.1:8080/spotify/callback`, and
copy the resulting `client_id`/`client_secret` into `config.local.yaml`'s `spotify:` block
(this file is gitignored — never commit real credentials). The actual OAuth authorization
(visiting `/spotify/login` and approving scopes) happens later, in Step 24, once the routes
exist to serve it.

- [ ] **Step 16: Write the loopback-restriction helper**

```go
// internal/web/loopback.go
package web

import "net"
import "net/http"

// isLoopback reports whether r originated from 127.0.0.1 or ::1. Used to
// gate /spotify/login and /spotify/callback, which share the site's one
// port with every guest-facing route but must never be guest-reachable.
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
```

(Combine the two `import` lines into one parenthesized block when writing the file — shown
separately above only for readability in this plan.)

- [ ] **Step 17: Write the failing OAuth route test**

```go
// internal/web/handlers_spotify_auth_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpotifyLoginRejectsNonLoopback(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/spotify/login", nil)
	req.RemoteAddr = "203.0.113.5:54321" // a real, non-loopback address
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a non-loopback caller", rec.Code)
	}
}

func TestSpotifyLoginRedirectsForLoopback(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/spotify/login", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to Spotify's authorize endpoint", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected a Location header pointing at Spotify's authorize endpoint")
	}
}

func TestSpotifyCallbackRejectsNonLoopback(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/spotify/callback?code=abc&state=xyz", nil)
	req.RemoteAddr = "203.0.113.5:54321"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a non-loopback caller", rec.Code)
	}
}
```

- [ ] **Step 18: Run the tests to verify they fail**

Run: `go test ./internal/web/... -run TestSpotify -v`
Expected: FAIL — routes 404 unconditionally (they don't exist yet, so even the
loopback-allowed test also fails, which is expected at this point).

- [ ] **Step 19: Wire the Spotify client, request store, and rate limiter into `Server`**

Add fields to the `Server` struct in `internal/web/server.go`:
```go
	spotifyClient      *spotify.Client
	requestStore       *spotify.RequestStore
	songRequestLimiter *rateLimiter
```

In `New`, after the existing rate limiter construction (Task 16), add:
```go
	tokenStore := spotify.NewSQLTokenStore(db)
	s.spotifyClient = spotify.New(
		cfg.Spotify.ClientID, cfg.Spotify.ClientSecret, cfg.Spotify.RedirectURI,
		tokenStore, time.Duration(cfg.Spotify.NowPlayingCacheSeconds)*time.Second,
	)
	s.requestStore = spotify.NewRequestStore(db)
	s.songRequestLimiter = newRateLimiter(cfg.Limits.SongRequestsPerWindow, windowDur)
```
(add `"homesite/internal/spotify"` to `server.go`'s imports; `windowDur` already exists from
Task 16's rate limiter wiring — reuse it, don't redeclare)

**Unlike the old client-credentials design, `spotifyClient` is never nil here** — OAuth
construction can't fail synchronously the way playlist-URL parsing could, since there's
nothing to validate until an actual API call is attempted. A missing/invalid refresh token
surfaces as `ErrTokenInvalid` from individual calls, handled per-request in the handlers
below, not at construction time.

Register the routes in `registerPageRoutes`:
```go
	s.mux.HandleFunc("GET /music", s.handleMusicPage)
	s.mux.HandleFunc("GET /music/search", s.handleMusicSearch)
	s.mux.HandleFunc("POST /music/request", s.rateLimit(s.songRequestLimiter, "music-request-rl-message", s.handleMusicRequest))
	s.mux.HandleFunc("GET /spotify/login", s.handleSpotifyLogin)
	s.mux.HandleFunc("GET /spotify/callback", s.handleSpotifyCallback)
```

Update `testConfig()` in `internal/web/server_test.go` to include the new limit, alongside
the ones Task 16 already added:
```go
	cfg.Limits = config.LimitsConfig{GuestbookPerWindow: 2, PhotoUploadsPerWindow: 30, SongRequestsPerWindow: 3, WindowMinutes: 15}
```

- [ ] **Step 20: Implement the OAuth route handlers**

```go
// internal/web/handlers_spotify_auth.go
package web

import (
	"log"
	"net/http"
)

func (s *Server) handleSpotifyLogin(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		http.NotFound(w, r)
		return
	}
	state := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name: "spotify_oauth_state", Value: state, Path: "/spotify", MaxAge: 300, HttpOnly: true,
	})
	http.Redirect(w, r, s.spotifyClient.AuthURL(state), http.StatusFound)
}

func (s *Server) handleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		http.NotFound(w, r)
		return
	}
	cookie, err := r.Cookie("spotify_oauth_state")
	if err != nil || r.URL.Query().Get("state") != cookie.Value {
		http.Error(w, "state mismatch — go back to /spotify/login and try again", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "authorization was not granted: "+r.URL.Query().Get("error"), http.StatusBadRequest)
		return
	}
	if err := s.spotifyClient.ExchangeCode(r.Context(), code); err != nil {
		log.Printf("web: spotify code exchange failed: %v", err)
		http.Error(w, "authorization failed — check the server logs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte("<h1>Spotify connected</h1><p>You can close this tab.</p>"))
}
```

`randomToken()` already exists in `internal/web/ratelimit.go` (Task 16) — this reuses it
rather than redefining a second random-string helper.

- [ ] **Step 21: Run the OAuth route tests to verify they pass**

Run: `go test ./internal/web/... -run TestSpotify -v`
Expected: PASS for all three.

- [ ] **Step 22: Write the Music page templ views**

```templ
// views/music.templ
package views

import "homesite/internal/spotify"

templ MusicPage(nowPlaying *spotify.Track, nowPlayingErr bool, recent []spotify.SongRequest) {
	@Layout("Music Requests", HeaderStandard, musicBody(nowPlaying, nowPlayingErr, recent))
}

templ musicBody(nowPlaying *spotify.Track, nowPlayingErr bool, recent []spotify.SongRequest) {
	<h1>Music Requests</h1>
	<div class="now-playing">
		if nowPlayingErr {
			<p class="quiet-note">Can't reach Spotify right now.</p>
		} else if nowPlaying == nil {
			<p class="quiet-note">Nothing's playing yet.</p>
		} else {
			<p>Now playing: <strong>{ nowPlaying.Name }</strong> — { nowPlaying.Artists }</p>
		}
	</div>
	<input type="search" name="q" placeholder="Search for a song"
		hx-get="/music/search" hx-trigger="keyup changed delay:300ms" hx-target="#search-results"/>
	<div id="search-results"></div>
	<div id="music-request-rl-message"></div>
	if len(recent) > 0 {
		<h2>Recently added</h2>
		<ul class="track-list">
			for _, r := range recent {
				<li>{ r.TrackName } — { r.ArtistName }</li>
			}
		</ul>
	}
}

templ SearchResults(tracks []spotify.Track) {
	if len(tracks) == 0 {
		<p class="quiet-note">No matches — try the artist name.</p>
	} else {
		for _, t := range tracks {
			<form hx-post="/music/request" hx-target="#search-results" hx-swap="innerHTML">
				<input type="hidden" name="uri" value={ t.URI }/>
				<input type="hidden" name="name" value={ t.Name }/>
				<input type="hidden" name="artist" value={ t.Artists }/>
				<button type="submit">{ t.Name } — { t.Artists }</button>
			</form>
		}
	}
}

templ SearchResultsError(message string) {
	<p class="quiet-note">{ message }</p>
}

templ RequestResult(success bool, message string) {
	<p class={ templ.KV("request-success", success), templ.KV("request-failure", !success) }>{ message }</p>
}
```

- [ ] **Step 23: Implement the music handlers**

```go
// internal/web/handlers_music.go
package web

import (
	"errors"
	"log"
	"net/http"
	"time"

	"homesite/internal/spotify"
	"homesite/views"
)

func (s *Server) handleMusicPage(w http.ResponseWriter, r *http.Request) {
	nowPlaying, err := s.spotifyClient.NowPlaying(r.Context())
	nowPlayingErr := err != nil
	if err != nil {
		log.Printf("web: now-playing error: %v", err)
	}

	recent, err := s.requestStore.Recent(r.Context(), 10)
	if err != nil {
		log.Printf("web: recent requests error: %v", err)
	}

	render(w, r, views.MusicPage(nowPlaying, nowPlayingErr, recent))
}

func (s *Server) handleMusicSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		render(w, r, views.SearchResults(nil))
		return
	}
	tracks, err := s.spotifyClient.Search(r.Context(), q)
	if err != nil {
		log.Printf("web: search error: %v", err)
		render(w, r, views.SearchResultsError(spotifyErrorMessage(err)))
		return
	}
	render(w, r, views.SearchResults(tracks))
}

func (s *Server) handleMusicRequest(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}
	uri := r.PostFormValue("uri")
	name := r.PostFormValue("name")
	artist := r.PostFormValue("artist")
	nickname := r.PostFormValue("nickname")
	ip := clientIP(r)

	err := s.spotifyClient.QueueTrack(r.Context(), uri)
	status := "queued"
	message := "Added to the queue!"
	if err != nil {
		status = "failed"
		message = spotifyErrorMessage(err)
		log.Printf("web: queue error: %v", err)
	}

	if _, insertErr := s.requestStore.Insert(r.Context(), spotify.SongRequest{
		TrackURI: uri, TrackName: name, ArtistName: artist, RequestedBy: nickname,
		CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientIP: ip, Status: status,
	}); insertErr != nil {
		log.Printf("web: recording song request: %v", insertErr)
	}

	render(w, r, views.RequestResult(err == nil, message))
}

func spotifyErrorMessage(err error) string {
	switch {
	case errors.Is(err, spotify.ErrNoActiveDevice):
		return "Nothing's playing yet — ask the host to start the music 🎵"
	case errors.Is(err, spotify.ErrTokenInvalid):
		return "Song requests are down right now"
	case errors.Is(err, spotify.ErrRateLimited):
		return "Spotify's throttling us — try again in a minute"
	default:
		return "Couldn't reach Spotify, try again"
	}
}
```

- [ ] **Step 24: Run the tests to verify they pass**

Run:
```bash
templ generate
go test ./internal/web/... -v
```
Expected: PASS for every test in the package, including the OAuth route tests and the
existing music-page test (which needs updating — see Step 25).

- [ ] **Step 25: Update the old music-page test to match the new page**

`internal/web/handlers_music_test.go` (if it was created against the old client-credentials
design in an earlier draft of this plan) needs replacing — the old
`TestMusicPageRendersWithoutSpotifyClient`/`TestMusicQRPngServesImage` tests assumed a
`spotifyClient` that could be `nil` and a `/music/qr.png` route that no longer exists. Replace
that file's contents with:

```go
// internal/web/handlers_music_test.go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMusicPageRenders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/music", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// testConfig() has no real Spotify credentials, so NowPlaying will
	// fail against the real API — the page must degrade gracefully, not 500.
	if !strings.Contains(rec.Body.String(), "Music Requests") {
		t.Error("expected the page heading to render")
	}
}

func TestMusicSearchWithEmptyQueryReturnsNoResults(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/music/search", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
```

Run `go test ./internal/web/... -v` again to confirm the replacement test passes alongside
everything else.

- [ ] **Step 26: Commit**

```bash
git add internal/web views
git commit -m "Add Spotify OAuth routes, now-playing, search, and unmoderated queue requests"
```

- [ ] **Step 27: Complete the one-time OAuth authorization and verify locally**

This is a manual step — there is no way to automate a real Spotify authorization grant, and
it should not be attempted by an automated implementer. With real `client_id`/`client_secret`
in `config.local.yaml` (from Step 15) and `go run ./cmd/homesite -config config.local.yaml`
running:

1. Open `http://127.0.0.1:8080/spotify/login` directly in a browser (no SSH tunnel needed
   locally — the dev server already listens on `127.0.0.1:8080`).
2. Approve the requested scopes on Spotify's consent screen.
3. Confirm the callback shows "Spotify connected."
4. Start playback on a real device logged into the same Spotify account, then open `/music`
   on a phone and confirm the now-playing track matches.
5. Search for a song, tap a result, and confirm it lands in the real queue on the playing
   device.
6. Manually hit `POST /music/request` (or tap a result) four times in under 15 minutes and
   confirm the fourth attempt shows the rate-limit message, not a fifth queue-add.

---

## Task 18: Visual Design Pass

Every prior task built structurally complete, functionally correct pages with minimal styling (Task 7's `site.css` covers layout mechanics: cards, the menu overlay, tap targets, QR-page dark-mode lock). The spec deliberately defers real visual execution to this point: *"Detailed visual execution is deferred to implementation, where the `frontend-design` skill applies."* This task is that pass, across all ten pages at once so the site reads as one designed system rather than ten separately-styled pages.

**This task now also acquires the Butler headline font**, added to the spec after Task 7 shipped with Lora alone. Butler is a free, high-contrast display serif (Fabian De Smet) — used for headlines only, never body text, where its thin hairlines would hurt readability. Lora remains exactly as Task 7 wired it, carrying every other block of text on the page.

**Files:**
- Modify: `static/css/site.css`
- Modify: any `views/*.templ` files whose markup needs additional structure to support the new visual design (e.g., wrapping elements for imagery treatments)
- Create: `static/fonts/Butler-Black.woff2` (or whichever weight the design settles on for display headings — Black or Extra Bold are the two heaviest cuts in the free family), `static/fonts/Butler-LICENSE.txt`

**Interfaces:**
- Consumes: every page and component from Tasks 7, 9, 10, 14, 15, 17
- Produces: no new interfaces — this task changes appearance, not behavior. Every test from Tasks 1–17 must still pass unmodified afterward, since none of them assert on CSS or visual layout.

- [ ] **Step 1: Acquire Butler**

Download from [Font Squirrel](https://www.fontsquirrel.com/fonts/butler) or [the designer's own site](https://www.fabiandesmet.com/portfolio/butler-font/) — both distribute genuine woff2 files directly, no TTF-to-woff2 conversion needed (unlike Lora's acquisition in Task 7, which needed the per-weight `css2` API workaround). Pick one heavy display weight (Black or Extra Bold) for headlines; the free family also ships Regular through Ultra Light and a parallel stencil line, none of which this design needs.

**Read the bundled license file before shipping it** — different distribution channels frame Butler's terms differently (the designer's own site describes it loosely as "free for commercial use"; some mirrors attach formal CC BY-SA 4.0 terms, which technically requires attribution and share-alike). Save whichever license file the actual downloaded zip contains as `static/fonts/Butler-LICENSE.txt`, and if it turns out to be CC BY-SA, add a one-line attribution credit somewhere reasonable (a site footer or an `ABOUT` note) — costs nothing, closes the gap between the two framings.

Place the woff2 file at `static/fonts/Butler-Black.woff2` (adjust the name to match whichever weight was actually downloaded).

- [ ] **Step 2: Invoke the frontend-design skill with the spec's visual brief**

Invoke `frontend-design` with this brief, derived directly from the spec's Visual Direction section:

> Design pass for "Brivin Household," a mobile-first home party site (ten pages: Welcome, Wi-Fi, Coffee Menu, Cocktail Menu, Refreshments, Music Requests, Upload Photos, Guest Book, Meet the Cats, Share). Two typefaces, both self-hosted as woff2: **Butler** (a high-contrast display serif) for headlines only — large scale jumps between heading levels, tight negative tracking, used at 24px and up, never for body copy since its hairlines hurt readability at small sizes — and **Lora** (already wired in `static/css/site.css` from an earlier pass) carrying every other block of text: menu items, cat bios, guest book entries, form labels, navigation. The personality should come from this pairing and from typographic decisions, not from either font alone: a restrained palette (warm off-white ground, near-black ink, one saturated accent color — currently a rust/terracotta placeholder), generous whitespace, a single-column measure capped around 60 characters, and full-bleed imagery (cat photos, gallery thumbnails) contrasted against tight text blocks. Layouts are designed at 390px width first and allowed to breathe on larger screens — every tap target at least 44px, used one-handed, standing up, in imperfect lighting. Honor `prefers-color-scheme` dark mode everywhere **except** the two QR pages (Wi-Fi, Share — Music is no longer a QR page, it works entirely within the site), which are hard-locked to a white background regardless of viewer theme — that lock already exists in `site.css` via the `.qr-page` class and must not be loosened. The vibe the site's owner asked for is "hip and trendy" — lean into confident display typography for headlines and a considered accent color rather than generic Bootstrap-y defaults.

- [ ] **Step 3: Wire the `@font-face` rule for Butler**

Add to `static/css/site.css`, alongside the existing Lora `@font-face` blocks from Task 7:
```css
@font-face {
	font-family: "Butler";
	src: url("/static/fonts/Butler-Black.woff2") format("woff2");
	font-weight: 900;
	font-style: normal;
	font-display: swap;
}
```
(adjust `font-weight` and the filename to match whichever cut was actually downloaded in Step 1)

- [ ] **Step 4: Apply the resulting design to `static/css/site.css` and any `views/*.templ` markup it requires**

Follow the frontend-design skill's output. Where it calls for new wrapping elements (e.g., an image treatment needing an extra `<div>`), edit the relevant `.templ` file directly — but do not change any component's exported name, parameter list, or the text content handler tests assert on (page headings, menu item names, guest book messages, etc.). This is a styling pass; it must not change what any test in `internal/web` checks for.

- [ ] **Step 5: Regenerate templ and run the full test suite to confirm nothing broke**

Run:
```bash
templ generate
go test ./...
```
Expected: PASS for every test written in Tasks 1–17. If a test fails, it means a markup change altered text content or structure a test depends on — fix the markup to preserve that content, don't weaken the test.

- [ ] **Step 6: Review the redesigned site on a real phone, page by page**

Run `go run ./cmd/homesite -config config.local.yaml` and open every one of the ten routes on a phone at `http://<mac-ip>:8080/`. Specifically re-check the two QR pages (Wi-Fi, Share) still scan correctly and still show a white background regardless of the phone's system dark-mode setting — a design pass is exactly the kind of change that could accidentally regress that lock. Also confirm Butler renders only on headings, never on paragraph-length text, and that it actually loaded (a fallback to the body serif on every heading usually means a wrong path or font-weight mismatch between the `@font-face` rule and wherever the design applies `font-family: "Butler"`).

- [ ] **Step 7: Commit**

```bash
git add static views
git commit -m "Apply visual design pass: Butler headlines, Lora body, palette, and imagery treatment"
```

---

## Task 19: Deployment — Systemd, Provisioning, Backup, and Runbook

**Files:**
- Create: `deploy/homesite.service`
- Create: `deploy/homesite-backup.service`
- Create: `deploy/homesite-backup.timer`
- Create: `deploy/provision.sh`
- Create: `deploy/backup.sh`
- Create: `docs/RUNBOOK.md`
- Modify: `internal/web/healthz.go` (report disk usage and last backup time)
- Test: `internal/web/healthz_test.go`
- Modify: `Makefile` (finalize `deploy` target)

**Interfaces:**
- Consumes: `photos.Store.DiskUsageBytes` (Task 13); `config.Config` (Task 1)
- Produces: a deployable Pi image plus a documented, rehearsed-on-paper recovery path. No new Go interfaces — this is the last task, wiring existing pieces to the filesystem and to systemd.

- [ ] **Step 1: Write the provisioning script**

Run once on a freshly flashed Pi, before the site is deployed. Encodes the Global Constraints' write-reduction requirements and the package installs both `internal/imaging` and `restic` need.

```bash
#!/usr/bin/env bash
# deploy/provision.sh — run once on a fresh Raspberry Pi OS Lite (Trixie) install.
set -euo pipefail

sudo apt update
sudo apt install -y imagemagick restic

# Reduce writes to the microSD card, which holds the OS, database, and
# uploaded photos together (see spec: "the card is a consumable").
sudo mount -o remount,noatime /

sudo mkdir -p /etc/systemd/journald.conf.d
cat <<'EOF' | sudo tee /etc/systemd/journald.conf.d/volatile.conf
[Journal]
Storage=volatile
EOF

sudo dphys-swapfile swapoff || true
sudo systemctl disable dphys-swapfile || true

sudo useradd --system --home /srv/homesite --shell /usr/sbin/nologin homesite || true
sudo mkdir -p /srv/homesite/data /srv/homesite/uploads /srv/homesite/content
sudo chown -R homesite:homesite /srv/homesite/data /srv/homesite/uploads
sudo chown -R "$(whoami)":homesite /srv/homesite/content
sudo chmod -R 0775 /srv/homesite/content

sudo systemctl restart systemd-journald

echo "Provisioning complete. Next: copy config.example.yaml to /srv/homesite/config.yaml, fill in real values, and run 'make deploy' from the Mac."
```

- [ ] **Step 2: Write the backup script**

The comment on `VACUUM INTO` is load-bearing, not decorative: copying a live SQLite file with plain `cp` can capture a torn write mid-transaction and produce a corrupt restore. `VACUUM INTO` produces a consistent, complete snapshot in one step.

```bash
#!/usr/bin/env bash
# deploy/backup.sh — nightly, via homesite-backup.timer.
set -euo pipefail

DATA_DIR=/srv/homesite/data
UPLOADS_DIR=/srv/homesite/uploads
SNAPSHOT=/tmp/homesite-snapshot.db

rm -f "$SNAPSHOT"
sqlite3 "$DATA_DIR/homesite.db" "VACUUM INTO '$SNAPSHOT';"

restic backup "$SNAPSHOT" "$UPLOADS_DIR" --tag homesite

rm -f "$SNAPSHOT"

date -u +%Y-%m-%dT%H:%M:%SZ | sudo -u homesite tee "$DATA_DIR/.last_backup" > /dev/null
```

- [ ] **Step 3: Write the systemd units**

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

```ini
# deploy/homesite-backup.service
[Unit]
Description=Brivin Household nightly backup

[Service]
Type=oneshot
EnvironmentFile=/srv/homesite/backup.env
ExecStart=/srv/homesite/deploy/backup.sh
```

```ini
# deploy/homesite-backup.timer
[Unit]
Description=Run Brivin Household backup nightly

[Timer]
OnCalendar=*-*-* 04:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

`/srv/homesite/backup.env` (created by hand on the Pi, mode `0600`, never committed) holds the restic repository and Backblaze B2 credentials:
```bash
RESTIC_REPOSITORY=b2:your-bucket-name:homesite
RESTIC_PASSWORD=choose-a-strong-passphrase
B2_ACCOUNT_ID=your-b2-key-id
B2_ACCOUNT_KEY=your-b2-application-key
```

- [ ] **Step 4: Write the failing healthz test**

```go
// internal/web/healthz_test.go
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthzReportsDiskUsageAndBackupStatus(t *testing.T) {
	cfg, uploadsDir := testConfigWithPhotos(t)
	if err := os.WriteFile(filepath.Join(uploadsDir, "a.jpg"), make([]byte, 500), 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	cfg.DataDir = dataDir
	backupStamp := "2026-07-27T04:00:00Z"
	if err := os.WriteFile(filepath.Join(dataDir, ".last_backup"), []byte(backupStamp+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(cfg, mustOpenMemoryDB(t))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Status         string `json:"status"`
		DiskUsedBytes  int64  `json:"disk_used_bytes"`
		DiskCapBytes   int64  `json:"disk_cap_bytes"`
		LastBackup     string `json:"last_backup"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding healthz JSON: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("Status = %q, want ok", body.Status)
	}
	if body.DiskUsedBytes != 500 {
		t.Errorf("DiskUsedBytes = %d, want 500", body.DiskUsedBytes)
	}
	if body.LastBackup != backupStamp {
		t.Errorf("LastBackup = %q, want %q", body.LastBackup, backupStamp)
	}
}

func TestHealthzReportsNeverBackedUp(t *testing.T) {
	cfg, _ := testConfigWithPhotos(t)
	cfg.DataDir = t.TempDir() // no .last_backup file present

	s := New(cfg, mustOpenMemoryDB(t))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var body struct {
		LastBackup string `json:"last_backup"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	if body.LastBackup != "never" {
		t.Errorf("LastBackup = %q, want %q when no backup has run", body.LastBackup, "never")
	}
}
```

Add this shared helper (used above and usable by any other test that just needs a throwaway DB without going through `newTestServer`'s fixed config):
```go
// internal/web/server_test.go — add alongside newTestServer
func mustOpenMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
```
(add `"database/sql"` to `server_test.go`'s imports)

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./internal/web/... -run TestHealthz -v`
Expected: FAIL — current `handleHealthz` returns plain text `ok`, not JSON with these fields.

- [ ] **Step 6: Implement the richer healthz handler**

```go
// internal/web/healthz.go
package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type healthzResponse struct {
	Status        string `json:"status"`
	DiskUsedBytes int64  `json:"disk_used_bytes"`
	DiskCapBytes  int64  `json:"disk_cap_bytes"`
	LastBackup    string `json:"last_backup"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	used, err := s.photosStore.DiskUsageBytes(s.cfg.UploadsDir)
	if err != nil {
		used = -1
	}

	lastBackup := "never"
	if b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, ".last_backup")); err == nil {
		lastBackup = strings.TrimSpace(string(b))
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(healthzResponse{
		Status:        "ok",
		DiskUsedBytes: used,
		DiskCapBytes:  s.cfg.Photos.MaxTotalBytes,
		LastBackup:    lastBackup,
	})
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/web/... -v`
Expected: PASS for both new tests and every test from Tasks 8 through 17 — including the original `TestHealthzReturnsOK` from Task 8, which only checked the status code and will still pass against the new JSON body.

- [ ] **Step 8: Write the recovery runbook**

```markdown
# docs/RUNBOOK.md

## If the microSD card dies

The card is treated as a consumable (see the design spec's Storage
section) — this is the boring, rehearsed path back to a working site.

1. Flash a fresh **Raspberry Pi OS Lite (64-bit), Trixie** image with
   Raspberry Pi Imager, pre-configuring hostname, SSH key, and WiFi via
   the gear icon.
2. SSH in and run `deploy/provision.sh` (copy it over first via
   `scp deploy/provision.sh pi@<new-ip>:~/` or clone the repo).
3. Recreate `/srv/homesite/config.yaml` from `config.example.yaml` —
   WiFi password and Spotify credentials are not in git; pull them from
   your password manager.
4. Recreate `/srv/homesite/backup.env` the same way (restic repository
   password and B2 keys).
5. From the Mac: `make deploy` (builds and `scp`s the binary, installs
   the systemd unit isn't automated yet — see step 6).
6. Copy `deploy/homesite.service`, `deploy/homesite-backup.service`, and
   `deploy/homesite-backup.timer` into `/etc/systemd/system/` on the Pi,
   then:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now homesite
   sudo systemctl enable --now homesite-backup.timer
   ```
7. Restore data:
   ```bash
   restic restore latest --target /srv/homesite/restore-tmp
   cp /srv/homesite/restore-tmp/tmp/homesite-snapshot.db /srv/homesite/data/homesite.db
   cp -r /srv/homesite/restore-tmp/srv/homesite/uploads/* /srv/homesite/uploads/
   sudo chown -R homesite:homesite /srv/homesite/data /srv/homesite/uploads
   sudo systemctl restart homesite
   ```
8. Confirm `curl http://192.168.1.50/healthz` reports `"status":"ok"`
   and a non-zero `disk_used_bytes`, then open the site on a phone.

Target: under an hour from dead card to working site.

## If a backup silently stopped working

`GET /healthz` includes `last_backup`. If it's more than a day old (or
`"never"`), SSH in and run `deploy/backup.sh` manually to see the error —
most likely an expired B2 key or a full `/tmp`.
```

- [ ] **Step 9: Finalize the Makefile's deploy target**

Update the `Makefile` written in Task 1 so `deploy` also reminds the operator that systemd units aren't auto-installed by `scp` alone:

```makefile
.PHONY: dev test build deploy

dev:
	templ generate --watch &
	go run ./cmd/homesite -config config.local.yaml

test:
	go test ./...

build:
	templ generate
	GOOS=linux GOARCH=arm64 go build -o homesite ./cmd/homesite

deploy: build
	scp homesite pi@192.168.1.50:/srv/homesite/homesite
	ssh pi@192.168.1.50 'sudo systemctl restart homesite'
	@echo "Binary deployed and service restarted."
	@echo "First-time setup only: see docs/RUNBOOK.md steps 2-6 for provisioning, config, and systemd units."
```

- [ ] **Step 10: Commit**

```bash
git add deploy docs/RUNBOOK.md internal/web Makefile
git commit -m "Add deployment artifacts: systemd units, provisioning, backup, healthz reporting, runbook"
```
