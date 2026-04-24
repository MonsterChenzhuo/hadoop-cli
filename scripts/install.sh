#!/usr/bin/env bash
# hadoop-cli installer / upgrader.
# Re-run the same command to upgrade to the latest release.
#
# Env overrides:
#   VERSION=v0.1.2        pin a specific release (default: latest)
#   PREFIX=/usr/local/bin install directory for the binary
#   SKILL_DIR=~/.hadoop-cli/skills  install directory for bundled skills
#   NO_SUDO=1             never use sudo; fail if PREFIX is not writable
#   REPO=MonsterChenzhuo/hadoop-cli  override repo slug

set -euo pipefail

REPO="${REPO:-MonsterChenzhuo/hadoop-cli}"
PREFIX="${PREFIX:-/usr/local/bin}"
SKILL_DIR="${SKILL_DIR:-$HOME/.hadoop-cli/skills}"
VERSION="${VERSION:-}"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*" >&2; }
warn()  { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()   { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"; }
need curl
need tar
need uname

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) die "unsupported OS: $os (only linux/darwin)";;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported arch: $arch (only amd64/arm64)";;
esac

if [ -z "$VERSION" ]; then
  info "resolving latest release from github.com/$REPO"
  # Prefer the /releases/latest redirect: no API rate limit and no pipe-to-grep
  # SIGPIPE trap (which in `set -o pipefail` mode killed the old flow with
  # `curl: (23) Failure writing output to destination`).
  redirect=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPO}/releases/latest" 2>/dev/null || true)
  VERSION="${redirect##*/}"
  # Fall back to the GitHub API if the redirect shape changed or the host only
  # reaches api.github.com (some mirrored environments).
  if [ -z "$VERSION" ] || [ "$VERSION" = "latest" ]; then
    api_resp=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || true)
    VERSION=$(awk -F'"' '/"tag_name":/ { print $4; exit }' <<<"$api_resp")
  fi
  [ -n "$VERSION" ] || die "could not determine latest release tag; pin with VERSION=vX.Y.Z"
fi
ver_no_v="${VERSION#v}"

tmpdir=$(mktemp -d); trap 'rm -rf "$tmpdir"' EXIT
archive="hadoop-cli_${ver_no_v}_${os}_${arch}.tar.gz"
checksums="hadoop-cli_${ver_no_v}_checksums.txt"
base="https://github.com/${REPO}/releases/download/${VERSION}"

info "downloading ${archive}"
curl -fsSL "${base}/${archive}"    -o "${tmpdir}/${archive}"
curl -fsSL "${base}/${checksums}"  -o "${tmpdir}/${checksums}" || warn "checksums file not found, skipping verification"

if [ -s "${tmpdir}/${checksums}" ]; then
  info "verifying checksum"
  expected=$(awk -v f="$archive" '$2==f {print $1}' "${tmpdir}/${checksums}")
  [ -n "$expected" ] || die "no checksum entry for ${archive}"
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "${tmpdir}/${archive}" | awk '{print $1}')
  else
    actual=$(shasum -a 256 "${tmpdir}/${archive}" | awk '{print $1}')
  fi
  [ "$expected" = "$actual" ] || die "checksum mismatch (expected $expected, got $actual)"
fi

info "extracting"
tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"
[ -x "${tmpdir}/hadoop-cli" ] || die "binary not found in archive"

sudo_cmd=""
if [ ! -w "$PREFIX" ] && [ "$(id -u)" -ne 0 ]; then
  if [ "${NO_SUDO:-0}" = "1" ]; then
    die "PREFIX=$PREFIX not writable and NO_SUDO=1"
  fi
  need sudo
  sudo_cmd="sudo"
fi

info "installing binary to ${PREFIX}/hadoop-cli"
$sudo_cmd install -d "$PREFIX"
$sudo_cmd install -m 0755 "${tmpdir}/hadoop-cli" "${PREFIX}/hadoop-cli"

if [ -d "${tmpdir}/skills" ]; then
  info "installing skills to ${SKILL_DIR}"
  mkdir -p "$SKILL_DIR"
  # mirror the extracted skills tree, overwriting existing files
  (cd "${tmpdir}/skills" && tar -cf - .) | (cd "$SKILL_DIR" && tar -xf -)
fi

installed_version=$("${PREFIX}/hadoop-cli" --version 2>/dev/null || echo "$VERSION")
info "done: ${installed_version}"
info "run: hadoop-cli --help"
