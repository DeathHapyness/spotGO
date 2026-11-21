package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/config"
)

const (
	testID     = "0123456789abcdef0123456789abcdef"
	testSecret = "fedcba9876543210fedcba9876543210"
)

func setupConfig(t *testing.T, creds *config.Credentials) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spotgo", "config.json")
	t.Setenv(envConfigPath, path)
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvClientSecret, "")
	if creds != nil {
		if err := config.Save(path, config.Config{Spotify: *creds}); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestAuthStatusMasksSecret(t *testing.T) {
	path := setupConfig(t, &config.Credentials{ClientID: testID, ClientSecret: testSecret})

	out, err := runCmd(t, "auth", "--status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{config.Mask(testID), string(config.SourceFile), path} {
		if !strings.Contains(out, want) {
			t.Errorf("saída sem %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, testSecret) || strings.Contains(out, testID) {
		t.Fatalf("--status vazou credencial:\n%s", out)
	}
}

func TestAuthStatusWithoutCredentials(t *testing.T) {
	setupConfig(t, nil)

	out, err := runCmd(t, "auth", "--status")
	if err != nil {
		t.Fatalf("--status sem credenciais não é erro: %v", err)
	}
	if !strings.Contains(out, "developer.spotify.com/dashboard") {
		t.Fatalf("deveria explicar como configurar:\n%s", out)
	}
}

func TestAuthClear(t *testing.T) {
	path := setupConfig(t, &config.Credentials{ClientID: testID, ClientSecret: testSecret})

	if _, err := runCmd(t, "auth", "--clear"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("arquivo ainda existe: %v", err)
	}
}

func TestAuthNonInteractiveFails(t *testing.T) {
	setupConfig(t, nil)

	_, err := runCmd(t, "auth")
	if err == nil || !strings.Contains(err.Error(), config.EnvClientID) {
		t.Fatalf("sem terminal deve falhar indicando a alternativa por env; veio %v", err)
	}
}

func TestAuthStatusAndClearAreExclusive(t *testing.T) {
	setupConfig(t, nil)

	if _, err := runCmd(t, "auth", "--status", "--clear"); err == nil {
		t.Fatal("--status e --clear juntos deveria ser erro")
	}
}
