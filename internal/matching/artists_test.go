package matching

import (
	"math"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/song"
)

func s(title string, artists ...string) song.Song { return song.Song{Title: title, Artists: artists} }
func r(name string, artists ...string) Result     { return Result{Name: name, Artists: artists} }

// Casos de test_calc_main_artist_match_single_result_artist (spotDL).
func TestMainArtistMatchSingleResultArtist(t *testing.T) {
	cases := []struct {
		song    song.Song
		result  Result
		want    float64
		atLeast bool
	}{
		{s("", "LUCO", "Alice Gray"), r("", "LUCO"), 100, false},
		{s("", "LUCO", "Alice Gray"), r("", "LUCO & Alice Gray"), 100, false},
		{s("", "LUCO", "Alice Gray"), r("", "LUCA"), 0, false},
		{s("", "Queen", "David Bowie"), r("", "Queens of the Stone Age"), 0, false},
		{s("", "LUCO", "Alice Gray"), r("", "LUCO mit Alice Gray"), 100, false},
		{s("", "Drake", "Rihanna"), r("", "Drake Bell"), 0, false},
		{s("", "The The", "Alice Gray"), r("", "The Weeknd"), 0, false},
		{s("", "A$AP Rocky", "Skepta"), r("", "ASAP Rocky"), 85, true},
		{s("", "Alpha Bravo", "Alpha"), r("", "Alpha Bravo & Alpha"), 100, false},
		{s("", "Alpha", "Alpha Bravo"), r("", "Alpha Bravo & Alpha"), 100, false},
	}
	for _, c := range cases {
		got := mainArtistMatch(c.song, c.result)
		if (c.atLeast && got < c.want) || (!c.atLeast && got != c.want) {
			t.Errorf("mainArtistMatch(%v, %v) = %v, quero %v", c.song.Artists, c.result.Artists, got, c.want)
		}
	}
}

// Casos de test_calc_artists_match_single_result_artist (spotDL).
func TestOtherArtistsMatchSingleResultArtist(t *testing.T) {
	cases := []struct {
		song   song.Song
		result Result
		want   float64
	}{
		{s("Wasted", "LUCO", "Alice Gray"), r("Wasted (feat. Alice Gray)", "LUCO"), 100},
		{s("Wasted (feat. Alice Gray)", "LUCO", "Alice Gray"), r("Wasted (feat. Alice Gray)", "LUCO"), 100},
		{s("Wasted", "LUCO", "Alice Gray"), r("Wasted", "LUCO & Alice Gray"), 100},
		{s("Wasted", "LUCO", "Alice Gray"), r("Wasted", "LUCO"), 0},
		{s("Love Me", "LUCO", "Love"), r("Love Me", "LUCO"), 0},
		{s("Wasted", "Gray", "Alice Gray"), r("Wasted (feat. Alice)", "Gray"), 0},
		{s("🔥", "LUCO", "🔥"), r("🔥", "LUCO"), 0},
		{s("zapach zimy i papierosów", "Hodak", "Miroff", "DJ Eprom"), r("Hodak/Miroff - zapach zimy i papierosów (feat. DJ Eprom)", "Hodak"), 100},
		{s("With or Without You", "LUCO", "Alice Gray"), r("With or Without You (feat. Alice Gray)", "LUCO"), 100},
		{s("Wasted - Remix", "LUCO", "Alice Gray"), r("Wasted (mit Alice Gray) [Remix]", "LUCO"), 100},
		{s("Song", "DJ Khalid", "Khalid"), r("Song", "DJ Khalid"), 0},
		{s("Song", "DJ Khalid", "Khalid"), r("DJ Khalid - Song", "DJ Khalid"), 0},
		{s("Song", "DJ Khalid", "Khalid"), r("Song", "DJ Khalid & Khalid"), 100},
		{s("Song", "DJ Khalid", "Khalid"), r("Song (feat. Khalid)", "DJ Khalid"), 100},
		{s("Song", "Khalid", "DJ Khalid"), r("Song", "DJ Khalid"), 100},
	}
	for _, c := range cases {
		if got := otherArtistsMatch(c.song, c.result); got != c.want {
			t.Errorf("otherArtistsMatch(%q %v, %q %v) = %v, quero %v",
				c.song.Title, c.song.Artists, c.result.Name, c.result.Artists, got, c.want)
		}
	}
}

func TestOtherArtistsMatchMultipleResultArtists(t *testing.T) {
	got := otherArtistsMatch(s("Down", "Jay Sean", "Lil Wayne"), r("Down", "Jay Sean", "Lil Wayne"))
	if got != 100 {
		t.Fatalf("mesmos artistas = %v, quero 100", got)
	}
	got = otherArtistsMatch(s("Down", "Jay Sean", "Lil Wayne", "X"), r("Down", "Jay Sean", "Lil Wayne"))
	if !near(got, 50) {
		t.Fatalf("artista faltando = %v, quero 50", got)
	}
}

func TestArtistsMatchVerified(t *testing.T) {
	res := r("Come Together (Remastered 2009)", "The Beatles")
	res.Verified = true
	if got := artistsMatch(s("Come Together - Remastered 2009", "The Beatles"), res); got != 100 {
		t.Fatalf("artistsMatch = %v, quero 100", got)
	}
}

func TestArtistsMatchUnverifiedChannel(t *testing.T) {
	res := Result{Name: "The Beatles - Come Together", Author: "TheBeatlesVEVO"}
	got := artistsMatch(s("Come Together", "The Beatles"), res)
	if got < 70 {
		t.Fatalf("artista no título de um upload = %v, quero >= 70", got)
	}
}

func TestArtistsMatchEmpty(t *testing.T) {
	cases := []struct {
		song   song.Song
		result Result
	}{
		{s("X"), r("X", "A")},
		{s("X", "A"), r("X")},
		{s("X", "A", "B"), r("X")},
		{s("🔥", "🔥"), r("🔥", "🔥")},
	}
	for _, c := range cases {
		got := artistsMatch(c.song, c.result)
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("artistsMatch(%v, %v) = %v", c.song.Artists, c.result.Artists, got)
		}
	}
	if got := artistsMatch(s("X"), r("X", "A")); got != 0 {
		t.Errorf("song sem artistas = %v, quero 0", got)
	}
}
