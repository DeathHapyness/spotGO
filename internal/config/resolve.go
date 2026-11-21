package config

import (
	"errors"
	"fmt"
	"strings"
)

// Variáveis de ambiente aceitas.
const (
	EnvClientID     = "SPOTGO_CLIENT_ID"
	EnvClientSecret = "SPOTGO_CLIENT_SECRET"
)

// Source diz de onde as credenciais vieram.
type Source string

const (
	SourceFlags Source = "flags"
	SourceEnv   Source = "env"
	SourceFile  Source = "file"
)

// ErrNoCredentials indica que nenhuma fonte tem credenciais.
// Quem chama decide se abre o assistente ou só mostra o erro.
var ErrNoCredentials = errors.New(`nenhuma credencial do Spotify configurada.

  1. Crie um app grátis em https://developer.spotify.com/dashboard
  2. Rode: spotgo auth

ou use --client-id/--client-secret ou ` + EnvClientID + `/` + EnvClientSecret)

// Resolve procura as credenciais nesta ordem: flags, ambiente e arquivo.
// A primeira fonte que tiver os dois campos vence. Se uma fonte tiver só um
// dos campos, isso é erro: misturar o ID de uma fonte com o secret de outra
// geraria falhas difíceis de entender.
//
// getenv é injetado (normalmente os.Getenv) para os testes não dependerem
// do ambiente real.
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
