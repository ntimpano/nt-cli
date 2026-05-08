// Package scripts_test drives install.sh as a black box to verify key
// behavioral scenarios without requiring network access or a real GitHub
// release. Each test exercises a slice of the installer using a controlled
// environment (fake binaries, stub servers, or early-exit flags).
package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptPath returns the absolute path to install.sh.
func installScriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(file), "install.sh")
}

// requireBash skips the test if bash is not available.
func requireBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

// requireJq skips the test if jq is not available.
func requireJq(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
}

// runInstallWithEnv runs install.sh with the given env overrides and returns
// combined stdout+stderr output plus the exit error (nil on success).
func runInstallWithEnv(t *testing.T, env []string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", installScriptPath(t))
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// fakeCurlEnv builds a fake environment where curl serves files from fileDir,
// uname reports linux/amd64, and NT_CLI_VERSION is set to avoid real network
// calls. Returns env slice and the fakeBinDir path.
func fakeCurlEnv(t *testing.T, fileDir string) (env []string, fakeBinDir string) {
	t.Helper()
	fakeBinDir = t.TempDir()

	// Fake curl: parse -o <dest> <url> and copy from fileDir.
	fakeCurl := filepath.Join(fakeBinDir, "curl")
	curlScript := `#!/usr/bin/env bash
dest=""; url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -fsSL|-fSL|-fsL|-fL) shift ;;
    -o) dest="$2"; shift 2 ;;
    *) url="$1"; shift ;;
  esac
done
filename="${url##*/}"
src="` + fileDir + `/${filename}"
if [[ -f "$src" ]]; then
  cp "$src" "$dest"
else
  echo "fake-curl: file not found: ${filename}" >&2; exit 22
fi
`
	must(t, os.WriteFile(fakeCurl, []byte(curlScript), 0755))

	// Fake uname — linux/amd64.
	fakeUname := filepath.Join(fakeBinDir, "uname")
	must(t, os.WriteFile(fakeUname, []byte(`#!/usr/bin/env bash
case "$1" in
  -s) echo "Linux" ;;
  -m) echo "x86_64" ;;
esac`), 0755))

	oldPath := os.Getenv("PATH")
	env = []string{
		"PATH=" + fakeBinDir + ":" + oldPath,
		"NT_CLI_VERSION=v0.0.0-test",
	}
	return env, fakeBinDir
}

func must(t *testing.T, err error, mode ...os.FileMode) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// buildFakeRelease creates a fake tarball + sha256sums.txt in dir, matching the
// linux/amd64 asset name. The tarball contains a minimal fake nt-cli binary and
// optionally the provided extraFiles map (filename -> content).
func buildFakeRelease(t *testing.T, dir string, extraFiles map[string][]byte) {
	t.Helper()
	requireBash(t)

	// Create a staging dir for the tarball contents.
	stage := t.TempDir()
	// Fake nt-cli binary (just a shell echo stub).
	must(t, os.WriteFile(filepath.Join(stage, "nt-cli"), []byte("#!/usr/bin/env bash\necho \"nt-cli stub $*\"\n"), 0755))
	for name, content := range extraFiles {
		must(t, os.WriteFile(filepath.Join(stage, name), content, 0644))
	}

	tarball := filepath.Join(dir, "nt-cli_linux_amd64.tar.gz")
	cmd := exec.Command("tar", "-czf", tarball, "-C", stage, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar failed: %v\n%s", err, out)
	}

	// Generate real sha256sum.
	var checkCmd *exec.Cmd
	if _, err := exec.LookPath("sha256sum"); err == nil {
		checkCmd = exec.Command("bash", "-c", "sha256sum nt-cli_linux_amd64.tar.gz > sha256sums.txt")
	} else {
		checkCmd = exec.Command("bash", "-c", "shasum -a 256 nt-cli_linux_amd64.tar.gz > sha256sums.txt")
	}
	checkCmd.Dir = dir
	if out, err := checkCmd.CombinedOutput(); err != nil {
		t.Fatalf("checksum generation failed: %v\n%s", err, out)
	}
}

// runInstall runs install.sh with HOME set to homeDir and the fakeCurl env,
// returning combined output and exit error.
func runInstall(t *testing.T, homeDir string, env []string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", installScriptPath(t))
	cmd.Env = append(env, "HOME="+homeDir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func ensureOpenCodeDir(t *testing.T, homeDir string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Join(homeDir, ".config", "opencode"), 0755))
}

// TestInstall_UnsupportedArch verifies the installer exits non-zero and prints
// a helpful message listing supported architectures when the arch is unknown.
func TestInstall_UnsupportedArch(t *testing.T) {
	requireBash(t)

	fakeUname := t.TempDir()
	fakeBin := filepath.Join(fakeUname, "uname")
	must(t, os.WriteFile(fakeBin, []byte(`#!/usr/bin/env bash
case "$1" in
  -s) echo "Linux" ;;
  -m) echo "mips64" ;;
  *)  /usr/bin/uname "$@" ;;
esac`), 0755))

	oldPath := os.Getenv("PATH")
	out, err := runInstallWithEnv(t, []string{"PATH=" + fakeUname + ":" + oldPath})

	if err == nil {
		t.Fatal("expected non-zero exit for unsupported arch, got success")
	}
	if !strings.Contains(out, "unsupported arch") {
		t.Errorf("expected 'unsupported arch' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "amd64") || !strings.Contains(out, "arm64") {
		t.Errorf("expected supported arch list in error message, got:\n%s", out)
	}
}

// TestInstall_UnsupportedOS verifies the installer exits non-zero with a
// message listing supported operating systems when the OS is unknown.
func TestInstall_UnsupportedOS(t *testing.T) {
	requireBash(t)

	fakeUname := t.TempDir()
	fakeBin := filepath.Join(fakeUname, "uname")
	must(t, os.WriteFile(fakeBin, []byte(`#!/usr/bin/env bash
case "$1" in
  -s) echo "FreeBSD" ;;
  -m) echo "x86_64" ;;
  *)  /usr/bin/uname "$@" ;;
esac`), 0755))

	oldPath := os.Getenv("PATH")
	out, err := runInstallWithEnv(t, []string{"PATH=" + fakeUname + ":" + oldPath})

	if err == nil {
		t.Fatal("expected non-zero exit for unsupported OS, got success")
	}
	if !strings.Contains(out, "unsupported os") {
		t.Errorf("expected 'unsupported os' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "linux") || !strings.Contains(out, "darwin") {
		t.Errorf("expected supported OS list in error message, got:\n%s", out)
	}
}

// TestInstall_MissingJq verifies the installer exits non-zero and prints an
// actionable install hint when jq is not found on PATH.
func TestInstall_MissingJq(t *testing.T) {
	requireBash(t)

	unamePath, err := exec.LookPath("uname")
	if err != nil {
		t.Skip("uname not found")
	}
	trPath, err := exec.LookPath("tr")
	if err != nil {
		t.Skip("tr not found")
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}

	fakeDir := t.TempDir()
	scriptPath := installScriptPath(t)

	for _, tool := range []struct{ name, src string }{
		{"uname", unamePath},
		{"tr", trPath},
		{"bash", bashPath},
	} {
		dst := filepath.Join(fakeDir, tool.name)
		if err := os.Symlink(tool.src, dst); err != nil {
			t.Fatalf("symlink %s: %v", tool.name, err)
		}
	}

	wrapper := "#!/usr/bin/env bash\nexport PATH=\"" + fakeDir + "\"\nexec bash \"" + scriptPath + "\"\n"
	wrapperPath := filepath.Join(fakeDir, "run.sh")
	must(t, os.WriteFile(wrapperPath, []byte(wrapper), 0755))

	cmd := exec.Command("bash", wrapperPath)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("expected non-zero exit for missing jq, got success")
	}
	if !strings.Contains(string(out), "jq") {
		t.Errorf("expected 'jq' in error message, got:\n%s", string(out))
	}
}

// TestInstall_ChecksumMismatch verifies the installer aborts and exits non-zero
// when the downloaded tarball does not match the expected SHA256.
func TestInstall_ChecksumMismatch(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	tarball := filepath.Join(fileDir, "nt-cli_linux_amd64.tar.gz")
	must(t, os.WriteFile(tarball, []byte("not a real tarball"), 0644))

	badHash := "0000000000000000000000000000000000000000000000000000000000000000"
	must(t, os.WriteFile(filepath.Join(fileDir, "sha256sums.txt"),
		[]byte(badHash+"  nt-cli_linux_amd64.tar.gz\n"), 0644))

	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstallWithEnv(t, env)

	if err == nil {
		t.Fatal("expected non-zero exit on checksum mismatch, got success")
	}
	if !strings.Contains(out, "checksum") {
		t.Errorf("expected 'checksum' in error output, got:\n%s", out)
	}
}

// TestInstall_Sha256Portability verifies that the installer source contains
// both sha256sum and shasum branches for cross-platform compatibility.
func TestInstall_Sha256Portability(t *testing.T) {
	requireBash(t)

	_, hasSha256sum := exec.LookPath("sha256sum")
	_, hasShasum := exec.LookPath("shasum")
	if hasSha256sum != nil && hasShasum != nil {
		t.Skip("neither sha256sum nor shasum available on this host")
	}

	scriptBytes, err := os.ReadFile(installScriptPath(t))
	if err != nil {
		t.Fatal(err)
	}
	src := string(scriptBytes)

	if !strings.Contains(src, "shasum") {
		t.Error("install.sh must contain a shasum fallback for macOS compatibility")
	}
	if !strings.Contains(src, "sha256sum") {
		t.Error("install.sh must contain sha256sum for Linux")
	}
}

// TestInstall_BinaryPlacement verifies that a successful install places the
// nt-cli binary in ~/.local/bin/nt-cli with executable permissions.
func TestInstall_BinaryPlacement(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, nil)

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	binPath := filepath.Join(homeDir, ".local", "bin", "nt-cli")
	info, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("binary not found at %s: %v", binPath, statErr)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("binary at %s is not executable", binPath)
	}
}

