package deezer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

func searchFixture[T any](t *testing.T, name string) []T {
	t.Helper()
	var p page[T]
	if err := json.Unmarshal(fixture(t, name), &p); err != nil {
		t.Fatal(err)
	}
	return p.Data
}

func TestPickTrackComeTogether(t *testing.T) {
	cands := searchFixture[apiTrack](t, "search_track.json")
	src := song.Song{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}, Duration: 259946 * time.Millisecond}

	got, ok := pickTrack(src, "", cands)
	if !ok || got.ID != 116348452 || got.Album.Title != "Abbey Road (Remastered 2009)" {
		t.Fatalf("pickTrack = %d %q, %v; quero Abbey Road (Remastered 2009)", got.ID, got.Album.Title, ok)
	}
}

func TestPickTrackRejectsVariants(t *testing.T) {
	cands := searchFixture[apiTrack](t, "search_track.json")
	cases := []struct {
		name   string
		src    song.Song
		wantID int64
	}{
		{"live só aceita live", song.Song{Title: "Come Together (live)", Artists: []string{"The Beatles Revival Band"}, Duration: 278 * time.Second}, 1849334137},
		{"live não aceita estúdio de mesma duração", song.Song{Title: "Come Together - Live", Artists: []string{"The Beatles"}, Duration: 258 * time.Second}, 0},
		{"estúdio não aceita take de mesma duração", song.Song{Title: "Come Together", Artists: []string{"The Beatles"}, Duration: 216 * time.Second}, 0},
		{"duração longe demais", song.Song{Title: "Come Together", Artists: []string{"The Beatles"}, Duration: 240 * time.Second}, 0},
	}
	for _, c := range cases {
		got, ok := pickTrack(c.src, "", cands)
		if c.wantID == 0 && ok {
			t.Errorf("%s: escolheu %d %q, quero nenhum", c.name, got.ID, got.Title)
		}
		if c.wantID != 0 && (!ok || got.ID != c.wantID) {
			t.Errorf("%s: pickTrack = %d, %v; quero %d", c.name, got.ID, ok, c.wantID)
		}
	}
}

func TestPickTrackRejectsCovers(t *testing.T) {
	cands := searchFixture[apiTrack](t, "search_track.json")
	src := song.Song{Title: "Come Together", Artists: []string{"Someone Else"}, Duration: 258 * time.Second}
	if got, ok := pickTrack(src, "", cands); ok {
		t.Fatalf("escolheu %d %q de %q", got.ID, got.Title, got.Artist.Name)
	}
}

func TestPickTrackKnownAlbum(t *testing.T) {
	cands := searchFixture[apiTrack](t, "search_track.json")
	src := song.Song{Title: "Come Together", Artists: []string{"The Beatles"}, Duration: 259946 * time.Millisecond}

	got, ok := pickTrack(src, "1", cands)
	if !ok || got.Album.Title != "1 (Remastered 2015)" {
		t.Fatalf("pickTrack = %q, %v; quero 1 (Remastered 2015)", got.Album.Title, ok)
	}
	got, ok = pickTrack(src, "Abbey Road", cands)
	if !ok || got.Album.Title != "Abbey Road (Remastered 2009)" {
		t.Fatalf("pickTrack = %q, %v; quero Abbey Road (Remastered 2009)", got.Album.Title, ok)
	}
}

func TestPickTrackNoDuration(t *testing.T) {
	cands := searchFixture[apiTrack](t, "search_track.json")
	src := song.Song{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}}
	if got, ok := pickTrack(src, "", cands); !ok || got.ID != 116348452 {
		t.Fatalf("pickTrack = %d, %v", got.ID, ok)
	}
}

func TestPickAlbumAbbeyRoad(t *testing.T) {
	cands := searchFixture[apiAlbum](t, "search_album.json")
	cases := []struct {
		name    string
		nTracks int
		want    int64
	}{
		{"Abbey Road (Remastered 2009)", 17, 12047952},
		{"Abbey Road", 40, 112126942},
	}
	for _, c := range cases {
		got, ok := pickAlbum(c.name, "The Beatles", c.nTracks, cands)
		if !ok || got.ID != c.want {
			t.Errorf("pickAlbum(%q, %d) = %d %q, %v; quero %d", c.name, c.nTracks, got.ID, got.Title, ok, c.want)
		}
	}
	if got, ok := pickAlbum("Abbey Road", "Ninguém", 17, cands); ok {
		t.Errorf("artista errado escolheu %d", got.ID)
	}
}

func TestIsCompilation(t *testing.T) {
	cases := map[string]bool{
		"1 (Remastered 2015)":          true,
		"The Very Best Of":             true,
		"Greatest Hits":                true,
		"Beatles Gold":                 true,
		"Abbey Road (Remastered 2009)": false,
		"Golden Hour":                  false,
	}
	for title, want := range cases {
		if got := isCompilation(title); got != want {
			t.Errorf("isCompilation(%q) = %v, quero %v", title, got, want)
		}
	}
}
