package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/DeathHapyness/spotGO/internal/config"
	"github.com/DeathHapyness/spotGO/internal/spotify"
)

type loginDeps struct {
	open     func(url string) error
	tokenURL string
	addr     string
	timeout  time.Duration
}

func defaultLoginDeps() loginDeps {
	return loginDeps{open: openBrowser, tokenURL: spotify.TokenURL, timeout: spotify.LoginTimeout}
}

func newLoginCmd(opts *globalOptions, d loginDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Entra na sua conta do Spotify (Premium) para ler playlists suas inteiras",
		Long: `Opcional. Faz login na sua conta pelo navegador (OAuth com PKCE), usando o app
configurado no spotgo auth. Depois disso, as playlists que são suas ou em que você
colabora são lidas pela API oficial: todas as faixas (sem o limite de 100 do embed)
e com os metadados do Spotify. O resto continua igual, sem a API.

Precisa de Spotify Premium (exigência do Spotify para apps de desenvolvedor) e da
Redirect URI ` + spotify.RedirectURI + ` cadastrada no app.

O login fica salvo no config.json (permissão 0600); spotgo logout o apaga.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			flags := config.Credentials{ClientID: opts.ClientID, ClientSecret: opts.ClientSecret}
			creds, _, err := config.Resolve(flags, os.Getenv, path)
			if err != nil {
				return err
			}
			return runLogin(cmd, d, path, creds.ClientID)
		},
	}
}

func runLogin(cmd *cobra.Command, d loginDeps, path, clientID string) error {
	ctx, out := cmd.Context(), cmd.OutOrStdout()
	l, err := spotify.NewLogin(clientID)
	if err != nil {
		return err
	}
	l.Addr = d.addr
	ln, err := l.Listen()
	if err != nil {
		return err
	}
	if d.addr != "" { // testes: porta livre qualquer
		l.Redirect = "http://" + ln.Addr().String() + "/callback"
	}
	u := l.URL()
	fmt.Fprintf(out, "Abrindo o navegador para o login no Spotify. Se ele não abrir, acesse:\n\n  %s\n\n", u)
	if d.open != nil {
		_ = d.open(u)
	}
	fmt.Fprintln(out, "Esperando o login...")
	code, err := l.Wait(ctx, ln, d.timeout)
	if err != nil {
		return err
	}
	_, refresh, err := spotify.ExchangeCode(ctx, httpClient, d.tokenURL, clientID, code, l.Verifier, l.Redirect)
	if err != nil {
		return fmt.Errorf("trocando o código do login: %w", err)
	}
	if refresh == "" {
		return errors.New("o Spotify não devolveu um refresh token")
	}
	if err := saveLogin(path, &config.Login{ClientID: clientID, RefreshToken: refresh}); err != nil {
		return err
	}
	fmt.Fprintf(out, "Login feito. As playlists suas agora vêm inteiras pela API (salvo em %s).\n", path)
	return nil
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Apaga o login do Spotify (as credenciais do spotgo auth continuam)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if _, ok := cfg.LoggedIn(); !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "Você não está logado.")
				return nil
			}
			if err := saveLogin(path, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Login apagado. Para revogar o acesso também no Spotify: https://www.spotify.com/account/apps/")
			return nil
		},
	}
}

// saveLogin troca só o login no config.json, mantendo as credenciais.
func saveLogin(path string, l *config.Login) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg.Login = l
	return config.Save(path, cfg)
}

func openBrowser(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}
