package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp .env: %v", err)
	}
	return p
}

func TestLoadDotEnv_MissingFileIsNoOp(t *testing.T) {
	n, err := LoadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatalf("missing file should be no-op, got err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 vars, got %d", n)
	}
}

func TestLoadDotEnv_ParsesKeyValueSkipsCommentsAndBlanks(t *testing.T) {
	path := writeEnvFile(t, "# comment\n\nFIGMA_TOKEN=figd_abc123\n  # indented comment\nFOO_BAR = baz \n")
	// ensure clean slate
	os.Unsetenv("FIGMA_TOKEN")
	os.Unsetenv("FOO_BAR")
	t.Cleanup(func() { os.Unsetenv("FIGMA_TOKEN"); os.Unsetenv("FOO_BAR") })

	n, err := LoadDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 vars set, got %d", n)
	}
	if got := os.Getenv("FIGMA_TOKEN"); got != "figd_abc123" {
		t.Fatalf("FIGMA_TOKEN = %q, want figd_abc123", got)
	}
	if got := os.Getenv("FOO_BAR"); got != "baz" {
		t.Fatalf("FOO_BAR = %q, want baz (trimmed)", got)
	}
}

func TestLoadDotEnv_ExistingEnvWins(t *testing.T) {
	path := writeEnvFile(t, "FIGMA_TOKEN=from_file\n")
	t.Setenv("FIGMA_TOKEN", "from_shell")

	n, err := LoadDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 vars set (shell wins), got %d", n)
	}
	if got := os.Getenv("FIGMA_TOKEN"); got != "from_shell" {
		t.Fatalf("FIGMA_TOKEN = %q, want from_shell (env must win)", got)
	}
}

func TestLoadDotEnv_StripsQuotesAndExportPrefix(t *testing.T) {
	path := writeEnvFile(t, "export QUOTED_D=\"dval\"\nQUOTED_S='sval'\n")
	os.Unsetenv("QUOTED_D")
	os.Unsetenv("QUOTED_S")
	t.Cleanup(func() { os.Unsetenv("QUOTED_D"); os.Unsetenv("QUOTED_S") })

	if _, err := LoadDotEnv(path); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := os.Getenv("QUOTED_D"); got != "dval" {
		t.Fatalf("QUOTED_D = %q, want dval (quotes + export stripped)", got)
	}
	if got := os.Getenv("QUOTED_S"); got != "sval" {
		t.Fatalf("QUOTED_S = %q, want sval", got)
	}
}