// TestInstall_PathHint verifies that the installer prints a PATH hint when
// ~/.local/bin is not in PATH.
func TestInstall_PathHint(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, nil)

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, fakeBinDir := fakeCurlEnv(t, fileDir)
	// Override PATH to not include ~/.local/bin.
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + fakeBinDir + ":/usr/bin:/bin"
		}
	}
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "PATH") {
		t.Errorf("expected PATH hint in output when ~/.local/bin not in PATH, got:\n%s", out)
	}
}

// TestInstall_OpencodeJsonMerge verifies that install.sh merges nt-* agents
// additively into an existing opencode.json, preserving non-nt-* keys.
func TestInstall_OpencodeJsonMerge(t *testing.T) {
	requireBash(t)
	requireJq(t)

	// Build a fake release bundle that includes .nt-cli-agents.json.
	agents := map[string]interface{}{
		"nt-test-agent": map[string]interface{}{"model": "test-model"},
	}
	agentsJSON, _ := json.Marshal(agents)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{
		".nt-cli-agents.json": agentsJSON,
	})

	// Existing opencode.json with a non-nt-* key that must be preserved.
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "opencode")
	must(t, os.MkdirAll(configDir, 0755))
	existing := map[string]interface{}{
		"theme": "dark",
		"agent": map[string]interface{}{
			"other-agent": map[string]interface{}{"model": "other"},
		},
	}
	existingJSON, _ := json.Marshal(existing)
	must(t, os.WriteFile(filepath.Join(configDir, "opencode.json"), existingJSON, 0644))

	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	// Read merged result.
	resultBytes, readErr := os.ReadFile(filepath.Join(configDir, "opencode.json"))
	if readErr != nil {
		t.Fatalf("could not read opencode.json after install: %v", readErr)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("opencode.json is not valid JSON after merge: %v", err)
	}

	// "theme" key must be preserved.
	if result["theme"] != "dark" {
		t.Errorf("non-nt-* key 'theme' was lost during merge; got: %v", result["theme"])
	}

	// nt-test-agent must be present.
	agents2, ok := result["agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("'agent' key missing or wrong type after merge")
	}
	if _, ok := agents2["nt-test-agent"]; !ok {
		t.Error("nt-test-agent not found in merged opencode.json")
	}
	// other-agent must be preserved.
	if _, ok := agents2["other-agent"]; !ok {
		t.Error("other-agent was removed during merge (must be preserved)")
	}
}

