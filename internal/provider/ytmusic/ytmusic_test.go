package ytmusic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/song"
)

type sentBody struct {
	Context struct {
		Client map[string]string `json:"client"`
	} `json:"context"`
	Query  string  `json:"query"`
	Params *string `json:"params"`
}

type fakeYTM struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []sentBody
}

// serve responde com a fixture escolhida pelo params do corpo:
// songs, videos ou "" (sem params, a busca por ISRC). "400" = erro HTTP.
func serve(t *testing.T, fixtures map[string]string) (*Client, *fakeYTM) {
	t.Helper()
	fake := &fakeYTM{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b sentBody
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Errorf("corpo inválido: %s", raw)
		}
		fake.mu.Lock()
		fake.requests = append(fake.requests, r)
		fake.bodies = append(fake.bodies, b)
		fake.mu.Unlock()

		key := ""
		if b.Params != nil {
			key = map[string]string{paramsSongs: "songs", paramsVideos: "videos"}[*b.Params]
		}
		name, ok := fixtures[key]
		if !ok || name == "400" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write(fixture(t, name))
	}))
	t.Cleanup(srv.Close)
	c := New(fetch.New(fetch.Options{}))
	c.base = srv.URL
	c.now = func() time.Time { return time.Date(2026, 11, 22, 23, 0, 0, 0, time.UTC) }
	return c, fake
}

var comeTogether = song.Song{
	Title:    "Come Together - Remastered 2009",
	Artists:  []string{"The Beatles"},
	Album:    "Abbey Road (Remastered 2009)",
	Duration: 4*time.Minute + 19*time.Second,
	ISRC:     "GBAYE0601690",
}

func TestSearchRequestBody(t *testing.T) {
	c, fake := serve(t, map[string]string{"songs": "songs"})
	if _, err := c.Search(context.Background(), "the beatles - come together", kindSongs); err != nil {
		t.Fatal(err)
	}
	r, b := fake.requests[0], fake.bodies[0]
	if r.Method != http.MethodPost || r.URL.Path != "/youtubei/v1/search" || r.URL.Query().Get("prettyPrint") != "false" {
		t.Errorf("requisição = %s %s", r.Method, r.URL)
	}
	if r.Header.Get("Origin") != "https://music.youtube.com" || r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", r.Header)
	}
	cl := b.Context.Client
	if cl["clientName"] != "WEB_REMIX" || cl["clientVersion"] != "1.20261122.01.00" || cl["hl"] != "en" {
		t.Errorf("client = %v", cl)
	}
	if _, ok := cl["gl"]; ok {
		t.Errorf("gl não deveria ser enviado")
	}
	if b.Query != "the beatles - come together" || b.Params == nil || *b.Params != paramsSongs {
		t.Errorf("query/params = %q %v", b.Query, b.Params)
	}
}

func TestSearchISRCHasNoParams(t *testing.T) {
	c, fake := serve(t, map[string]string{"": "isrc"})
	rs, err := c.Search(context.Background(), "GBAYE0601690", kindAll)
	if err != nil || len(rs) == 0 {
		t.Fatalf("Search = %v, %v", rs, err)
	}
	if fake.bodies[0].Params != nil {
		t.Errorf("params = %q, quero ausente", *fake.bodies[0].Params)
	}
}

func TestFindISRCShortCircuit(t *testing.T) {
	c, fake := serve(t, map[string]string{"": "isrc", "songs": "songs", "videos": "videos"})
	got, err := c.Find(context.Background(), comeTogether)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.ID != "oolpPmuK2I8" || !got.Result.ISRCSearch || got.Score <= 80 {
		t.Fatalf("Find = %+v", got)
	}
	if len(fake.bodies) != 1 {
		t.Fatalf("requisições = %d, quero 1", len(fake.bodies))
	}
}

func TestFindNoISRC(t *testing.T) {
	c, fake := serve(t, map[string]string{"songs": "songs", "videos": "videos"})
	s := comeTogether
	s.ISRC = ""
	got, err := c.Find(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.ID != "oolpPmuK2I8" || len(fake.bodies) != 1 {
		t.Fatalf("Find = %+v com %d requisições", got, len(fake.bodies))
	}
	if fake.bodies[0].Query != "the beatles - come together - remastered 2009" {
		t.Errorf("query = %q", fake.bodies[0].Query)
	}
}

func TestFindFallsBackToVideos(t *testing.T) {
	c, fake := serve(t, map[string]string{"songs": "multi_songs", "videos": "videos"})
	s := comeTogether
	s.ISRC = ""
	got, err := c.Find(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	videos, _ := parseSearch(fixture(t, "videos"), kindVideos)
	found := false
	for _, v := range videos {
		found = found || v.ID == got.Result.ID
	}
	if !found || len(fake.bodies) != 2 {
		t.Fatalf("Find = %+v com %d requisições; quero um vídeo de videos.json", got.Result, len(fake.bodies))
	}
}

func TestFindNotFound(t *testing.T) {
	c, _ := serve(t, map[string]string{"songs": "songs", "videos": "videos"})
	_, err := c.Find(context.Background(), song.Song{Title: "Música Inexistente", Artists: []string{"Ninguém"}, Duration: time.Minute})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quero ErrNotFound", err)
	}
}

func TestFindHTTPError(t *testing.T) {
	c, _ := serve(t, map[string]string{"songs": "400"})
	s := comeTogether
	s.ISRC = ""
	_, err := c.Find(context.Background(), s)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quero erro HTTP", err)
	}
}

func TestFindCanceled(t *testing.T) {
	c, _ := serve(t, map[string]string{"": "isrc", "songs": "songs", "videos": "videos"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Find(ctx, comeTogether); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, quero context.Canceled", err)
	}
}

// Resposta real em que o card do topo é um vídeo aleatório e a faixa certa
// vem como item comum, sem duração.
func TestFindISRCWithoutDuration(t *testing.T) {
	c, fake := serve(t, map[string]string{"": "isrc_nocard", "songs": "multi_songs", "videos": "multi_videos"})
	s := song.Song{Title: "Something - Remastered 2009", Artists: []string{"The Beatles"},
		Album: "Abbey Road (Remastered 2009)", Duration: 182 * time.Second, ISRC: "GBAYE0601691"}
	got, err := c.Find(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.ID != "VWO3nEuWo4k" || len(fake.bodies) != 1 {
		t.Fatalf("Find = %+v com %d requisições; quero VWO3nEuWo4k com 1", got.Result, len(fake.bodies))
	}
}

func TestSearchRetriesUnknownFormat(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.Write([]byte(`{}`))
			return
		}
		w.Write(fixture(t, "songs"))
	}))
	t.Cleanup(srv.Close)
	c := New(fetch.New(fetch.Options{Sleep: func(context.Context, time.Duration) error { return nil }}))
	c.base = srv.URL
	rs, err := c.Search(context.Background(), "x", kindSongs)
	if err != nil || len(rs) == 0 || n != 2 {
		t.Fatalf("Search = %d resultados, %v, %d requisições", len(rs), err, n)
	}
}
