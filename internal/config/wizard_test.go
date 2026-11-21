package config

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const (
	validID     = "0123456789abcdef0123456789abcdef"
	validSecret = "fedcba9876543210fedcba9876543210"
)

var errFakeInvalid = errors.New("fake: credencial inválida")

func isFakeInvalid(err error) bool { return errors.Is(err, errFakeInvalid) }

type fakeValidator struct {
	results []error
	calls   int
}

func (f *fakeValidator) validate(_ context.Context, _ Credentials) error {
	f.calls++
	if len(f.results) == 0 {
		return nil
	}
	err := f.results[0]
	f.results = f.results[1:]
	return err
}

func newPrompter(ids string, secrets ...string) (Prompter, *bytes.Buffer) {
	var out bytes.Buffer
	return Prompter{
		In:  bufio.NewReader(strings.NewReader(ids)),
		Out: &out,
		ReadSecret: func() (string, error) {
			if len(secrets) == 0 {
				return "", errors.New("sem mais secrets")
			}
			s := secrets[0]
			secrets = secrets[1:]
			return s, nil
		},
	}, &out
}

func TestWizardHappyPath(t *testing.T) {
	p, _ := newPrompter(" "+validID+"\n", validSecret+"\n")
	v := &fakeValidator{}

	got, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Credentials{validID, validSecret}) {
		t.Fatalf("got %+v", got)
	}
	if v.calls != 1 {
		t.Fatalf("validate chamado %d vezes, quero 1", v.calls)
	}
}

func TestWizardPrintsDashboardURL(t *testing.T) {
	p, out := newPrompter(validID+"\n", validSecret)
	v := &fakeValidator{}
	if _, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "https://developer.spotify.com/dashboard") {
		t.Fatalf("instruções sem a URL do dashboard:\n%s", out)
	}
}

func TestWizardBadFormatRetriesWithoutNetwork(t *testing.T) {
	p, out := newPrompter("abc\n"+validID+"\n", validSecret, validSecret)
	v := &fakeValidator{}

	if _, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid); err != nil {
		t.Fatal(err)
	}
	if v.calls != 1 {
		t.Fatalf("formato inválido não deve ir à rede; validate chamado %d vezes", v.calls)
	}
	if !strings.Contains(out.String(), "32") {
		t.Fatalf("mensagem deveria explicar o formato (32 caracteres):\n%s", out)
	}
}

func TestWizardInvalidThenValid(t *testing.T) {
	p, _ := newPrompter(validID+"\n"+validID+"\n", validSecret, validSecret)
	v := &fakeValidator{results: []error{errFakeInvalid, nil}}

	if _, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid); err != nil {
		t.Fatal(err)
	}
	if v.calls != 2 {
		t.Fatalf("validate chamado %d vezes, quero 2", v.calls)
	}
}

func TestWizardNetworkErrorAborts(t *testing.T) {
	p, _ := newPrompter(validID+"\n"+validID+"\n", validSecret, validSecret)
	netErr := errors.New("dial tcp: connection refused")
	v := &fakeValidator{results: []error{netErr}}

	_, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid)
	if !errors.Is(err, netErr) {
		t.Fatalf("quero o erro de rede embrulhado, veio %v", err)
	}
	if v.calls != 1 {
		t.Fatalf("erro de rede não deve repetir; validate chamado %d vezes", v.calls)
	}
}

func TestWizardGivesUpAfter3(t *testing.T) {
	ids := strings.Repeat(validID+"\n", 4)
	p, _ := newPrompter(ids, validSecret, validSecret, validSecret, validSecret)
	v := &fakeValidator{results: []error{errFakeInvalid, errFakeInvalid, errFakeInvalid, nil}}

	if _, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid); err == nil {
		t.Fatal("quero erro depois de 3 tentativas")
	}
	if v.calls != 3 {
		t.Fatalf("validate chamado %d vezes, quero 3", v.calls)
	}
}

func TestWizardStdinClosed(t *testing.T) {
	p, _ := newPrompter("")
	v := &fakeValidator{}

	if _, err := RunWizard(context.Background(), p, v.validate, isFakeInvalid); err == nil {
		t.Fatal("stdin fechado deve dar erro, não loop")
	}
}

func TestEnsureNonInteractive(t *testing.T) {
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	in := strings.NewReader(validID + "\n")
	p := Prompter{In: bufio.NewReader(in), Out: &bytes.Buffer{}}
	path := filepath.Join(t.TempDir(), "config.json")

	_, err := Ensure(context.Background(), Credentials{}, path, false, p, (&fakeValidator{}).validate, isFakeInvalid)
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("quero ErrNoCredentials, veio %v", err)
	}
	if in.Len() != len(validID)+1 {
		t.Fatal("modo não interativo não pode ler stdin")
	}
}

func TestEnsureSavesAfterWizard(t *testing.T) {
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	p, out := newPrompter(validID+"\n", validSecret)
	path := filepath.Join(t.TempDir(), "spotgo", "config.json")

	got, err := Ensure(context.Background(), Credentials{}, path, true, p, (&fakeValidator{}).validate, isFakeInvalid)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Spotify != got || got != (Credentials{validID, validSecret}) {
		t.Fatalf("salvo %+v, retornado %+v", cfg.Spotify, got)
	}
	if !strings.Contains(out.String(), path) {
		t.Fatalf("saída deveria dizer onde salvou:\n%s", out)
	}
}

func TestEnsureUsesExistingWithoutPrompt(t *testing.T) {
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	path := filepath.Join(t.TempDir(), "config.json")
	want := Credentials{validID, validSecret}
	if err := Save(path, Config{Spotify: want}); err != nil {
		t.Fatal(err)
	}
	p, _ := newPrompter("")
	v := &fakeValidator{}

	got, err := Ensure(context.Background(), Credentials{}, path, true, p, v.validate, isFakeInvalid)
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	if v.calls != 0 {
		t.Fatal("credenciais existentes não devem passar pelo assistente")
	}
}

func TestMask(t *testing.T) {
	tests := map[string]string{
		"abcd1234efgh5678": "abcd…5678",
		"short":            "********",
		"12345678":         "********",
		"":                 "********",
	}
	for in, want := range tests {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, quero %q", in, got, want)
		}
	}
}
