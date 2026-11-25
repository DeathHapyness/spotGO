package bandcamp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/song"
)

func client(t *testing.T, body []byte) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		json.Unmarshal(raw, &b)
		if b["search_filter"] != "t" || b["search_text"] == "" || r.URL.Path != searchPath {
			t.Errorf("requisição: %s %s", r.URL.Path, raw)
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c := New(fetch.New(fetch.Options{Sleep: func(context.Context, time.Duration) error { return nil }}))
	c.base = srv.URL
	return c
}

func TestFind(t *testing.T) {
	b, err := os.ReadFile("testdata/maria_tambien.json")
	if err != nil {
		t.Fatal(err)
	}
	c := client(t, b)
	s := song.Song{Title: "Maria También", Artists: []string{"Khruangbin"}, Album: "Con Todo El Mundo", Duration: 200 * time.Second}
	got, err := c.Find(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Author != "Khruangbin" || got.Result.URL == "" || got.Result.Source != "bandcamp" {
		t.Errorf("escolhido: %+v", got.Result)
	}
	if got.Result.Name != "Maria También" {
		t.Errorf("devia ficar com a de estúdio, não a ao vivo: %q", got.Result.Name)
	}
}

func TestFindNothing(t *testing.T) {
	c := client(t, []byte(`{"auto":{"results":[]}}`))
	if _, err := c.Find(context.Background(), song.Song{Title: "x", Artists: []string{"y"}, Duration: time.Minute}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if _, err := parse([]byte(`{}`)); err == nil {
		t.Error("formato desconhecido deve dar erro")
	}
}
