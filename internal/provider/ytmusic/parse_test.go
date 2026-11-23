package ytmusic

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/matching"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parseFixture(t *testing.T, name string, k kind) []matching.Result {
	t.Helper()
	rs, err := parseSearch(fixture(t, name), k)
	if err != nil {
		t.Fatalf("parseSearch(%s): %v", name, err)
	}
	if len(rs) == 0 {
		t.Fatalf("parseSearch(%s): nenhum resultado", name)
	}
	return rs
}

func TestParseSongs(t *testing.T) {
	r := parseFixture(t, "songs", kindSongs)[0]
	if r.ID != "oolpPmuK2I8" || r.Name != "Come Together (Remastered 2009)" ||
		!slices.Equal(r.Artists, []string{"The Beatles"}) || r.Author != "The Beatles" ||
		r.Album != "Abbey Road (Remastered 2009)" || r.Duration != 4*time.Minute+20*time.Second ||
		!r.Verified || r.Views != 351_000_000 || r.Source != "youtube-music" ||
		r.URL != "https://music.youtube.com/watch?v=oolpPmuK2I8" {
		t.Fatalf("1º resultado = %+v", r)
	}
	if r.Explicit == nil || *r.Explicit {
		t.Fatalf("Explicit = %v, quero false", r.Explicit)
	}
}

func TestParseVideos(t *testing.T) {
	r := parseFixture(t, "videos", kindVideos)[0]
	if r.ID != "45cYwDMibGo" || r.Verified || r.Album != "" || r.Views != 166_000_000 ||
		r.Duration != 4*time.Minute+19*time.Second || r.URL != "https://www.youtube.com/watch?v=45cYwDMibGo" ||
		!slices.Equal(r.Artists, []string{"The Beatles"}) {
		t.Fatalf("1º resultado = %+v", r)
	}
}

func TestParseExplicitAndChannels(t *testing.T) {
	s := parseFixture(t, "multi_songs", kindSongs)[0]
	if s.ID != "gfIFUdFJdoM" || s.Explicit == nil || !*s.Explicit {
		t.Fatalf("1º song = %+v (Explicit %v)", s, s.Explicit)
	}
	v := parseFixture(t, "multi_videos", kindVideos)[1]
	if !slices.Equal(v.Artists, []string{"Lion Beatz"}) || v.Views != 2300 {
		t.Fatalf("2º vídeo = %+v", v)
	}
}

func TestParseISRC(t *testing.T) {
	rs := parseFixture(t, "isrc", kindAll)
	r := rs[0]
	if r.ID != "oolpPmuK2I8" || !r.Verified || !slices.Equal(r.Artists, []string{"The Beatles"}) ||
		r.Duration != 4*time.Minute+20*time.Second {
		t.Fatalf("1º resultado = %+v", r)
	}
	for _, r := range rs {
		if r.Name == "" || r.ID == "" || len(r.Artists) == 0 {
			t.Errorf("resultado incompleto: %+v", r)
		}
		for _, a := range r.Artists {
			if a == "Song" || a == "Video" || a == "Artist" || a == "Album" || a == "Playlist" {
				t.Errorf("tipo lido como artista: %+v", r)
			}
		}
	}
}

func TestParseViews(t *testing.T) {
	cases := map[string]int64{
		"1.1B plays": 1_100_000_000, "6B plays": 6_000_000_000, "2.3K views": 2300,
		"470 views": 470, "1 view": 1, "80 plays": 80, "351M plays": 351_000_000, "1,234 views": 1234,
	}
	for in, want := range cases {
		if got, ok := parseViews(in); !ok || got != want {
			t.Errorf("parseViews(%q) = %v, %v; quero %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"The Beatles", "4:20", "", "Song"} {
		if _, ok := parseViews(in); ok {
			t.Errorf("parseViews(%q) aceitou", in)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{"4:20": 4*time.Minute + 20*time.Second, "1:02:03": time.Hour + 2*time.Minute + 3*time.Second, "0:13": 13 * time.Second}
	for in, want := range cases {
		if got, ok := parseDuration(in); !ok || got != want {
			t.Errorf("parseDuration(%q) = %v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"abc", "", "2009", "4:"} {
		if _, ok := parseDuration(in); ok {
			t.Errorf("parseDuration(%q) aceitou", in)
		}
	}
}

func TestParseBadJSON(t *testing.T) {
	for _, in := range []string{`{}`, `[]`, `not json`, `{"contents":{"tabbedSearchResultsRenderer":{"tabs":[]}}}`} {
		if _, err := parseSearch([]byte(in), kindSongs); !errors.Is(err, errFormat) {
			t.Errorf("parseSearch(%s) err = %v, quero errFormat", in, err)
		}
	}
	noID := `{"contents":{"tabbedSearchResultsRenderer":{"tabs":[{"tabRenderer":{"content":{"sectionListRenderer":{"contents":[{"musicShelfRenderer":{"contents":[{"musicResponsiveListItemRenderer":{"flexColumns":[]}}]}}]}}}}]}}}`
	rs, err := parseSearch([]byte(noID), kindSongs)
	if err != nil || len(rs) != 0 {
		t.Fatalf("item sem videoId = %v, %v", rs, err)
	}
}

func TestFillRunsNumericArtist(t *testing.T) {
	var r matching.Result
	fillRuns(&r, []run{{Text: "50 Cent"}, {Text: " • "}, {Text: "2 views"}, {Text: " • "}, {Text: "3:40"}})
	if !slices.Equal(r.Artists, []string{"50 Cent"}) || r.Views != 2 || r.Duration != 220*time.Second {
		t.Errorf("fillRuns = %+v", r)
	}
}
