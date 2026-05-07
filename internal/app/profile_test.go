package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Language != "es" || cfg.Tone != "argentino" || cfg.Verbosity != "short" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.AskBeforeMutation || !cfg.ContextAutoswitch {
		t.Fatalf("expected bool defaults true, got %+v", cfg)
	}
}

func TestValidate(t *testing.T) {
	valid := Defaults()
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid defaults, got %v", err)
	}

	cases := []struct {
		name   string
		mutate func(c *ProfileConfig)
		want   string
	}{
		{
			name: "invalid language",
			mutate: func(c *ProfileConfig) {
				c.Language = "pt"
			},
			want: "language",
		},
		{
			name: "invalid tone",
			mutate: func(c *ProfileConfig) {
				c.Tone = "aggressive"
			},
			want: "tone",
		},
		{
			name: "invalid verbosity",
			mutate: func(c *ProfileConfig) {
				c.Verbosity = "tiny"
			},
			want: "verbosity",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected validation error")
			}
			msg := err.Error()
			if !strings.Contains(strings.ToLower(msg), tc.want) {
				t.Fatalf("expected error to mention %q, got %q", tc.want, msg)
			}
			if !strings.Contains(msg, "valid:") {
				t.Fatalf("expected valid values hint, got %q", msg)
			}
		})
	}
}

func TestSaveAtomicWriteAndCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Defaults()
	cfg.Language = "en"
	cfg.Tone = "neutral"

	if err := cfg.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path, err := ProfilePath()
	if err != nil {
		t.Fatalf("ProfilePath: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"language": "en"`) || !strings.Contains(text, `"tone": "neutral"`) {
		t.Fatalf("saved content mismatch: %s", text)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".profile-") && strings.HasSuffix(e.Name(), ".json") {
			t.Fatalf("unexpected temp file left behind: %s", e.Name())
		}
	}
}

func TestLoadProfileFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	gotMissing := LoadProfile()
	if gotMissing != Defaults() {
		t.Fatalf("missing file should return defaults, got %+v", gotMissing)
	}

	path, err := ProfilePath()
	if err != nil {
		t.Fatalf("ProfilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	gotMalformed := LoadProfile()
	_ = w.Close()
	os.Stderr = orig
	warn, _ := io.ReadAll(r)
	_ = r.Close()

	if gotMalformed != Defaults() {
		t.Fatalf("malformed file should return defaults, got %+v", gotMalformed)
	}
	if !strings.Contains(string(warn), "warning") {
		t.Fatalf("expected warning on malformed profile, got %q", string(warn))
	}

	valid := Defaults()
	valid.Language = "en"
	if err := valid.Save(); err != nil {
		t.Fatalf("save valid: %v", err)
	}
	loaded := LoadProfile()
	if loaded.Language != "en" {
		t.Fatalf("expected loaded profile, got %+v", loaded)
	}
}

func TestRunInitProfileNonInteractive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out, errOut bytes.Buffer
	code := runInitProfile(
		[]string{"--language=en", "--tone=neutral"},
		strings.NewReader(""),
		&out,
		&errOut,
		false,
	)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, errOut.String())
	}

	loaded := LoadProfile()
	if loaded.Language != "en" || loaded.Tone != "neutral" {
		t.Fatalf("expected overrides applied, got %+v", loaded)
	}
	if loaded.Verbosity != "short" || !loaded.AskBeforeMutation || !loaded.ContextAutoswitch {
		t.Fatalf("expected remaining defaults, got %+v", loaded)
	}
}

func TestRunInitProfileExistingNoForceUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := Defaults()
	original.Language = "en"
	if err := original.Save(); err != nil {
		t.Fatalf("pre-save: %v", err)
	}
	path, _ := ProfilePath()
	before, _ := os.ReadFile(path)

	var out, errOut bytes.Buffer
	code := runInitProfile(nil, strings.NewReader(""), &out, &errOut, false)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, errOut.String())
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("profile should remain unchanged without --force")
	}
}

func TestRunInitProfileInvalidFlagValueNoWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out, errOut bytes.Buffer
	code := runInitProfile(
		[]string{"--non-interactive", "--tone=aggressive"},
		strings.NewReader(""),
		&out,
		&errOut,
		false,
	)
	if code == 0 {
		t.Fatalf("expected non-zero for invalid tone")
	}
	path, _ := ProfilePath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no profile file written, err=%v", err)
	}
}
