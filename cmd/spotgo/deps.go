package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DeathHapyness/spotGO/internal/config"
	"github.com/DeathHapyness/spotGO/internal/spotify"
)

const envConfigPath = "SPOTGO_CONFIG"

var httpClient = &http.Client{Timeout: 15 * time.Second}

func configPath() (string, error) {
	if p := os.Getenv(envConfigPath); p != "" {
		return p, nil
	}
	return config.DefaultPath()
}

func stdinTerminal(cmd *cobra.Command) (*os.File, bool) {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil, false
	}
	return f, true
}

func newPrompter(cmd *cobra.Command) config.Prompter {
	in := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()
	return config.Prompter{
		In:         in,
		Out:        out,
		ReadSecret: secretReader(cmd, in, out),
	}
}

func secretReader(cmd *cobra.Command, in *bufio.Reader, out io.Writer) func() (string, error) {
	return func() (string, error) {
		if f, ok := stdinTerminal(cmd); ok {
			b, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(out)
			return string(b), err
		}
		return in.ReadString('\n')
	}
}

func validateCredentials(ctx context.Context, c config.Credentials) error {
	_, err := spotify.RequestToken(ctx, httpClient, spotify.TokenURL, c)
	return err
}

func isInvalidCredentials(err error) bool {
	return errors.Is(err, spotify.ErrInvalidCredentials)
}
