//go:build integration

package deezer

import (
	"context"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/embed"
	"github.com/DeathHapyness/spotGO/internal/fetch"
)

func TestIntegrationEmbedPlusDeezer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := fetch.New(fetch.Options{UserAgent: "Mozilla/5.0 (X11; Linux x86_64) Chrome/131.0 Safari/537.36"})
	e, d := embed.New(f), New(f)

	a, err := e.Album(ctx, "0ETFjACtuP2ADo6LFhL6HN")
	if err != nil {
		t.Fatal(err)
	}
	songs, err := d.EnrichAlbum(ctx, a.Name, a.Artist, a.Songs)
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, s := range songs {
		if s.ISRC == "" {
			missing++
			t.Logf("sem ISRC: %s", s.Title)
		}
	}
	t.Logf("%s (%s): %d faixas, %d sem dados do Deezer; [0] = %d/%d %s %s", a.Name, a.Artist, len(songs), missing,
		songs[0].TrackNumber, songs[0].TrackCount, songs[0].ISRC, songs[0].CoverURL)
	if missing > 0 {
		t.Errorf("%d faixas sem ISRC", missing)
	}

	s, ok, err := d.EnrichTrack(ctx, a.Songs[1])
	if err != nil || !ok {
		t.Fatalf("EnrichTrack(%q): ok=%v err=%v", a.Songs[1].Title, ok, err)
	}
	t.Logf("faixa: %s → %s, faixa %d, ISRC %s", s.Title, s.Album, s.TrackNumber, s.ISRC)
}
