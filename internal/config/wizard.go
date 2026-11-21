package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxAttempts = 3

type Validator func(ctx context.Context, c Credentials) error

type Prompter struct {
	In         *bufio.Reader
	Out        io.Writer
	ReadSecret func() (string, error)
}

const instructions = `
Configuração das credenciais do Spotify
---------------------------------------
O spotgo usa as SUAS credenciais da API do Spotify (é grátis):

  1. Acesse https://developer.spotify.com/dashboard e faça login
  2. Clique em "Create app" (nome e descrição à sua escolha;
     em Redirect URI use http://127.0.0.1:8888/callback)
  3. Abra o app, copie o "Client ID" e clique em "View client secret"

`

func RunWizard(ctx context.Context, p Prompter, validate Validator, isInvalid func(error) bool) (Credentials, error) {
	fmt.Fprint(p.Out, instructions)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Credentials{}, err
		}

		fmt.Fprint(p.Out, "Client ID: ")
		id, err := readLine(p.In)
		if err != nil {
			return Credentials{}, err
		}
		if !looksLikeKey(id) {
			fmt.Fprintln(p.Out, "  O Client ID tem 32 caracteres hexadecimais (0-9, a-f). Tente de novo.")
			continue
		}

		fmt.Fprint(p.Out, "Client Secret (não aparece enquanto você digita): ")
		secret, err := p.ReadSecret()
		if err != nil {
			return Credentials{}, fmt.Errorf("lendo o Client Secret: %w", err)
		}
		secret = strings.TrimSpace(secret)
		if !looksLikeKey(secret) {
			fmt.Fprintln(p.Out, "  O Client Secret tem 32 caracteres hexadecimais (0-9, a-f). Tente de novo.")
			continue
		}

		creds := Credentials{ClientID: id, ClientSecret: secret}
		fmt.Fprintln(p.Out, "Validando com o Spotify...")
		err = validate(ctx, creds)
		if err == nil {
			fmt.Fprintln(p.Out, "Credenciais válidas.")
			return creds, nil
		}
		if !isInvalid(err) {
			return Credentials{}, fmt.Errorf("não consegui validar as credenciais: %w", err)
		}
		fmt.Fprintf(p.Out, "  O Spotify recusou essas credenciais (%v). Confira e tente de novo.\n", err)
	}
	return Credentials{}, errors.New("muitas tentativas inválidas; rode `spotgo auth` para tentar de novo")
}

func Ensure(ctx context.Context, flags Credentials, path string, interactive bool, p Prompter, validate Validator, isInvalid func(error) bool) (Credentials, error) {
	creds, _, err := Resolve(flags, os.Getenv, path)
	if !errors.Is(err, ErrNoCredentials) {
		return creds, err
	}
	if !interactive {
		return Credentials{}, err
	}

	creds, err = RunWizard(ctx, p, validate, isInvalid)
	if err != nil {
		return Credentials{}, err
	}
	if err := Save(path, Config{Spotify: creds}); err != nil {
		return Credentials{}, err
	}
	fmt.Fprintf(p.Out, "Credenciais salvas em %s\n\n", path)
	return creds, nil
}

func Mask(s string) string {
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "…" + s[len(s)-4:]
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return "", fmt.Errorf("entrada encerrada antes de terminar a configuração: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func looksLikeKey(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
