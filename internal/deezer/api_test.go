package deezer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtureRoutes liga cada caminho da API à fixture salva.
var fixtureRoutes = map[string]string{
	"/search":                "search_track.json",
	"/search/album":          "search_album.json",
	"/search/artist":         "search_artist.json",
	"/track/116348452":       "track.json",
	"/album/12047952":        "album.json",
	"/album/12047952/tracks": "album_tracks.json",
	"/artist/1/albums":       "artist_albums.json",
}

type recorded struct {
	mu    sync.Mutex
	reqs  []*http.Request
	sleep []time.Duration
}

func (r *recorded) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
}

func newTestClient(t *testing.T, h http.Handler, cacheDir string) (*Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	f := fetch.New(fetch.Options{
		CacheDir: cacheDir,
		Sleep: func(_ context.Context, d time.Duration) error {
			rec.mu.Lock()
			rec.sleep = append(rec.sleep, d)
			rec.mu.Unlock()
			return nil
		},
	})
	c := New(f)
	c.base = srv.URL
	return c, rec
}

func fixtureServer(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := fixtureRoutes[r.URL.Path]
		if !ok {
			w.Write(fixture(t, "error_800.json"))
			return
		}
		w.Write(fixture(t, name))
	})
}

func sequence(t *testing.T, names ...string) http.Handler {
	var n atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(names) {
			i = len(names) - 1
		}
		w.Write(fixture(t, names[i]))
	})
}

func TestSearchTracksFixture(t *testing.T) {
	c, rec := newTestClient(t, fixtureServer(t), "")
	got, err := c.searchTracks(context.Background(), "The Beatles Come Together")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("len = %d, quero 10", len(got))
	}
	if got[0].TitleShort != "Come Together" || got[0].Album.Title != "Abbey Road (Remastered 2009)" || got[0].Duration != 258 {
		t.Fatalf("[0] = %+v", got[0])
	}
	if got[0].Artist.Name != "The Beatles" {
		t.Fatalf("[0].Artist = %+v", got[0].Artist)
	}
	q := rec.reqs[0].URL.Query()
	if q.Get("q") != "The Beatles Come Together" || q.Get("limit") != "10" {
		t.Fatalf("query = %v", q)
	}
}

func TestTrackFixture(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	tr, err := c.track(context.Background(), 116348452)
	if err != nil {
		t.Fatal(err)
	}
	if tr.ISRC != "GBAYE0601690" || tr.TrackPosition != 1 || tr.DiskNumber != 1 {
		t.Fatalf("track = %+v", tr)
	}
	if !strings.HasSuffix(tr.Album.CoverXL, "1000x1000-000000-80-0-0.jpg") {
		t.Fatalf("CoverXL = %q", tr.Album.CoverXL)
	}
}

func TestSearchAlbumsAndArtists(t *testing.T) {
	c, rec := newTestClient(t, fixtureServer(t), "")
	albums, err := c.searchAlbums(context.Background(), "The Beatles Abbey Road")
	if err != nil || len(albums) != 10 || albums[0].ID != 12047952 || albums[0].NbTracks != 17 {
		t.Fatalf("searchAlbums = %+v, %v", albums, err)
	}
	artists, err := c.searchArtists(context.Background(), "The Beatles")
	if err != nil || len(artists) != 5 || artists[0].ID != 1 || artists[0].NbFan == 0 {
		t.Fatalf("searchArtists = %+v, %v", artists, err)
	}
	if l := rec.reqs[1].URL.Query().Get("limit"); l != "5" {
		t.Fatalf("limit de artistas = %q, quero 5", l)
	}
	al, err := c.album(context.Background(), 12047952)
	if err != nil || al.Title != "Abbey Road (Remastered 2009)" || al.Artist.Name != "The Beatles" {
		t.Fatalf("album = %+v, %v", al, err)
	}
	disc, err := c.artistAlbums(context.Background(), 1)
	if err != nil || len(disc) != 6 || disc[0].RecordType != "album" {
		t.Fatalf("artistAlbums = %+v, %v", disc, err)
	}
}

func TestAlbumTracksPaginates(t *testing.T) {
	const total = 250
	var indexes []string
	var mu sync.Mutex
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		indexes = append(indexes, q.Get("index"))
		mu.Unlock()
		idx, _ := strconv.Atoi(q.Get("index"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		var items []string
		for i := idx; i < idx+limit && i < total; i++ {
			items = append(items, fmt.Sprintf(`{"id":%d,"title":"t%d","track_position":%d}`, i+1, i, i+1))
		}
		fmt.Fprintf(w, `{"data":[%s],"total":%d,"next":"http://outro-host/nao-seguir"}`, strings.Join(items, ","), total)
	})
	c, _ := newTestClient(t, h, "")

	got, err := c.albumTracks(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != total || got[249].TrackPosition != 250 {
		t.Fatalf("len = %d", len(got))
	}
	if strings.Join(indexes, ",") != "0,100,200" {
		t.Fatalf("indexes = %v, quero 0,100,200", indexes)
	}
}

func TestNotFound(t *testing.T) {
	c, _ := newTestClient(t, sequence(t, "error_800.json"), "")
	if _, err := c.track(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quero ErrNotFound", err)
	}
}

func TestQuotaRetried(t *testing.T) {
	c, rec := newTestClient(t, sequence(t, "error_4.json", "track.json"), "")
	tr, err := c.track(context.Background(), 116348452)
	if err != nil || tr.ISRC != "GBAYE0601690" {
		t.Fatalf("track = %+v, %v", tr, err)
	}
	if len(rec.sleep) != 1 || rec.sleep[0] != 5*time.Second {
		t.Fatalf("sleeps = %v, quero [5s]", rec.sleep)
	}
}

func TestOtherErrorCode(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"error":{"type":"ParameterException","message":"Wrong parameter","code":501}}`))
	})
	c, _ := newTestClient(t, h, "")
	_, err := c.track(context.Background(), 1)
	if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "Wrong parameter") {
		t.Fatalf("err = %v", err)
	}
}

func TestErrorsNotCached(t *testing.T) {
	c, rec := newTestClient(t, sequence(t, "error_800.json", "track.json"), t.TempDir())
	if _, err := c.track(context.Background(), 116348452); !errors.Is(err, ErrNotFound) {
		t.Fatalf("primeira: err = %v", err)
	}
	tr, err := c.track(context.Background(), 116348452)
	if err != nil || tr.ISRC != "GBAYE0601690" {
		t.Fatalf("segunda: %+v, %v", tr, err)
	}
	if len(rec.reqs) != 2 {
		t.Fatalf("requisições = %d, quero 2", len(rec.reqs))
	}
}
