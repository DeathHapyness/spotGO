package deezer

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

func TestEnrichTrack(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	src := song.Song{
		Title:     "Come Together - Remastered 2009",
		Artists:   []string{"The Beatles"},
		Duration:  259946 * time.Millisecond,
		SpotifyID: "x",
	}

	got, ok, err := c.EnrichTrack(context.Background(), src)
	if err != nil || !ok {
		t.Fatalf("EnrichTrack: ok=%v err=%v", ok, err)
	}
	if got.ISRC != "GBAYE0601690" || got.TrackNumber != 1 || got.DiscNumber != 1 {
		t.Fatalf("números = %+v", got)
	}
	if got.Album != "Abbey Road (Remastered 2009)" || got.AlbumArtist != "The Beatles" {
		t.Fatalf("álbum = %q / %q", got.Album, got.AlbumArtist)
	}
	if !strings.HasSuffix(got.CoverURL, "1000x1000-000000-80-0-0.jpg") {
		t.Fatalf("CoverURL = %q", got.CoverURL)
	}
	if got.ReleaseDate == "" {
		t.Fatal("ReleaseDate vazio")
	}
	if got.Title != src.Title || got.SpotifyID != "x" || !slices.Equal(got.Artists, src.Artists) {
		t.Fatalf("campos do Spotify alterados: %+v", got)
	}
}

func TestEnrichTrackKeepsSpotifyFields(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	src := song.Song{
		Title:       "Come Together - Remastered 2009",
		Artists:     []string{"The Beatles"},
		Album:       "Abbey Road (Remastered 2009)",
		ReleaseDate: "1969-09-26",
		Duration:    259946 * time.Millisecond,
	}
	got, ok, err := c.EnrichTrack(context.Background(), src)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.ReleaseDate != "1969-09-26" || got.Album != src.Album {
		t.Fatalf("sobrescreveu dados do Spotify: %q %q", got.ReleaseDate, got.Album)
	}
}

func TestEnrichTrackNoMatch(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	src := song.Song{Title: "Come Together", Artists: []string{"Ninguém"}, Duration: 258 * time.Second, SpotifyID: "x"}

	got, ok, err := c.EnrichTrack(context.Background(), src)
	if ok || err != nil {
		t.Fatalf("ok=%v err=%v, quero false/nil", ok, err)
	}
	if !reflect.DeepEqual(got, src) {
		t.Fatalf("Song alterada: %+v", got)
	}
}

func TestEnrichTrackNetworkError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	c, _ := newTestClient(t, h, "")
	src := song.Song{Title: "Come Together", Artists: []string{"The Beatles"}}

	got, ok, err := c.EnrichTrack(context.Background(), src)
	if ok || err == nil {
		t.Fatalf("ok=%v err=%v, quero false/erro", ok, err)
	}
	if !reflect.DeepEqual(got, src) {
		t.Fatalf("Song alterada: %+v", got)
	}
}

func albumSongs() []song.Song {
	return []song.Song{
		{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}, Album: "Abbey Road (Remastered 2009)", Duration: 259946 * time.Millisecond, SpotifyID: "a"},
		{Title: "Something - Remastered 2009", Artists: []string{"The Beatles"}, Album: "Abbey Road (Remastered 2009)", Duration: 182293 * time.Millisecond, SpotifyID: "b"},
		{Title: "Maxwell's Silver Hammer - Remastered 2009", Artists: []string{"The Beatles"}, Album: "Abbey Road (Remastered 2009)", Duration: 207106 * time.Millisecond, SpotifyID: "c"},
	}
}

func TestEnrichAlbum(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	songs := albumSongs()
	orig := albumSongs()

	got, err := c.EnrichAlbum(context.Background(), "Abbey Road (Remastered 2009)", "The Beatles", songs)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range got {
		if s.TrackNumber != i+1 || s.DiscNumber != 1 || s.TrackCount != 17 || s.ISRC == "" || s.CoverURL == "" {
			t.Errorf("[%d] = %+v", i, s)
		}
		if s.Title != orig[i].Title || s.SpotifyID != orig[i].SpotifyID {
			t.Errorf("[%d] campos do Spotify alterados: %+v", i, s)
		}
	}
	if !reflect.DeepEqual(songs, orig) {
		t.Fatal("a slice original foi modificada")
	}
}

func TestEnrichAlbumOutOfOrder(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	songs := albumSongs()
	songs[0], songs[2] = songs[2], songs[0]

	got, err := c.EnrichAlbum(context.Background(), "Abbey Road (Remastered 2009)", "The Beatles", songs)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].TrackNumber != 3 || got[1].TrackNumber != 2 || got[2].TrackNumber != 1 {
		t.Fatalf("faixas = %d %d %d, quero 3 2 1", got[0].TrackNumber, got[1].TrackNumber, got[2].TrackNumber)
	}
}

func TestEnrichAlbumNotFound(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	songs := albumSongs()

	got, err := c.EnrichAlbum(context.Background(), "Disco Que Não Existe", "The Beatles", songs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, albumSongs()) {
		t.Fatalf("songs alteradas: %+v", got)
	}
}

