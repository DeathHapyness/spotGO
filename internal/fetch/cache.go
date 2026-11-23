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

// cachePath sem corpo continua sendo sha256(URL), para o cache antigo valer.
func (c *Client) cachePath(r Request) string {
	key := r.URL
	if r.Body != nil {
		key += "\x00" + r.Method + "\x00" + string(r.Body)
	}
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.cacheDir, h[:2], h)
}

func (c *Client) cacheEnabled(r Request) bool {
	return r.CacheTTL > 0 && c.cacheDir != ""
}

func (c *Client) cacheGet(r Request) ([]byte, bool) {
	data, err := os.ReadFile(c.cachePath(r))
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
func (c *Client) cachePut(r Request, body []byte) {
	path := c.cachePath(r)
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
