package lyrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/song"
)

func client(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(fetch.New(fetch.Options{Sleep: func(context.Context, time.Duration) error { return nil }}))
	c.base = srv.URL
	return c
}

var come = song.Song{Title: "Come Together - Remastered 2009", Artists: []string{"The Beatles"}, Album: "Abbey Road", Duration: 259 * time.Second}

func TestGetExact(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/get" || q.Get("duration") != "259" || q.Get("album_name") != "Abbey Road" || q.Get("track_name") != come.Title {
			t.Errorf("requisição: %s", r.URL)
		}
		w.Write([]byte(`{"plainLyrics":"Here come old flat top","syncedLyrics":"[00:01.00] Here come old flat top"}`))
	})
	l, err := c.Get(context.Background(), come)
	if err != nil || l.Text() != "[00:01.00] Here come old flat top" || l.Plain == "" {
		t.Fatalf("Get = %+v, %v", l, err)
	}
}

func TestGetFallsBackToSearch(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/get":
			w.WriteHeader(404)
			w.Write([]byte(`{"code":404,"name":"TrackNotFound"}`))
		case "/api/search":
			if r.URL.Query().Get("track_name") != "Come Together" {
				t.Errorf("busca sem o sufixo: %s", r.URL)
			}
			w.Write([]byte(`[
				{"duration":300,"plainLyrics":"longa demais"},
				{"duration":258,"plainLyrics":"só texto"},
				{"duration":260,"plainLyrics":"texto","syncedLyrics":"[00:01.00] sincronizada"},
				{"duration":259,"plainLyrics":""}
			]`))
		}
	})
	l, err := c.Get(context.Background(), come)
	if err != nil {
		t.Fatal(err)
	}
	if l.Synced != "[00:01.00] sincronizada" {
		t.Errorf("devia escolher a de duração próxima com sincronia: %+v", l)
	}
}

func TestGetNotFound(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/get" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`[{"duration":100,"plainLyrics":"outra música"}]`))
	})
	if _, err := c.Get(context.Background(), come); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if _, err := c.Get(context.Background(), song.Song{Title: "sem artista"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("sem artista: %v", err)
	}
}

func TestGetInstrumental(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"instrumental":true}`))
	})
	l, err := c.Get(context.Background(), come)
	if err != nil || !l.Instrumental || l.Text() != "" {
		t.Errorf("Get = %+v, %v", l, err)
	}
}

func TestGetServerError(t *testing.T) {
	c := client(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	if _, err := c.Get(context.Background(), come); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("erro do servidor não é 'não encontrada': %v", err)
	}
}
