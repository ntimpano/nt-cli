// Package scripts_test drives the opencode-mcp-dev.sh wrapper as a black box
// to verify the host profile toggle (PR4 of the nt-cli rollout slice).
//
// The wrapper resolves NTCLI_PROFILE into one of {shadow, pilot}. The
// resolved profile is observable via two paths:
//   - --print-profile: prints the resolved profile to stdout and exits 0
//     without launching the MCP process. This is the test surface.
//   - normal launch: emits "ntcli-mcp-dev profile=<name>" to stderr before
//     execing nt-cli (covered by the launch tests below using --print-profile
//     so we never spawn a long-running MCP server in CI).
package scripts_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// wrapperPath returns the absolute path to scripts/opencode-mcp-dev.sh
// independent of the working directory the test runner uses.
func wrapperPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	return filepath.Join(filepath.Dir(file), "opencode-mcp-dev.sh")
}

// runWrapper invokes the wrapper with the given args and env overrides.
// It returns stdout, stderr, and the exit code. The PATH is preserved so
// bash and standard utilities remain available.
func runWrapper(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(wrapperPath(t), args...)
	// Start from a clean env to make tests deterministic, then add PATH.
	baseEnv := []string{"PATH=" + getPath()}
	cmd.Env = append(baseEnv, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("wrapper failed to start: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func getPath() string {
	out, err := exec.Command("bash", "-c", "echo -n $PATH").Output()
	if err != nil {
		return "/usr/bin:/bin"
	}
	return string(out)
}

// TestHostProfile_Default proves: when NTCLI_PROFILE is unset, the wrapper
// resolves to "shadow" (the safe default for the rollout's shadow phase).
// Spec mapping: G5/G6 docs visibility — operators must be able to see which
// profile is active.
func TestHostProfile_Default(t *testing.T) {
	stdout, stderr, code := runWrapper(t, nil, "--print-profile")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	got := strings.TrimSpace(stdout)
	if got != "shadow" {
		t.Errorf("default profile: expected %q, got %q", "shadow", got)
	}
}

// TestHostProfile_Shadow proves explicit shadow selection works.
func TestHostProfile_Shadow(t *testing.T) {
	stdout, _, code := runWrapper(t, []string{"NTCLI_PROFILE=shadow"}, "--print-profile")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if got := strings.TrimSpace(stdout); got != "shadow" {
		t.Errorf("shadow: expected %q, got %q", "shadow", got)
	}
}

// TestHostProfile_Pilot proves: NTCLI_PROFILE=pilot resolves to "pilot",
// the documented pilot profile that makes nt-cli the canonical backend.
// Spec scenario: "Pilot profile makes nt-cli the canonical memory backend".
func TestHostProfile_Pilot(t *testing.T) {
	stdout, _, code := runWrapper(t, []string{"NTCLI_PROFILE=pilot"}, "--print-profile")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if got := strings.TrimSpace(stdout); got != "pilot" {
		t.Errorf("pilot: expected %q, got %q", "pilot", got)
	}
}

// TestHostProfile_Invalid proves: unknown profile values fail loudly with
// an error message that names the offending value AND lists the valid set.
// This is the safety property — operators cannot silently end up in an
// undefined profile state.
func TestHostProfile_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"typo", "shadwo"},
		{"empty_explicit", ""},
		{"unknown", "production"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := []string{"NTCLI_PROFILE=" + tc.value}
			// Empty env-set value: bash treats unset and empty differently;
			// our wrapper must reject empty explicit assignment as invalid
			// only when the variable is set but blank. To keep the test
			// deterministic we always set the var; for the "empty_explicit"
			// case the wrapper sees NTCLI_PROFILE="" which should fall
			// through to the default "shadow". So the "empty_explicit"
			// case is actually a positive case — adjust expectations.
			stdout, stderr, code := runWrapper(t, env, "--print-profile")
			if tc.value == "" {
				if code != 0 {
					t.Fatalf("empty NTCLI_PROFILE should default to shadow, got exit %d (stderr=%q)", code, stderr)
				}
				if got := strings.TrimSpace(stdout); got != "shadow" {
					t.Errorf("empty value should resolve to shadow, got %q", got)
				}
				return
			}
			if code == 0 {
				t.Fatalf("expected non-zero exit for invalid profile %q, got 0 (stdout=%q)", tc.value, stdout)
			}
			if !strings.Contains(stderr, tc.value) {
				t.Errorf("stderr must name the offending value %q; got %q", tc.value, stderr)
			}
			if !strings.Contains(stderr, "shadow") || !strings.Contains(stderr, "pilot") {
				t.Errorf("stderr must list valid profiles {shadow, pilot}; got %q", stderr)
			}
		})
	}
}

// TestHostProfile_LaunchEmitsMarker proves: a normal launch (without
// --print-profile) emits a structured stderr marker identifying the
// resolved profile, matching the NTCLI_MCP_DEBUG observability pattern.
//
// We use NTCLI_PROFILE_DRYRUN=1 to short-circuit before exec'ing the real
// MCP process so the test stays fast and hermetic. The marker is written
// BEFORE the dryrun gate so this is a real assertion of the launch path.
func TestHostProfile_LaunchEmitsMarker(t *testing.T) {
	env := []string{
		"NTCLI_PROFILE=pilot",
		"NTCLI_PROFILE_DRYRUN=1",
	}
	_, stderr, code := runWrapper(t, env)
	if code != 0 {
		t.Fatalf("dryrun launch: expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	want := "ntcli-mcp-dev profile=pilot"
	if !strings.Contains(stderr, want) {
		t.Errorf("expected stderr to contain %q, got %q", want, stderr)
	}
}

// TestHostProfile_LaunchDefaultMarker proves the marker is emitted even
// when the default profile is used (no NTCLI_PROFILE set). This is what
// makes the active profile discoverable in production logs.
func TestHostProfile_LaunchDefaultMarker(t *testing.T) {
	env := []string{"NTCLI_PROFILE_DRYRUN=1"}
	_, stderr, code := runWrapper(t, env)
	if code != 0 {
		t.Fatalf("dryrun launch (default): expected exit 0, got %d (stderr=%q)", code, stderr)
	}
	want := "ntcli-mcp-dev profile=shadow"
	if !strings.Contains(stderr, want) {
		t.Errorf("expected stderr to contain %q, got %q", want, stderr)
	}
}