// TestInstall_OpencodeJsonBackup verifies that a timestamped backup of the
// original opencode.json is created before the merge.
func TestInstall_OpencodeJsonBackup(t *testing.T) {
	requireBash(t)
	requireJq(t)

	agents := map[string]interface{}{"nt-x": map[string]interface{}{}}
	agentsJSON, _ := json.Marshal(agents)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{".nt-cli-agents.json": agentsJSON})

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "opencode")
	must(t, os.MkdirAll(configDir, 0755))
	must(t, os.WriteFile(filepath.Join(configDir, "opencode.json"), []byte(`{"theme":"dark"}`), 0644))

	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	// At least one backup file must exist.
	entries, _ := filepath.Glob(filepath.Join(configDir, "opencode.json.bak.*"))
	if len(entries) == 0 {
		t.Error("expected a timestamped backup of opencode.json, found none")
	}
}

// TestInstall_IdempotentReinstall verifies that running the installer twice
// succeeds without errors (idempotent re-run).
func TestInstall_IdempotentReinstall(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, nil)

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, _ := fakeCurlEnv(t, fileDir)

	// First install.
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("first install failed: %v\n%s", err, out)
	}

	// Second install (idempotent).
	out, err = runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("second install (idempotent) failed: %v\n%s", err, out)
	}
}

