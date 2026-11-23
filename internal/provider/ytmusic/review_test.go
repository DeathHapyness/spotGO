package ytmusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/song"
)

type fakeItem struct{ id, title, artist, album, duration string }

func itemJSON(it fakeItem, typ string) string {
	runs := []string{}
	if typ != "" {
		runs = append(runs, fmt.Sprintf(`{"text":%q},{"text":" • "}`, typ))
	}
	runs = append(runs, fmt.Sprintf(`{"text":%q,"navigationEndpoint":{"browseEndpoint":{"browseId":"UCx"}}}`, it.artist))
	if it.album != "" {
		runs = append(runs, fmt.Sprintf(`{"text":" • "},{"text":%q,"navigationEndpoint":{"browseEndpoint":{"browseId":"MPREx"}}}`, it.album))
	}
	if it.duration != "" {
		runs = append(runs, fmt.Sprintf(`{"text":" • "},{"text":%q}`, it.duration))
	}
	return fmt.Sprintf(`{"musicResponsiveListItemRenderer":{"playlistItemData":{"videoId":%q},"flexColumns":[`+
		`{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":%q}]}}},`+
		`{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[%s]}}}]}}`, it.id, it.title, strings.Join(runs, ","))
}

// pageJSON monta uma resposta do innertube: filtrada (musicShelfRenderer) ou,
// com typ != "", sem filtro (itemSectionRenderer).
func pageJSON(typ string, items ...fakeItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = itemJSON(it, typ)
	}
	shelf := `{"musicShelfRenderer":{"contents":[` + strings.Join(parts, ",") + `]}}`
	if typ != "" {
		shelf = `{"itemSectionRenderer":{"contents":[` + strings.Join(parts, ",") + `]}}`
	}
	return `{"contents":{"tabbedSearchResultsRenderer":{"tabs":[{"tabRenderer":{"content":{"sectionListRenderer":{"contents":[` + shelf + `]}}}}]}}}`
}

func serveJSON(t *testing.T, pages map[string]string, hits *int) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b sentBody
		json.Unmarshal(raw, &b)
		*hits++
		key := ""
		if b.Params != nil {
			key = map[string]string{paramsSongs: "songs", paramsVideos: "videos"}[*b.Params]
		}
		w.Write([]byte(pages[key]))
	}))
	t.Cleanup(srv.Close)
	c := New(fetch.New(fetch.Options{CacheDir: t.TempDir()}))
	c.base = srv.URL
	return c
}

// A busca por ISRC às vezes devolve lixo (músicas populares da região). Um
// desses resultados não pode virar "confiável" só por aparecer de novo na busca
// por nome.
func TestFindIgnoresUnmatchedISRCResult(t *testing.T) {
	wrong := fakeItem{"WRONG", "Outra Música", "Artista", "Outro Álbum", "3:00"}
	right := fakeItem{"RIGHT", "Mudei", "Artista", "Mudei", "3:00"}
	hits := 0
	c := serveJSON(t, map[string]string{
		"":      pageJSON("Song", fakeItem{"JUNK", "Hit Popular", "Fulano", "", ""}, wrong),
		"songs": pageJSON("", wrong, right),
	}, &hits)
	s := song.Song{Title: "Mudei", Artists: []string{"Artista"}, Album: "Mudei", Duration: 3 * time.Minute, ISRC: "ZZZZ99999999"}
	got, err := c.Find(context.Background(), s)
	if err != nil || got.Result.ID != "RIGHT" {
		t.Fatalf("Find = %+v, %v; quero RIGHT", got.Result, err)
	}
}

// O clientVersion muda todo dia (UTC); a chave do cache não pode mudar junto.
func TestSearchCacheSurvivesDayChange(t *testing.T) {
	hits := 0
	c := serveJSON(t, map[string]string{"songs": pageJSON("", fakeItem{"A", "X", "Y", "", "3:00"})}, &hits)
	day := time.Date(2026, 11, 22, 23, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return day }
	if _, err := c.Search(context.Background(), "x", kindSongs); err != nil {
		t.Fatal(err)
	}
	day = day.Add(2 * time.Hour)
	if _, err := c.Search(context.Background(), "x", kindSongs); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("requisições = %d, quero 1 (a segunda vem do cache)", hits)
	}
}

func TestFillRunsUnparsableNumbers(t *testing.T) {
	var r matching.Result
	fillRuns(&r, []run{{Text: "Metro Boomin"}, {Text: ", "}, {Text: "21 Savage"}, {Text: " • "}, {Text: "3:30"}})
	if !slices.Equal(r.Artists, []string{"Metro Boomin", "21 Savage"}) {
		t.Errorf("artistas = %v", r.Artists)
	}
	var v matching.Result
	fillRuns(&v, []run{{Text: "Canal"}, {Text: " • "}, {Text: "No views"}, {Text: " • "}, {Text: "1:00"}})
	if !slices.Equal(v.Artists, []string{"Canal"}) || v.Views != 0 {
		t.Errorf("No views: %+v", v)
	}
}
