package fetch

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const cacheHeaderLen = 8

func DefaultCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("não achei a pasta de cache do usuário: %w", err)
	}
	return filepath.Join(dir, "spotgo", "http"), nil
}

func (c *Client) cachePath(url string) string {
	sum := sha256.Sum256([]byte(url))
	key := hex.EncodeToString(sum[:])
	return filepath.Join(c.cacheDir, key[:2], key)
}

func (c *Client) cacheEnabled(r Request) bool {
	return r.CacheTTL > 0 && c.cacheDir != ""
}

func (c *Client) cacheGet(r Request) ([]byte, bool) {
	data, err := os.ReadFile(c.cachePath(r.URL))
	if err != nil || len(data) < cacheHeaderLen {
		return nil, false
	}
	written := time.Unix(0, int64(binary.BigEndian.Uint64(data[:cacheHeaderLen])))
	age := c.now().Sub(written)
	if age < 0 || age >= r.CacheTTL {
		return nil, false
	}
	return data[cacheHeaderLen:], true
}

// cachePut é melhor esforço: uma falha ao gravar o cache não pode derrubar
// uma requisição que já deu certo.
func (c *Client) cachePut(url string, body []byte) {
	path := c.cachePath(url)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())

	var header [cacheHeaderLen]byte
	binary.BigEndian.PutUint64(header[:], uint64(c.now().UnixNano()))
	if _, err := tmp.Write(header[:]); err != nil {
		tmp.Close()
		return
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	os.Rename(tmp.Name(), path)
}
