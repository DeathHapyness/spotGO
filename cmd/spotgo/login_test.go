package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/DeathHapyness/spotGO/internal/config"
)

func loginRig(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(envConfigPath, path)
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvClientSecret, "")
	config.Save(path, config.Config{Spotify: config.Credentials{ClientID: "cid", ClientSecret: "sec"}})
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.PostForm.Get("code") != "abc" || r.PostForm.Get("code_verifier") == "" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"access_token":"a","expires_in":3600,"refresh_token":"rt"}`))
	}))
	t.Cleanup(tok.Close)
	return path, tok
}

func execCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestLoginAndLogout(t *testing.T) {
	path, tok := loginRig(t)
	d := loginDeps{tokenURL: tok.URL, addr: "127.0.0.1:0", timeout: 5 * time.Second}
	d.open = func(raw string) error {
		u, _ := url.Parse(raw)
		q := u.Query()
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?code=abc&state=" + q.Get("state"))
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	out, err := execCmd(t, newLoginCmd(&globalOptions{}, d))
	if err != nil {
		t.Fatal(err, out)
	}
	cfg, _ := config.Load(path)
	if l, ok := cfg.LoggedIn(); !ok || l.RefreshToken != "rt" || l.ClientID != "cid" || cfg.Spotify.ClientSecret != "sec" {
		t.Fatalf("config = %+v", cfg)
	}

	out, _ = execCmd(t, newAuthCmd(&globalOptions{}), "--status")
	if !strings.Contains(out, "Login:     sim") {
		t.Errorf("status:\n%s", out)
	}

	if out, err := execCmd(t, newLogoutCmd()); err != nil || !strings.Contains(out, "Login apagado") {
		t.Fatal(err, out)
	}
	cfg, _ = config.Load(path)
	if _, ok := cfg.LoggedIn(); ok || cfg.Spotify.ClientID != "cid" {
		t.Errorf("logout tem que manter as credenciais: %+v", cfg)
	}
	if out, _ := execCmd(t, newLogoutCmd()); !strings.Contains(out, "não está logado") {
		t.Errorf("2º logout: %s", out)
	}
}

func TestLoginNeedsCredentials(t *testing.T) {
	path, _ := loginRig(t)
	config.Delete(path)
	if _, err := execCmd(t, newLoginCmd(&globalOptions{}, loginDeps{addr: "127.0.0.1:0"})); err == nil || !strings.Contains(err.Error(), "spotgo auth") {
		t.Errorf("err = %v", err)
	}
}
