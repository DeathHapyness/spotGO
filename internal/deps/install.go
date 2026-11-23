package deps

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
)

const (
	envBinDir   = "SPOTGO_BIN_DIR"
	maxSumsSize = 1 << 20
)

type Installer struct {
	// HTTP nil = cliente sem timeout total; o ctx é quem cancela.
	HTTP     *http.Client
	Dir      string
	GOOS     string
	GOARCH   string
	Progress func(t Tool, done, total int64)
}

func ManagedDir() (string, error) {
	if d := os.Getenv(envBinDir); d != "" {
		return d, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("não achei a pasta de cache do usuário: %w", err)
	}
	return filepath.Join(dir, "spotgo", "bin"), nil
}

// Install baixa a ferramenta, confere o SHA-256 publicado e só então a coloca
// em Dir. Em qualquer erro, nada muda em Dir.
func (in *Installer) Install(ctx context.Context, t Tool) (string, error) {
	goos, goarch := in.GOOS, in.GOARCH
	if goos == "" {
		goos, goarch = runtime.GOOS, runtime.GOARCH
	}
	src, err := SourceFor(t, goos, goarch)
	if err != nil {
		return "", err
	}
	sumsData, err := in.get(ctx, src.SumsURL, maxSumsSize)
	if err != nil {
		return "", err
	}
	sums, err := ParseSums(sumsData)
	if err != nil {
		return "", fmt.Errorf("%s: %w", t, err)
	}
	asset, err := src.Pick(sums)
	if err != nil {
		return "", err
	}
	want, ok := sums[asset.Name]
	if !ok && len(sums) == 1 {
		for _, h := range sums {
			want = h
		}
	}

	if err := os.MkdirAll(in.Dir, 0o755); err != nil {
		return "", err
	}
	download, err := os.CreateTemp(in.Dir, "."+asset.Name+".tmp-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(download.Name())
	defer download.Close()

	got, err := in.download(ctx, t, asset.URL, download)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf("checksum do %s não confere (esperado %s, recebido %s); nada foi instalado", t, want, got)
	}

	bin, err := os.CreateTemp(in.Dir, "."+string(t)+".bin.tmp-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(bin.Name())
	defer bin.Close()

	if err := extract(download, asset, bin); err != nil {
		return "", fmt.Errorf("%s: %w", t, err)
	}
	if err := bin.Chmod(0o755); err != nil {
		return "", err
	}
	if err := bin.Sync(); err != nil {
		return "", err
	}
	if err := bin.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(in.Dir, BinaryName(t, goos))
	if err := os.Rename(bin.Name(), final); err != nil {
		return "", err
	}
	return final, nil
}

func (in *Installer) client() *http.Client {
	if in.HTTP != nil {
		return in.HTTP
	}
	return &http.Client{}
}

func (in *Installer) open(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := in.client().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

func (in *Installer) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := in.open(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, ctxErr(ctx, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: arquivo de checksums grande demais", url)
	}
	return b, nil
}

// download grava o corpo em dst calculando o SHA-256 ao mesmo tempo.
func (in *Installer) download(ctx context.Context, t Tool, url string, dst *os.File) (string, error) {
	resp, err := in.open(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	h := sha256.New()
	var w io.Writer = io.MultiWriter(dst, h)
	if in.Progress != nil {
		w = &progressWriter{w: w, report: func(n int64) { in.Progress(t, n, resp.ContentLength) }}
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return "", ctxErr(ctx, fmt.Errorf("baixando %s: %w", url, err))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

type progressWriter struct {
	w      io.Writer
	n      int64
	report func(int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	p.report(p.n)
	return n, err
}

func matchesMember(name, member string) bool {
	name = strings.ReplaceAll(name, `\`, "/")
	return name == member || strings.HasSuffix(name, "/"+member)
}

var errNoMember = errors.New("o binário não está dentro do arquivo baixado")

func extract(src *os.File, a Asset, dst io.Writer) error {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	switch a.Archive {
	case Raw:
		_, err := io.Copy(dst, src)
		return err

	case Zip:
		st, err := src.Stat()
		if err != nil {
			return err
		}
		zr, err := zip.NewReader(src, st.Size())
		if err != nil {
			return fmt.Errorf("zip inválido: %w", err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !matchesMember(f.Name, a.Member) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			_, err = io.Copy(dst, rc)
			return err
		}
		return fmt.Errorf("%w (%s)", errNoMember, a.Member)

	case TarXZ:
		return extractTarXZ(src, a.Member, dst)
	}
	return fmt.Errorf("formato de arquivo desconhecido")
}

func extractTarXZ(src io.Reader, member string, dst io.Writer) error {
	// O leitor de xz lê byte a byte; sem buffer, cada byte vira uma syscall.
	xr, err := xz.NewReader(bufio.NewReaderSize(src, 1<<20))
	if err != nil {
		return fmt.Errorf("xz inválido: %w", err)
	}
	tr := tar.NewReader(xr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%w (%s)", errNoMember, member)
		}
		if err != nil {
			return fmt.Errorf("tar inválido: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && matchesMember(hdr.Name, member) {
			_, err = io.Copy(dst, tr)
			return err
		}
	}
}
