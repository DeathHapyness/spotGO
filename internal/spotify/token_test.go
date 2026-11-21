package spotify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/config"
)

var testCreds = config.Credentials{ClientID: "my-id", ClientSecret: "my-secret"}

func TestRequestTokenOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método %s, quero POST", r.Method)
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "my-id" || secret != "my-secret" {
			t.Errorf("basic auth = %q/%q/%v", id, secret, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.PostForm.Get("grant_type"); got != "client_credentials" {
			t.Errorf("grant_type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	before := time.Now()
	tok, err := RequestToken(context.Background(), srv.Client(), srv.URL, testCreds)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
	lo, hi := before.Add(3590*time.Second), time.Now().Add(3600*time.Second)
	if tok.ExpiresAt.Before(lo) || tok.ExpiresAt.After(hi) {
		t.Errorf("ExpiresAt = %v, quero entre %v e %v", tok.ExpiresAt, lo, hi)
	}
}

func TestRequestTokenInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_client","error_description":"Invalid client"}`))
	}))
	defer srv.Close()

	_, err := RequestToken(context.Background(), srv.Client(), srv.URL, testCreds)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("quero ErrInvalidCredentials, veio %v", err)
	}
}

func TestRequestTokenServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := RequestToken(context.Background(), srv.Client(), srv.URL, testCreds)
	if err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("503 deve ser erro comum, não credencial inválida; veio %v", err)
	}
}

func TestRequestTokenNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // ninguém escutando mais nesse endereço

	_, err := RequestToken(context.Background(), http.DefaultClient, url, testCreds)
	if err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("falha de rede não pode virar credencial inválida; veio %v", err)
	}
}
