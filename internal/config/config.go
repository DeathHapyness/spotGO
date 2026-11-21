// Package config lê e grava a configuração do spotgo e decide de onde vêm
// as credenciais da API do Spotify.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Credentials são o Client ID e o Client Secret de um app do Spotify.
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Complete informa se os dois campos estão preenchidos.
func (c Credentials) Complete() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

// Config é o conteúdo de config.json.
type Config struct {
	Spotify Credentials `json:"spotify"`
}

const (
	dirPerm  fs.FileMode = 0o700 // só o dono entra na pasta
	filePerm fs.FileMode = 0o600 // só o dono lê o secret
)

// DefaultPath devolve <UserConfigDir>/spotgo/config.json
// (no Linux, ~/.config/spotgo/config.json).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("não achei a pasta de configuração do usuário: %w", err)
	}
	return filepath.Join(dir, "spotgo", "config.json"), nil
}

// Load lê a configuração. Arquivo inexistente não é erro: devolve Config vazio.
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

// Save grava a configuração de forma atômica: escreve num arquivo temporário
// na mesma pasta e só então renomeia. Se o programa cair no meio, o arquivo
// antigo continua intacto.
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("criando %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	// CreateTemp já cria o arquivo com permissão 0600.
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("criando arquivo temporário: %w", err)
	}
	// Se algo falhar antes do Rename, o temporário é removido.
	// Depois do Rename, o Remove falha em silêncio (o arquivo não existe mais).
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil { // garante que chegou ao disco
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

// Delete apaga o arquivo de configuração. Se ele não existir, não é erro.
func Delete(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
