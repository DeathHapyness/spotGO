package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	EnvClientID     = "SPOTGO_CLIENT_ID"
	EnvClientSecret = "SPOTGO_CLIENT_SECRET"
)

type Source string

const (
	SourceFlags Source = "flags"
	SourceEnv   Source = "env"
	SourceFile  Source = "file"
)

var ErrNoCredentials = errors.New(`nenhuma credencial do Spotify configurada.

  1. Crie um app grátis em https://developer.spotify.com/dashboard
  2. Rode: spotgo auth

ou use --client-id/--client-secret ou ` + EnvClientID + `/` + EnvClientSecret)

// Resolve usa a primeira fonte completa (flags → env → arquivo). Uma fonte com
// só um dos campos é erro: nunca mistura ID de uma fonte com secret de outra.
func Resolve(flags Credentials, getenv func(string) string, path string) (Credentials, Source, error) {
	candidates := []struct {
		src            Source
		creds          Credentials
		idName, secret string
	}{
		{SourceFlags, flags, "--client-id", "--client-secret"},
		{SourceEnv, Credentials{getenv(EnvClientID), getenv(EnvClientSecret)}, EnvClientID, EnvClientSecret},
	}

	for _, c := range candidates {
		creds := trim(c.creds)
		if creds.Complete() {
			return creds, c.src, nil
		}
		if creds.ClientID != "" || creds.ClientSecret != "" {
			return Credentials{}, "", fmt.Errorf("%s e %s devem ser usados juntos", c.idName, c.secret)
		}
	}

	cfg, err := Load(path)
	if err != nil {
		return Credentials{}, "", err
	}
	creds := trim(cfg.Spotify)
	if creds.Complete() {
		return creds, SourceFile, nil
	}
	if creds.ClientID != "" || creds.ClientSecret != "" {
		return Credentials{}, "", fmt.Errorf("%s está incompleto; rode `spotgo auth`", path)
	}
	return Credentials{}, "", ErrNoCredentials
}

func trim(c Credentials) Credentials {
	return Credentials{
		ClientID:     strings.TrimSpace(c.ClientID),
		ClientSecret: strings.TrimSpace(c.ClientSecret),
	}
}
