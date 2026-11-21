// Package config lê e grava a configuração e resolve as credenciais do Spotify.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func (c Credentials) Complete() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

type Config struct {
	Spotify Credentials `json:"spotify"`
}

const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("não achei a pasta de configuração do usuário: %w", err)
	}
	return filepath.Join(dir, "spotgo", "config.json"), nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("lendo %s: %w", path, err)
	}

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s está corrompido (%v); rode `spotgo auth` para recriá-lo", path, err)
	}
	return c, nil
}

// Save grava de forma atômica (temporário + rename): uma queda no meio não corrompe o arquivo antigo.
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("criando %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("criando arquivo temporário: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	return nil
}

func Delete(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
