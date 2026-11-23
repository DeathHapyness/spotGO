package deps

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeTarXZ(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(xw)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	xw.Close()
	return buf.Bytes()
}

const ffName = "ffmpeg-n9.0-latest-linux64-gpl-9.0.tar.xz"

type release struct {
	files map[string][]byte // caminho → corpo
	block map[string]bool   // manda 1 MB e trava até o cliente desistir
}

func fakeReleases(t *testing.T, rel release) *Installer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rel.block[r.URL.Path] {
			w.Header().Set("Content-Length", "100000000")
			w.Write(make([]byte, 1<<20))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		b, ok := rel.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	old := [3]string{ytdlpBase, denoBase, ffmpegBase}
	ytdlpBase, denoBase, ffmpegBase = srv.URL+"/ytdlp", srv.URL+"/deno", srv.URL+"/ffmpeg"
	t.Cleanup(func() { ytdlpBase, denoBase, ffmpegBase = old[0], old[1], old[2] })
	return &Installer{Dir: filepath.Join(t.TempDir(), "bin"), GOOS: "linux", GOARCH: "amd64"}
}

func ytdlpRelease(body []byte, hash string) release {
	return release{files: map[string][]byte{
		"/ytdlp/SHA2-256SUMS": []byte(hash + "  yt-dlp_linux\n" + h64 + "  yt-dlp.exe\n"),
		"/ytdlp/yt-dlp_linux": body,
	}}
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestInstallRaw(t *testing.T) {
	body := []byte("#!/bin/sh\necho 2026.10.26\n")
	in := fakeReleases(t, ytdlpRelease(body, sha(body)))
	path, err := in.Install(context.Background(), YTDLP)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	st, _ := os.Stat(path)
	if path != filepath.Join(in.Dir, "yt-dlp") || !bytes.Equal(got, body) || st.Mode().Perm() != 0o755 {
		t.Fatalf("path = %s, modo %v, conteúdo %q", path, st.Mode(), got)
	}
	if names := dirEntries(t, in.Dir); len(names) != 1 {
		t.Fatalf("pasta = %v", names)
	}
}

func TestInstallZip(t *testing.T) {
	z := makeZip(t, map[string]string{"deno": "DENO", "LICENSE": "x"})
	in := fakeReleases(t, release{files: map[string][]byte{
		"/deno/deno-x86_64-unknown-linux-gnu.zip.sha256sum": []byte(sha(z) + "  deno-x86_64-unknown-linux-gnu.zip\n"),
		"/deno/deno-x86_64-unknown-linux-gnu.zip":           z,
	}})
	path, err := in.Install(context.Background(), Deno)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "DENO" {
		t.Fatalf("conteúdo = %q", got)
	}
	if names := dirEntries(t, in.Dir); len(names) != 1 || names[0] != "deno" {
		t.Fatalf("pasta = %v", names)
	}
}

func TestInstallTarXZ(t *testing.T) {
	x := makeTarXZ(t, map[string]string{
		"ffmpeg-n9.0-latest-linux64-gpl-9.0/bin/ffprobe": "PROBE",
		"ffmpeg-n9.0-latest-linux64-gpl-9.0/bin/ffmpeg":  "FFMPEG",
		"ffmpeg-n9.0-latest-linux64-gpl-9.0/LICENSE.txt": "x",
	})
	in := fakeReleases(t, release{files: map[string][]byte{
		"/ffmpeg/checksums.sha256": []byte(sha(x) + "  " + ffName + "\n"),
		"/ffmpeg/" + ffName:        x,
	}})
	path, err := in.Install(context.Background(), FFmpeg)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "FFMPEG" {
		t.Fatalf("conteúdo = %q", got)
	}
	if names := dirEntries(t, in.Dir); len(names) != 1 || names[0] != "ffmpeg" {
		t.Fatalf("pasta = %v", names)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	in := fakeReleases(t, ytdlpRelease([]byte("malicioso"), h64))
	os.MkdirAll(in.Dir, 0o755)
	os.WriteFile(filepath.Join(in.Dir, "yt-dlp"), []byte("antigo"), 0o755)

	_, err := in.Install(context.Background(), YTDLP)
	if err == nil || !strings.Contains(err.Error(), "não confere") {
		t.Fatalf("err = %v", err)
	}
	if names := dirEntries(t, in.Dir); len(names) != 1 {
		t.Fatalf("sobrou temporário: %v", names)
	}
	if got, _ := os.ReadFile(filepath.Join(in.Dir, "yt-dlp")); string(got) != "antigo" {
		t.Fatalf("o antigo foi trocado: %q", got)
	}
}

func TestInstallCanceled(t *testing.T) {
	rel := ytdlpRelease(nil, h64)
	rel.block = map[string]bool{"/ytdlp/yt-dlp_linux": true}
	in := fakeReleases(t, rel)
	ctx, cancel := context.WithCancel(context.Background())
	in.Progress = func(_ Tool, done, _ int64) {
		if done >= 1<<20 {
			cancel()
		}
	}
	_, err := in.Install(ctx, YTDLP)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, quero context.Canceled", err)
	}
	if names := dirEntries(t, in.Dir); len(names) != 0 {
		t.Fatalf("sobrou arquivo: %v", names)
	}
}

func TestInstallMemberMissing(t *testing.T) {
	z := makeZip(t, map[string]string{"README": "x"})
	in := fakeReleases(t, release{files: map[string][]byte{
		"/deno/deno-x86_64-unknown-linux-gnu.zip.sha256sum": []byte(sha(z) + "  deno-x86_64-unknown-linux-gnu.zip\n"),
		"/deno/deno-x86_64-unknown-linux-gnu.zip":           z,
	}})
	_, err := in.Install(context.Background(), Deno)
	if err == nil || !strings.Contains(err.Error(), "deno") {
		t.Fatalf("err = %v", err)
	}
	if names := dirEntries(t, in.Dir); len(names) != 0 {
		t.Fatalf("sobrou arquivo: %v", names)
	}
}

func TestInstall404(t *testing.T) {
	rel := ytdlpRelease(nil, h64)
	delete(rel.files, "/ytdlp/yt-dlp_linux")
	in := fakeReleases(t, rel)
	_, err := in.Install(context.Background(), YTDLP)
	if err == nil || !strings.Contains(err.Error(), "/ytdlp/yt-dlp_linux") {
		t.Fatalf("err = %v", err)
	}
}

func TestManagedDirEnv(t *testing.T) {
	t.Setenv("SPOTGO_BIN_DIR", "/tmp/x/bin")
	if d, err := ManagedDir(); err != nil || d != "/tmp/x/bin" {
		t.Fatalf("ManagedDir = %q, %v", d, err)
	}
}

type countingReader struct {
	r     io.Reader
	reads int
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.reads++
	return c.r.Read(p)
}

// Sem buffer, o leitor de xz faz uma leitura por byte; com o ffmpeg real
// (150 MB) a extração levava mais de 2 minutos.
func TestExtractTarXZBuffered(t *testing.T) {
	big := strings.Repeat("ffmpeg-binario-", 1<<16) // ~1 MB
	x := makeTarXZ(t, map[string]string{"pkg/bin/ffmpeg": big})
	src := &countingReader{r: bytes.NewReader(x)}
	var out bytes.Buffer
	if err := extractTarXZ(src, "bin/ffmpeg", &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != big {
		t.Fatalf("conteúdo extraído errado (%d bytes)", out.Len())
	}
	if src.reads > 100 {
		t.Fatalf("%d leituras para %d bytes comprimidos; falta buffer", src.reads, len(x))
	}
}
