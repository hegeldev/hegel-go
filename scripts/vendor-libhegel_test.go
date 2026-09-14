//go:build ignore

package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockHTTP(t *testing.T, fn func(*http.Request) (int, string)) {
	t.Helper()
	old := httpClient
	t.Cleanup(func() { httpClient = old })
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status, body := fn(r)
		return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestLatestLibhegel(t *testing.T) {
	mockHTTP(t, func(r *http.Request) (int, string) {
		return 200, `[
   {"tag_name":"native-v9.0.0"},
   {"tag_name":"v9.0.0","assets":[{"name":"native.tar.gz"}]},
   {"tag_name":"libhegel-v0.43.0","prerelease":true,"assets":[{"name":"libhegel-linux-amd64.so"}]},
   {"tag_name":"v0.42.0","published_at":"2026-09-14T15:02:20Z","assets":[{"name":"libhegel-linux-amd64.so"}]},
   {"tag_name":"libhegel-v0.42.1","published_at":"2026-09-14T16:13:20Z","assets":[{"name":"libhegel-linux-amd64.so"}]},
   {"tag_name":"v0.37.1","assets":[{"name":"libhegel-linux-amd64.so"}]}
  ]`
	})
	rel, err := release("")
	if err != nil || rel.version() != "0.42.1" {
		t.Fatalf("release = %+v, %v", rel, err)
	}
}

func TestExactAndBareTags(t *testing.T) {
	for _, want := range []string{"libhegel-v0.42.1", "v0.37.1", "0.42.1", "0.37.1"} {
		t.Run(want, func(t *testing.T) {
			mockHTTP(t, func(r *http.Request) (int, string) {
				tag := filepath.Base(r.URL.Path)
				if tag != "libhegel-v0.42.1" && tag != "v0.37.1" {
					return 404, ""
				}
				return 200, fmt.Sprintf(`{"tag_name":%q}`, tag)
			})
			rel, err := release(want)
			if err != nil {
				t.Fatal(err)
			}
			expected := strings.TrimPrefix(strings.TrimPrefix(want, "libhegel-"), "v")
			if rel.version() != expected {
				t.Fatalf("got %s, want %s", rel.version(), expected)
			}
		})
	}
}

func TestDownloadChecksumBeforeReplace(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "libhegel-linux-amd64.so"
	dest := filepath.Join(libsDir, name)
	if err := os.WriteFile(dest, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_TOKEN", "test-secret")
	mockHTTP(t, func(r *http.Request) (int, string) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("token sent to asset host")
		}
		return 200, "downloaded"
	})
	a := asset{Name: name, URL: "https://github.com/asset", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("different")))}
	if err := download(a); err == nil {
		t.Fatal("accepted corrupt asset")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "existing" {
		t.Fatalf("existing file changed: %q, %v", got, err)
	}
	a.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("downloaded")))
	if err := download(a); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(dest)
	if err != nil || string(got) != "downloaded" {
		t.Fatalf("download not installed: %q, %v", got, err)
	}
}
