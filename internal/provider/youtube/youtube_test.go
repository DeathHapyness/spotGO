package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/song"
)

func TestParse(t *testing.T) {
	b, err := os.ReadFile("testdata/come_together.json")
	if err != nil {
		t.Fatal(err)
	}
	rs, err := parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) < 10 {
		t.Fatalf("%d resultados", len(rs))
	}
	r := rs[0]
	if r.ID != "45cYwDMibGo" || r.Author != "The Beatles" || r.Duration != 259*time.Second || r.Views != 166174637 ||
		r.URL != "https://www.youtube.com/watch?v=45cYwDMibGo" || r.Verified {
		t.Errorf("rs[0] = %+v", r)
	}
	if _, err := parse([]byte(`{}`)); err == nil {
		t.Error("formato desconhecido deve dar erro")
	}
	if _, err := parse([]byte(`não é json`)); err == nil {
		t.Error("JSON quebrado deve dar erro")
	}
}

func TestParseViews(t *testing.T) {
	for in, want := range map[string]int64{"166,174,637 views": 166174637, "1 view": 1, "": 0, "No views": 0} {
		if got := parseViews(in); got != want {
			t.Errorf("parseViews(%q) = %d", in, got)
		}
	}
}

func server(t *testing.T, fixture string, gotQuery *string) *Client {
	t.Helper()
	body, _ := os.ReadFile(fixture)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b searchBody
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &b)
		*gotQuery = b.Query
		if b.Params != paramsVideos || b.Context.Client.ClientName != "WEB" {
			t.Errorf("corpo: %s", raw)
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c := New(fetch.New(fetch.Options{Sleep: func(context.Context, time.Duration) error { return nil }}))
	c.base = srv.URL
	return c
}

func TestFind(t *testing.T) {
	var q string
	c := server(t, "testdata/come_together.json", &q)
	s := song.Song{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}, Duration: 259 * time.Second}
	got, err := c.Find(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "come together") {
		t.Errorf("busca = %q", q)
	}
	if got.Result.Author != "The Beatles" || got.Score < 60 {
		t.Errorf("escolhido: %+v (%.1f)", got.Result, got.Score)
	}

	s = song.Song{Title: "Uma Faixa Que Não Existe", Artists: []string{"Ninguém"}, Duration: 100 * time.Second}
	if _, err := c.Find(context.Background(), s); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}