func TestDiscography(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")

	got, err := c.Discography(context.Background(), "The Beatles", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, quero 4 álbuns: %+v", len(got), got)
	}
	for _, a := range got {
		if a.RecordType != "album" || a.ID == 0 || a.Title == "" {
			t.Errorf("entrada = %+v", a)
		}
	}

	all, err := c.Discography(context.Background(), "The Beatles", []string{"album", "single"})
	if err != nil || len(all) != 6 {
		t.Fatalf("com singles: len = %d, err = %v", len(all), err)
	}

	if _, err := c.Discography(context.Background(), "Ninguém", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quero ErrNotFound", err)
	}
}

func TestAlbumSongs(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")

	got, err := c.AlbumSongs(context.Background(), 12047952)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, quero 3", len(got))
	}
	for i, s := range got {
		if s.Album != "Abbey Road (Remastered 2009)" || s.TrackNumber != i+1 || s.ISRC == "" || s.SpotifyID != "" {
			t.Errorf("[%d] = %+v", i, s)
		}
		if s.Duration == 0 || len(s.Artists) != 1 || s.CoverURL == "" || s.TrackCount != 17 {
			t.Errorf("[%d] incompleta: %+v", i, s)
		}
	}
}

func TestEnrichTrackOtherAlbumOnlyISRC(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"data":[{"id":5,"title":"Come Together (Remastered 2015)","duration":258,"artist":{"name":"The Beatles"},"album":{"title":"1 (Remastered 2015)"}}],"total":1}`))
		case "/track/5":
			w.Write([]byte(`{"id":5,"title":"Come Together (Remastered 2015)","isrc":"GBAYE0601690","duration":258,"track_position":13,"disk_number":1,"release_date":"2015-11-06","artist":{"name":"The Beatles"},"album":{"title":"1 (Remastered 2015)","nb_tracks":27,"cover_xl":"http://capa-do-1.jpg","release_date":"2015-11-06"}}`))
		default:
			w.Write(fixture(t, "error_800.json"))
		}
	})
	c, _ := newTestClient(t, h, "")
	src := song.Song{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}, Album: "Abbey Road (Remastered 2009)", Duration: 259946 * time.Millisecond}

	got, ok, err := c.EnrichTrack(context.Background(), src)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := src
	want.ISRC = "GBAYE0601690"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("misturou dados de outro álbum:\n got %+v\nwant %+v", got, want)
	}
}

func TestEnrichAlbumMultiArtistQuery(t *testing.T) {
	c, rec := newTestClient(t, fixtureServer(t), "")
	if _, err := c.EnrichAlbum(context.Background(), "Abbey Road (Remastered 2009)", "The Beatles, Billy Preston", albumSongs()); err != nil {
		t.Fatal(err)
	}
	if q := rec.reqs[0].URL.Query().Get("q"); q != "The Beatles abbey road" {
		t.Fatalf("query = %q, quero só o artista principal", q)
	}
	if len(rec.reqs) != 3 {
		t.Fatalf("requisições = %d, quero 3 (busca + álbum + faixas)", len(rec.reqs))
	}
}

func TestSearchTrack(t *testing.T) {
	c, _ := newTestClient(t, fixtureServer(t), "")
	s, err := c.SearchTrack(context.Background(), "come together beatles")
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Come Together (Remastered 2009)" || s.ISRC != "GBAYE0601690" || s.MainArtist() == "" || s.CoverURL == "" || s.Duration == 0 {
		t.Errorf("SearchTrack = %+v", s)
	}

	empty, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[],"total":0}`))
	}), "")
	if _, err := empty.SearchTrack(context.Background(), "nada"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestSearchTrackSkipsVariousArtists(t *testing.T) {
	c, rec := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"data":[
				{"id":1,"title":"Never Gonna Give You Up","duration":211,"artist":{"name":"Rick Astley"},"album":{"id":10,"title":"Reeling In The Decades"}},
				{"id":6,"title":"Never Gonna Give You Up","duration":213,"artist":{"name":"Rick Astley"},"album":{"id":60,"title":"Greatest Hits"}},
				{"id":2,"title":"Never Gonna Give You Up","duration":213,"artist":{"name":"Rick Astley"},"album":{"id":20,"title":"Whenever You Need Somebody"}},
				{"id":3,"title":"Never Gonna Give You Up","duration":213,"artist":{"name":"Rick Astley"},"album":{"id":30,"title":"Outro"}}
			],"total":4}`))
		case "/album/10":
			w.Write([]byte(`{"id":10,"title":"Reeling In The Decades","artist":{"name":"Vários intérpretes"}}`))
		case "/album/20":
			w.Write([]byte(`{"id":20,"title":"Whenever You Need Somebody","artist":{"name":"Rick Astley"}}`))
		case "/track/2":
			w.Write([]byte(`{"id":2,"title":"Never Gonna Give You Up","duration":213,"artist":{"name":"Rick Astley"},"album":{"id":20,"title":"Whenever You Need Somebody","release_date":"1987-11-12"}}`))
		default:
			t.Errorf("não devia ler %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}), "")
	s, err := c.SearchTrack(context.Background(), "never gonna give you up")
	if err != nil {
		t.Fatal(err)
	}
	if s.Album != "Whenever You Need Somebody" || s.ReleaseDate != "1987-11-12" {
		t.Errorf("devia pular a coletânea: %+v", s)
	}
	_ = rec
}
