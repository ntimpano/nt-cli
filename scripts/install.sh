#!/usr/bin/env bash
set -euo pipefail

die() { echo "ERROR: $*" >&2; exit 1; }

on_init_error() {
  local line=${1:-unknown}
  echo "ERROR: nt-cli init --non-interactive failed near line ${line}." >&2
  echo "ERROR: Fix the init error above, then run '$HOME/.local/bin/nt-cli init --non-interactive'." >&2
}

# 1. OS/arch detection
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported arch: $ARCH (supported: x86_64/amd64, aarch64/arm64)" ;;
esac
[[ "$OS" == "linux" || "$OS" == "darwin" ]] \
  || die "unsupported os: $OS (supported: linux, darwin)"

# OpenCode prerequisite check
if [[ ! -d "$HOME/.config/opencode" ]]; then
  die "OpenCode is required. Install from https://opencode.ai"
fi

# 2. Dependency checks (fail fast)
command -v jq >/dev/null || die "jq required: brew install jq | apt install jq | dnf install jq"
command -v curl >/dev/null || die "curl required"

# 3. Version resolution
REPO="ntimpano/nt-cli"
VERSION="${NT_CLI_VERSION:-$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | jq -r .tag_name)}"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"
TARBALL="nt-cli_${OS}_${ARCH}.tar.gz"

# 4. Download
TMPDIR=$(mktemp -d); trap 'rm -rf "$TMPDIR"' EXIT
curl -fsSL -o "$TMPDIR/$TARBALL" "$BASE/$TARBALL"
curl -fsSL -o "$TMPDIR/sha256sums.txt" "$BASE/sha256sums.txt"

# 5. SHA256 validation
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMPDIR" && grep " ${TARBALL}$" sha256sums.txt | sha256sum -c -) \
    || die "checksum mismatch — download may be corrupted, aborting"
elif command -v shasum >/dev/null 2>&1; then
  (cd "$TMPDIR" && grep " ${TARBALL}$" sha256sums.txt | shasum -a 256 -c -) \
    || die "checksum mismatch — download may be corrupted, aborting"
else
  die "neither sha256sum nor shasum found — cannot verify download integrity"
fi

# 6. Extract
tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

# 7. Binary install to ~/.local/bin (no sudo)
mkdir -p "$HOME/.local/bin"
install -m 0755 "$TMPDIR/nt-cli" "$HOME/.local/bin/nt-cli"

# 8. opencode.json merge (additive, nt-* only, atomic)
BUNDLE="$TMPDIR/.nt-cli-agents.json"
OPENCODE_JSON="$HOME/.config/opencode/opencode.json"
if [[ -f "$BUNDLE" ]]; then
  TS=$(date -u +%Y%m%dT%H%M%SZ)
  mkdir -p "$(dirname "$OPENCODE_JSON")"
  if [[ -f "$OPENCODE_JSON" ]]; then
    cp -p "$OPENCODE_JSON" "$OPENCODE_JSON.bak.$TS"
    TMP=$(mktemp "$OPENCODE_JSON.XXXXXX")
    jq -s '
      .[0] as $existing
      | .[1] as $bundle
      | $existing
      | .agent = ((.agent // {}) + ($bundle | with_entries(select(.key | startswith("nt-")))))
    ' "$OPENCODE_JSON" "$BUNDLE" > "$TMP"
    mv -f "$TMP" "$OPENCODE_JSON"
  else
    cp "$BUNDLE" "$OPENCODE_JSON"
  fi
fi

# 9. AGENTS.md diff-check
BUNDLED_AGENTS="$TMPDIR/AGENTS.md"
DST_AGENTS="$HOME/.config/opencode/AGENTS.md"
if [[ -f "$BUNDLED_AGENTS" ]]; then
  TS="${TS:-$(date -u +%Y%m%dT%H%M%SZ)}"
  mkdir -p "$(dirname "$DST_AGENTS")"
  if [[ ! -f "$DST_AGENTS" ]]; then
    cp "$BUNDLED_AGENTS" "$DST_AGENTS"
  elif ! cmp -s "$BUNDLED_AGENTS" "$DST_AGENTS"; then
    cp -p "$DST_AGENTS" "$DST_AGENTS.bak.$TS"
    cp "$BUNDLED_AGENTS" "$DST_AGENTS"
    echo "WARNING: AGENTS.md differed from bundled version; backup at $DST_AGENTS.bak.$TS"
  fi
fi

# 10. Run nt-cli init (idempotent)
trap 'on_init_error $LINENO' ERR
"$HOME/.local/bin/nt-cli" init --non-interactive
trap - ERR

# 11. PATH hint
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) echo "Add to PATH: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac

echo "nt-cli ${VERSION} installed successfully."
