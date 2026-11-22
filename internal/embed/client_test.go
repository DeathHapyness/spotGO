package embed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/fetch"
)

const missingID = "0000000000000000000000"

type pathLog struct {
	mu    sync.Mutex
	paths []string
}

func (l *pathLog) add(p string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = append(l.paths, p)
}

func (l *pathLog) first() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.paths) == 0 {
		return ""
	}
	return l.paths[0]
}

func newTestServer(t *testing.T) (*httptest.Server, *atomic.Int32, *pathLog) {
	t.Helper()
	var calls atomic.Int32
	paths := &pathLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		paths.add(r.URL.Path)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 2 || parts[1] == missingID {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join("testdata", parts[0]+".html"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, paths
}

func newTestClient(srv *httptest.Server, o fetch.Options) *Client {
	c := New(fetch.New(o))
	c.base = srv.URL
	return c
}

func TestClientTrack(t *testing.T) {
	srv, _, paths := newTestServer(t)
	c := newTestClient(srv, fetch.Options{})

	s, err := c.Track(context.Background(), "4HCOHRCSQelz2GPX8iTamO")
	if err != nil {
		t.Fatal(err)
	}
	if paths.first() != "/track/4HCOHRCSQelz2GPX8iTamO" || s.Artists[0] != "Justice" {
		t.Fatalf("path=%v song=%+v", paths.first(), s)
	}
}

func TestClientAlbum(t *testing.T) {
	srv, _, paths := newTestServer(t)
	c := newTestClient(srv, fetch.Options{})

	a, err := c.Album(context.Background(), "0ETFjACtuP2ADo6LFhL6HN")
	if err != nil {
		t.Fatal(err)
	}
	if paths.first() != "/album/0ETFjACtuP2ADo6LFhL6HN" || a.Name != "Abbey Road (Remastered 2009)" {
		t.Fatalf("path=%v album=%q", paths.first(), a.Name)
	}
}

func TestClientPlaylist(t *testing.T) {
	srv, _, paths := newTestServer(t)
	c := newTestClient(srv, fetch.Options{})

	p, err := c.Playlist(context.Background(), "37i9dQZF1DXcBWIGoYBM5M")
	if err != nil {
		t.Fatal(err)
	}
	if paths.first() != "/playlist/37i9dQZF1DXcBWIGoYBM5M" || p.Name != "Today’s Top Hits" {
		t.Fatalf("path=%v playlist=%q", paths.first(), p.Name)
	}
}

func TestClientArtist(t *testing.T) {
	srv, _, paths := newTestServer(t)
	c := newTestClient(srv, fetch.Options{})

	a, err := c.Artist(context.Background(), "3WrFJ7ztbogyGnTHbHJFl2")
	if err != nil {
		t.Fatal(err)
	}
	if paths.first() != "/artist/3WrFJ7ztbogyGnTHbHJFl2" || a.Name != "The Beatles" {
		t.Fatalf("path=%v artist=%q", paths.first(), a.Name)
	}
}

func TestClientNotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)
	c := newTestClient(srv, fetch.Options{})

	_, err := c.Track(context.Background(), missingID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("quero ErrNotFound, veio %v", err)
	}
	if !strings.Contains(err.Error(), "open.spotify.com/track/"+missingID) {
		t.Fatalf("mensagem deveria citar o link público: %q", err)
	}
}

func TestClientUsesCache(t *testing.T) {
	srv, calls, _ := newTestServer(t)
	c := newTestClient(srv, fetch.Options{CacheDir: t.TempDir()})

	for i := 0; i < 2; i++ {
		if _, err := c.Album(context.Background(), "0ETFjACtuP2ADo6LFhL6HN"); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("chamadas = %d, quero 1 (segunda vem do cache)", calls.Load())
	}
}
