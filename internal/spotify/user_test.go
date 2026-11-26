package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
)

func TestExchangeAndRefreshForms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if _, _, ok := r.BasicAuth(); ok {
			t.Error("PKCE não manda o secret")
		}
		f := r.PostForm
		switch f.Get("grant_type") {
		case "authorization_code":
			if f.Get("code") != "c" || f.Get("code_verifier") != "v" || f.Get("client_id") != "cid" || f.Get("redirect_uri") != RedirectURI {
				t.Errorf("form = %v", f)
			}
		case "refresh_token":
			if f.Get("refresh_token") != "r1" || f.Get("client_id") != "cid" {
				t.Errorf("form = %v", f)
			}
		}
		w.Write([]byte(`{"access_token":"a","expires_in":3600,"refresh_token":"r2"}`))
	}))
	defer srv.Close()
	if tok, rt, err := ExchangeCode(context.Background(), srv.Client(), srv.URL, "cid", "c", "v", RedirectURI); err != nil || tok.AccessToken != "a" || rt != "r2" {
		t.Errorf("ExchangeCode = %v %q %v", tok, rt, err)
	}
	if _, rt, err := Refresh(context.Background(), srv.Client(), srv.URL, "cid", "r1"); err != nil || rt != "r2" {
		t.Errorf("Refresh = %q %v", rt, err)
	}
}

const trackJSON = `{"id":"%s","type":"track","name":"Música %s","duration_ms":200000,"track_number":3,"disc_number":1,
"artists":[{"name":"A"},{"name":"B"}],"external_ids":{"isrc":"ISRC%s"},
"album":{"name":"Disco","release_date":"1999-01-02","total_tracks":10,"artists":[{"name":"A"}],
"images":[{"url":"grande","width":640},{"url":"pequena","width":64}]}}`

type apiRig struct {
	srv      *httptest.Server
	tokens   atomic.Int32
	expired  atomic.Bool
	items    func(w http.ResponseWriter, r *http.Request)
	refreshT string
}

func newAPIRig(t *testing.T) *apiRig {
	g := &apiRig{refreshT: "r1"}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			r.ParseForm()
			if r.PostForm.Get("refresh_token") != g.refreshT {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"invalid_grant","error_description":"Refresh token revoked"}`))
				return
			}
			n := g.tokens.Add(1)
			g.refreshT = fmt.Sprint("r", n+1)
			fmt.Fprintf(w, `{"access_token":"tok%d","expires_in":3600,"refresh_token":%q}`, n, g.refreshT)
		case r.Header.Get("Authorization") == "" || (g.expired.Load() && r.Header.Get("Authorization") == "Bearer tok1"):
			w.WriteHeader(401)
		case strings.HasSuffix(r.URL.Path, "/items"):
			g.items(w, r)
		default:
			w.Write([]byte(`{"name":"Minha","owner":{"display_name":"Eu"}}`))
		}
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *apiRig) user(refresh string) (*User, *[]string) {
	u := NewUser(fetch.New(fetch.Options{Sleep: func(context.Context, time.Duration) error { return nil }}), "cid", refresh)
	u.HTTP, u.APIBase, u.TokenURL = g.srv.Client(), g.srv.URL, g.srv.URL+"/token"
	var saved []string
	u.OnRefresh = func(r string) error { saved = append(saved, r); return nil }
	return u, &saved
}

func TestUserPlaylistPages(t *testing.T) {
	g := newAPIRig(t)
	g.items = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "" {
			fmt.Fprintf(w, `{"items":[{"item":`+trackJSON+`},{"item":{"id":"ep","type":"episode"}},{"track":{"id":"","is_local":true}}],"next":%q}`,
				"1", "1", "1", g.srv.URL+"/playlists/p/items?offset=50")
			return
		}
		fmt.Fprintf(w, `{"items":[{"track":`+trackJSON+`}],"next":null}`, "2", "2", "2")
	}
	u, saved := g.user("r1")
	p, err := u.Playlist(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Minha" || p.Owner != "Eu" || len(p.Songs) != 2 || p.Skipped != 2 {
		t.Fatalf("Playlist = %+v", p)
	}
	s := p.Songs[0]
	if s.Title != "Música 1" || s.ISRC != "ISRC1" || s.AlbumArtist != "A" || len(s.Artists) != 2 || s.CoverURL != "grande" ||
		s.TrackNumber != 3 || s.TrackCount != 10 || s.ReleaseDate != "1999-01-02" || s.SpotifyID != "1" || p.Songs[1].SpotifyID != "2" {
		t.Errorf("song = %+v", s)
	}
	if g.tokens.Load() != 1 || len(*saved) != 1 || (*saved)[0] != "r2" {
		t.Errorf("um token só, e o refresh novo gravado: %d %v", g.tokens.Load(), *saved)
	}
}

func TestUserRefreshesOn401(t *testing.T) {
	g := newAPIRig(t)
	g.items = func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"items":[]}`)) }
	u, _ := g.user("r1")
	g.expired.Store(true)
	if _, err := u.Playlist(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if g.tokens.Load() != 2 {
		t.Errorf("tokens = %d, quero 2 (renovado depois do 401)", g.tokens.Load())
	}
}

func TestUserErrors(t *testing.T) {
	g := newAPIRig(t)
	for body, want := range map[string]error{
		`{"error":{"status":403,"message":"Forbidden"}}`:                                      ErrNotOwner,
		`{"error":{"status":403,"message":"User not registered in the Developer Dashboard"}}`: ErrNotAllowed,
	} {
		g.items = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); w.Write([]byte(body)) }
		g.refreshT = "r1"
		u, _ := g.user("r1")
		if _, err := u.Playlist(context.Background(), "p"); !errors.Is(err, want) {
			t.Errorf("%s: err = %v", body, err)
		}
	}
	u, _ := g.user("revogado")
	if _, err := u.Playlist(context.Background(), "p"); !errors.Is(err, ErrLoggedOut) {
		t.Errorf("refresh revogado: %v", err)
	}
}
