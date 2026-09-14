//go:build ignore

// Command vendor-libhegel vendors a hegel-rust release into the repo.
//
// The pre-compiled libhegel artifacts are checked into internal/libhegel/libs
// (stored via git-lfs) and go:embed'd into the client at build time, so there
// is no runtime download. This program queries a hegel-rust GitHub release (the
// latest by default, or the one named by -version), downloads each
// client-requestable artifact (one per supported GOOS/GOARCH, i.e. the
// .so/.dylib/.dll assets) into the libs dir — verifying each against the
// SHA-256 digest GitHub records — and rewrites internal/libhegel/version.go to
// pin the resolved version. Run it to bump the vendored release, then commit
// the result (with git-lfs installed).
//
// Usage:
//
//	go run scripts/vendor-libhegel.go [-version <v>]
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const repo = "hegeldev/hegel-rust"

// libsDir and versionFile are written relative to the current working
// directory, which `just vendor-libhegel` sets to the repo root.
const (
	libsDir     = "internal/libhegel/libs"
	versionFile = "internal/libhegel/version.go"
)

// clientAssetRE matches the assets a client can actually request: a .so
// (Linux/other), .dylib (darwin), or .dll (windows) name. The .sha256 sidecar
// assets are excluded — GitHub records a digest for every asset, which we use
// directly.
var clientAssetRE = regexp.MustCompile(`^libhegel-[a-z0-9]+-[a-z0-9]+\.(so|dylib|dll)$`)

type asset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []asset   `json:"assets"`
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}
var releaseTagRE = regexp.MustCompile(`^(libhegel-v|v)[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

func (r githubRelease) version() string {
	return strings.TrimPrefix(strings.TrimPrefix(r.TagName, "libhegel-"), "v")
}

func (r githubRelease) clientAssets() []asset {
	var assets []asset
	for _, a := range r.Assets {
		if clientAssetRE.MatchString(a.Name) {
			assets = append(assets, a)
		}
	}
	return assets
}

// get uses public endpoints without requiring gh authentication. A CI token is
// optional and is only sent to the GitHub API, never to asset download hosts.
func get(endpoint string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "hegel-go-vendor-libhegel")
	if req.URL.Host == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return httpClient.Do(req)
}

func releaseJSON(path string, dest any) (int, error) {
	resp, err := get("https://api.github.com/repos/" + repo + path)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("GitHub release API: %s", resp.Status)
	}
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(dest)
}

// release resolves an exact new/legacy tag, or tries both tag conventions for
// a bare version. Latest scans releases because /releases/latest can refer to
// an independently versioned native package instead of libhegel.
func release(want string) (githubRelease, error) {
	if want != "" {
		tags := []string{want}
		if !strings.HasPrefix(want, "v") && !strings.HasPrefix(want, "libhegel-") {
			tags = []string{"libhegel-v" + want, "v" + want}
		}
		for _, tag := range tags {
			if !releaseTagRE.MatchString(tag) {
				return githubRelease{}, fmt.Errorf("invalid libhegel release tag %q", tag)
			}
			var rel githubRelease
			status, err := releaseJSON("/releases/tags/"+url.PathEscape(tag), &rel)
			if status == http.StatusNotFound {
				continue
			}
			if err != nil {
				return githubRelease{}, err
			}
			if rel.TagName != tag {
				return githubRelease{}, fmt.Errorf("requested tag %q, got %q", tag, rel.TagName)
			}
			return rel, nil
		}
		return githubRelease{}, fmt.Errorf("libhegel release %q not found", want)
	}
	var latest githubRelease
	// GitHub lists releases by creation time, which can differ from publication
	// order when an older draft is published later. Inspect all pages.
	for page := 1; ; page++ {
		var releases []githubRelease
		_, err := releaseJSON(fmt.Sprintf("/releases?per_page=100&page=%d", page), &releases)
		if err != nil {
			return githubRelease{}, err
		}
		for _, rel := range releases {
			if !rel.Draft && !rel.Prerelease && releaseTagRE.MatchString(rel.TagName) && len(rel.clientAssets()) > 0 {
				if latest.TagName == "" || rel.PublishedAt.After(latest.PublishedAt) {
					latest = rel
				}
			}
		}
		if len(releases) < 100 {
			if latest.TagName != "" {
				return latest, nil
			}
			return githubRelease{}, fmt.Errorf("no stable libhegel release found")
		}
	}
}

// download verifies the GitHub SHA-256 digest before replacing a vendored file.
func download(a asset) error {
	wantHex, ok := strings.CutPrefix(a.Digest, "sha256:")
	digest, err := hex.DecodeString(wantHex)
	if !ok || err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("asset %s: expected a SHA-256 digest, got %q", a.Name, a.Digest)
	}
	resp, err := get(a.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", a.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", a.Name, resp.Status)
	}
	f, err := os.CreateTemp(libsDir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		return err
	}
	gotHex := hex.EncodeToString(h.Sum(nil))
	if gotHex != strings.ToLower(wantHex) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", a.Name, gotHex, wantHex)
	}
	if err := f.Chmod(0o644); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(libsDir, a.Name))
}

// versionTemplate is the generated version.go file.
const versionTemplate = `// Code generated by scripts/vendor-libhegel.go; DO NOT EDIT.
//
// Run ` + "`just vendor-libhegel`" + ` to vendor the latest hegel-rust release (this
// constant plus the binaries under libs/), then commit the result.

package libhegel

// hegelVersion is the libhegel version vendored into this client. The binaries
// embedded from libs/ are built from this hegel-rust release, and
// [TestLoadLibVersion] asserts that the loaded library reports this version.
const hegelVersion = %q
`

func run() error {
	// An empty -version selects the latest release; otherwise the named release
	// (bare version or exact new/legacy tag) is pinned. The repository_dispatch bump workflow
	// passes the exact released version so a later release can't race in.
	want := flag.String("version", "", "release version to vendor (empty selects the latest)")
	flag.Parse()

	rel, err := release(*want)
	if err != nil {
		return err
	}
	version, assets := rel.version(), rel.clientAssets()
	fmt.Fprintf(os.Stderr, "libhegel release: %s\n", rel.TagName)
	if len(assets) == 0 {
		return fmt.Errorf("no .so/.dylib/.dll assets found in release %s", rel.TagName)
	}

	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		return err
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	wanted := make(map[string]struct{}, len(assets))
	for _, a := range assets {
		wanted[a.Name] = struct{}{}
		if err := download(a); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "vendored %s\n", a.Name)
	}
	entries, err := os.ReadDir(libsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !clientAssetRE.MatchString(name) {
			continue
		}
		if _, ok := wanted[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(libsDir, name)); err != nil {
			return fmt.Errorf("remove stale asset %s: %w", name, err)
		}
		fmt.Fprintf(os.Stderr, "removed stale %s\n", name)
	}

	if err := os.WriteFile(versionFile, []byte(fmt.Sprintf(versionTemplate, version)), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %d artifact(s) to %s and pinned %s\n", len(assets), libsDir, versionFile)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
