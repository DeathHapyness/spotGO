package embed

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".html"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func wrapPage(t *testing.T, entity any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"props": map[string]any{"pageProps": map[string]any{"state": map[string]any{"data": map[string]any{"entity": entity}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`<html><body><script id="__NEXT_DATA__" type="application/json">` + string(data) + `</script></body></html>`)
}

func synthTrack(i int) map[string]any {
	return map[string]any{
		"uri":        fmt.Sprintf("spotify:track:%022d", i),
		"title":      fmt.Sprintf("Faixa %d", i),
		"subtitle":   "Artista",
		"duration":   180000,
		"isPlayable": true,
		"entityType": "track",
	}
}

func syntheticPage(t *testing.T, kind song.Kind, items []map[string]any) []byte {
	t.Helper()
	return wrapPage(t, map[string]any{
		"type":      string(kind),
		"uri":       "spotify:" + string(kind) + ":37i9dQZF1DXcBWIGoYBM5M",
		"name":      "Sintética",
		"subtitle":  "Alguém",
		"trackList": items,
	})
}

func TestParseTrackFixture(t *testing.T) {
	s, err := parseTrack(fixture(t, "track"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.Title, "Flower In A Hurricane") {
		t.Errorf("Title = %q", s.Title)
	}
	if len(s.Artists) != 5 || s.Artists[0] != "Justice" {
		t.Errorf("Artists = %v", s.Artists)
	}
	if s.Duration != 224877*time.Millisecond {
		t.Errorf("Duration = %v", s.Duration)
	}
	if s.ReleaseDate != "2026-10-15" {
		t.Errorf("ReleaseDate = %q", s.ReleaseDate)
	}
	if s.SpotifyID != "4HCOHRCSQelz2GPX8iTamO" {
		t.Errorf("SpotifyID = %q", s.SpotifyID)
	}
	if !strings.Contains(s.CoverURL, "ab67616d0000b273") {
		t.Errorf("CoverURL deveria ser a de 640px: %q", s.CoverURL)
	}
}

func TestParseAlbumFixture(t *testing.T) {
	c, err := parseCollection(fixture(t, "album"), song.KindAlbum)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Abbey Road (Remastered 2009)" || c.Artist != "The Beatles" || c.ID != "0ETFjACtuP2ADo6LFhL6HN" {
		t.Errorf("álbum = %q / %q / %q", c.Name, c.Artist, c.ID)
	}
	if len(c.Songs) != 3 {
		t.Fatalf("faixas = %d", len(c.Songs))
	}
	s := c.Songs[0]
	if s.Title != "Come Together - Remastered 2009" || s.TrackNumber != 1 || s.TrackCount != 3 {
		t.Errorf("faixa 1 = %+v", s)
	}
	if s.Album != c.Name || s.AlbumArtist != "The Beatles" || s.CoverURL != c.CoverURL || c.CoverURL == "" {
		t.Errorf("dados do álbum não propagados: %+v", s)
	}
	if s.Duration != 259946*time.Millisecond {
		t.Errorf("Duration = %v", s.Duration)
	}
}

func TestParsePlaylistFixture(t *testing.T) {
	c, err := parseCollection(fixture(t, "playlist"), song.KindPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Today’s Top Hits" {
		t.Errorf("Name = %q", c.Name)
	}
	if len(c.Songs) != 3 || len(c.Songs[0].Artists) != 5 {
		t.Fatalf("songs = %+v", c.Songs)
	}
	if c.Songs[0].Album != "" || c.Truncated {
		t.Errorf("playlist não deve preencher Album nem estar truncada: %+v", c)
	}
}

func TestParseArtistFixture(t *testing.T) {
	c, err := parseCollection(fixture(t, "artist"), song.KindArtist)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "The Beatles" || len(c.Songs) != 3 {
		t.Errorf("artista = %q, %d faixas", c.Name, len(c.Songs))
	}
}

func TestSplitArtists(t *testing.T) {
	tests := map[string][]string{
		"A, B":                           {"A", "B"},
		"Tyler, The Creator, Kali Uchis": {"Tyler, The Creator", "Kali Uchis"},
		"Solo":                           {"Solo"},
		"":                               nil,
		" A ,  B ":                       {"A", "B"},
	}
	for in, want := range tests {
		if got := splitArtists(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitArtists(%q) = %q, quero %q", in, got, want)
		}
	}
}

func TestPlaylistTruncated(t *testing.T) {
	for n, want := range map[int]bool{100: true, 99: false} {
		items := make([]map[string]any, n)
		for i := range items {
			items[i] = synthTrack(i)
		}
		c, err := parseCollection(syntheticPage(t, song.KindPlaylist, items), song.KindPlaylist)
		if err != nil {
			t.Fatal(err)
		}
		if c.Truncated != want {
			t.Errorf("%d faixas: Truncated = %v, quero %v", n, c.Truncated, want)
		}
	}
}

func TestSkipsNonTracks(t *testing.T) {
	episode := synthTrack(2)
	episode["entityType"] = "episode"
	local := synthTrack(3)
	local["uri"] = "spotify:local:Artista:Album:Faixa:180"
	unplayable := synthTrack(4)
	unplayable["isPlayable"] = false
	items := []map[string]any{episode, synthTrack(1), local, unplayable}

	c, err := parseCollection(syntheticPage(t, song.KindAlbum, items), song.KindAlbum)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Songs) != 1 || c.Skipped != 3 {
		t.Fatalf("songs=%d skipped=%d, quero 1 e 3", len(c.Songs), c.Skipped)
	}
	if c.Songs[0].TrackNumber != 1 || c.Songs[0].TrackCount != 1 {
		t.Errorf("numeração deve contar só faixas aceitas: %+v", c.Songs[0])
	}
}

func TestFormatChanged(t *testing.T) {
	pages := map[string][]byte{
		"vazio":      nil,
		"sem script": []byte("<html>sem script</html>"),
		"json ruim":  []byte(`<script id="__NEXT_DATA__" type="application/json">{quebrado</script>`),
		"sem entity": wrapPage(t, nil),
		"sem fechar": []byte(`<script id="__NEXT_DATA__" type="application/json">{"props":{}}`),
	}
	for name, page := range pages {
		if _, err := parseTrack(page); !errors.Is(err, ErrFormatChanged) {
			t.Errorf("%s: parseTrack err = %v", name, err)
		}
		if _, err := parseCollection(page, song.KindAlbum); !errors.Is(err, ErrFormatChanged) {
			t.Errorf("%s: parseCollection err = %v", name, err)
		}
	}
}
