#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
VERSION="${GITHUB_REF_NAME:-dev}"

mkdir -p "${DIST_DIR}"
rm -f "${DIST_DIR}"/nt-cli_*.tar.gz "${DIST_DIR}/sha256sums.txt"

# Partial artifact cleanup: if any target fails, remove dist/ output so no
# incomplete release set is left behind.
_cleanup_on_fail() {
  echo "build failed — removing partial artifacts from ${DIST_DIR}" >&2
  rm -f "${DIST_DIR}"/nt-cli_*.tar.gz "${DIST_DIR}/sha256sums.txt"
}
trap '_cleanup_on_fail' ERR

build_target() {
  local goos="$1"
  local goarch="$2"
  local stage_dir

  stage_dir="$(mktemp -d)"

  (
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
      go build -ldflags="-s -w -X main.version=${VERSION}" -o "${stage_dir}/nt-cli" ./cmd/nt-cli
  )

  chmod +x "${stage_dir}/nt-cli"

  cp "${ROOT_DIR}/.nt-cli-agents.json" "${stage_dir}/" 2>/dev/null || true
  cp "${ROOT_DIR}/AGENTS.md" "${stage_dir}/" 2>/dev/null || true
  cp "${ROOT_DIR}/README.md" "${stage_dir}/" 2>/dev/null || true
  cp -R "${ROOT_DIR}/prompts" "${stage_dir}/" 2>/dev/null || true

  tar -czf "${DIST_DIR}/nt-cli_${goos}_${goarch}.tar.gz" -C "${stage_dir}" .
  rm -rf "${stage_dir}"
}

if [[ -n "${GOOS:-}" || -n "${GOARCH:-}" ]]; then
  : "${GOOS:?GOOS is required when GOARCH is set}"
  : "${GOARCH:?GOARCH is required when GOOS is set}"
  build_target "${GOOS}" "${GOARCH}"
else
  build_target linux amd64
  build_target linux arm64
  build_target darwin amd64
  build_target darwin arm64
fi

if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "${DIST_DIR}"
    sha256sum nt-cli_*.tar.gz > sha256sums.txt
  )
elif command -v shasum >/dev/null 2>&1; then
  (
    cd "${DIST_DIR}"
    shasum -a 256 nt-cli_*.tar.gz > sha256sums.txt
  )
else
  echo "error: neither sha256sum nor shasum is available" >&2
  exit 1
fi