// TestInstall_AgentsMdCopiedWhenAbsent verifies that AGENTS.md is copied to
// ~/.config/opencode/AGENTS.md when it does not yet exist.
func TestInstall_AgentsMdCopiedWhenAbsent(t *testing.T) {
	requireBash(t)
	requireJq(t)

	agentsContent := []byte("# AGENTS\ntest content\n")
	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{"AGENTS.md": agentsContent})

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	dst := filepath.Join(homeDir, ".config", "opencode", "AGENTS.md")
	got, readErr := os.ReadFile(dst)
	if readErr != nil {
		t.Fatalf("AGENTS.md not found at %s: %v", dst, readErr)
	}
	if string(got) != string(agentsContent) {
		t.Errorf("AGENTS.md content mismatch\nwant: %q\ngot:  %q", agentsContent, got)
	}
}

// TestInstall_AgentsMdSkippedWhenIdentical verifies that AGENTS.md is not
// overwritten (no backup created) when the bundled and existing files are identical.
func TestInstall_AgentsMdSkippedWhenIdentical(t *testing.T) {
	requireBash(t)
	requireJq(t)

	agentsContent := []byte("# AGENTS\nidentical content\n")
	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{"AGENTS.md": agentsContent})

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "opencode")
	must(t, os.MkdirAll(configDir, 0755))
	// Pre-place identical AGENTS.md.
	must(t, os.WriteFile(filepath.Join(configDir, "AGENTS.md"), agentsContent, 0644))

	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	// No backup should have been created.
	backups, _ := filepath.Glob(filepath.Join(configDir, "AGENTS.md.bak.*"))
	if len(backups) > 0 {
		t.Errorf("expected no backup when AGENTS.md is identical, but found: %v", backups)
	}
	// No warning in output.
	if strings.Contains(out, "WARNING") {
		t.Errorf("unexpected WARNING for identical AGENTS.md:\n%s", out)
	}
}

// TestInstall_AgentsMdBackupWhenDiffers verifies that when the existing
// AGENTS.md differs from the bundled one, a backup is created and a warning
// is printed.
func TestInstall_AgentsMdBackupWhenDiffers(t *testing.T) {
	requireBash(t)
	requireJq(t)

	bundledContent := []byte("# AGENTS\nnew content\n")
	existingContent := []byte("# AGENTS\nold content\n")

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{"AGENTS.md": bundledContent})

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config", "opencode")
	must(t, os.MkdirAll(configDir, 0755))
	must(t, os.WriteFile(filepath.Join(configDir, "AGENTS.md"), existingContent, 0644))

	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	// A backup must exist.
	backups, _ := filepath.Glob(filepath.Join(configDir, "AGENTS.md.bak.*"))
	if len(backups) == 0 {
		t.Error("expected backup of AGENTS.md when content differs, found none")
	}
	// WARNING must be printed.
	if !strings.Contains(out, "WARNING") {
		t.Errorf("expected WARNING when AGENTS.md differs, got:\n%s", out)
	}
	// File must be updated to bundled content.
	got, _ := os.ReadFile(filepath.Join(configDir, "AGENTS.md"))
	if string(got) != string(bundledContent) {
		t.Errorf("AGENTS.md not updated to bundled content after differ")
	}
}

// TestInstall_SuccessMessage verifies that the installer prints a success
// message containing the version string on successful install.
func TestInstall_SuccessMessage(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, nil)

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "installed successfully") {
		t.Errorf("expected success message, got:\n%s", out)
	}
	if !strings.Contains(out, "v0.0.0-test") {
		t.Errorf("expected version in success message, got:\n%s", out)
	}
}

func TestInstall_FailsFastWhenInitFails(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{
		"nt-cli": []byte("#!/usr/bin/env bash\nif [[ \"$1\" == \"init\" ]]; then\n  echo \"init failed\" >&2\n  exit 42\nfi\nexit 0\n"),
	})

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err == nil {
		t.Fatalf("expected install to fail when nt-cli init fails, got success:\n%s", out)
	}
	if !strings.Contains(out, "nt-cli init --non-interactive") {
		t.Fatalf("expected actionable init failure message, got:\n%s", out)
	}
}

func TestInstall_SmokeSuccessStillPasses(t *testing.T) {
	requireBash(t)
	requireJq(t)

	fileDir := t.TempDir()
	buildFakeRelease(t, fileDir, map[string][]byte{
		"nt-cli": []byte("#!/usr/bin/env bash\nif [[ \"$1\" == \"init\" ]]; then\n  exit 0\nfi\nexit 0\n"),
	})

	homeDir := t.TempDir()
	ensureOpenCodeDir(t, homeDir)
	env, _ := fakeCurlEnv(t, fileDir)
	out, err := runInstall(t, homeDir, env)
	if err != nil {
		t.Fatalf("expected successful install smoke test, got error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed successfully") {
		t.Fatalf("expected install success output, got:\n%s", out)
	}
}
