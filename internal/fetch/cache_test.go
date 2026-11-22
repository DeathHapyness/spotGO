package fetch

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheHit(t *testing.T) {
	srv, n := sequence(t, status(200))
	u, _ := url.Parse(srv.URL)
	c, rec := newTestClient(Options{
		CacheDir: t.TempDir(),
		Limits:   map[string]time.Duration{u.Host: time.Hour},
	})
	req := Request{URL: srv.URL, CacheTTL: time.Hour}

	first, err := c.Get(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Get(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("chamadas ao servidor = %d, quero 1", n.Load())
	}
	if string(first) != string(second) {
		t.Fatalf("corpos diferentes: %q × %q", first, second)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("hit de cache não deve passar pelo limitador: sleeps=%v", rec.calls)
	}
}

func TestCacheExpired(t *testing.T) {
	srv, n := sequence(t, status(200))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	clock := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return clock }
	req := Request{URL: srv.URL, CacheTTL: time.Hour}

	if _, err := c.Get(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Hour)
	if _, err := c.Get(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Fatalf("chamadas = %d, quero 2 (cache expirado)", n.Load())
	}
}

func TestCacheSkipsErrors(t *testing.T) {
	srv, n := sequence(t, status(404), status(200))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	req := Request{URL: srv.URL, CacheTTL: time.Hour}

	if _, err := c.Get(context.Background(), req); err == nil {
		t.Fatal("quero o 404")
	}
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if n.Load() != 2 {
		t.Fatalf("chamadas = %d, quero 2 (404 não cacheado, 200 cacheado)", n.Load())
	}
}

func TestCacheCorruptFileIsMiss(t *testing.T) {
	srv, n := sequence(t, status(200))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	path := c.cachePath(srv.URL)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}

	body, err := c.Get(context.Background(), Request{URL: srv.URL, CacheTTL: time.Hour})
	if err != nil || string(body) != "corpo" {
		t.Fatalf("Get = %q, %v", body, err)
	}
	if n.Load() != 1 {
		t.Fatalf("arquivo corrompido deveria ir à rede")
	}
	if info, err := os.Stat(path); err != nil || info.Size() <= 8 {
		t.Fatalf("cache não foi regravado: %v %v", info, err)
	}
}

func TestNoCacheWhenTTLZero(t *testing.T) {
	srv, n := sequence(t, status(200))
	dir := t.TempDir()
	c, _ := newTestClient(Options{CacheDir: dir})

	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), Request{URL: srv.URL}); err != nil {
			t.Fatal(err)
		}
	}
	if n.Load() != 2 {
		t.Fatalf("chamadas = %d, quero 2", n.Load())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("TTL 0 não deveria criar arquivos: %v", entries)
	}
}

func TestCacheFilePermissions(t *testing.T) {
	srv, _ := sequence(t, status(200))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})

	if _, err := c.Get(context.Background(), Request{URL: srv.URL, CacheTTL: time.Hour}); err != nil {
		t.Fatal(err)
	}
	path := c.cachePath(srv.URL)
	for p, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s: %o, quero %o", p, info.Mode().Perm(), want)
		}
	}
}
