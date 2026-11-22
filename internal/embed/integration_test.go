//go:build integration

package embed

import (
	"context"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
)

func TestIntegrationRealEmbed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New(fetch.New(fetch.Options{UserAgent: "Mozilla/5.0 (X11; Linux x86_64) Chrome/131.0 Safari/537.36"}))

	p, err := c.Playlist(ctx, "37i9dQZF1DXcBWIGoYBM5M")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name == "" || len(p.Songs) == 0 {
		t.Fatalf("playlist vazia: %+v", p)
	}
	t.Logf("%s: %d faixas, primeira: %s — %v", p.Name, len(p.Songs), p.Songs[0].Title, p.Songs[0].Artists)

	a, err := c.Album(ctx, "0ETFjACtuP2ADo6LFhL6HN")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s (%s): %d faixas", a.Name, a.Artist, len(a.Songs))
}
