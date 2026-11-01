#!/bin/sh
set -eu

REPO="lucaswilliameufrasio/ai-usage-tracker"
TAG="latest"
BIN_NAME="ai-usage"
BIN_DIR="${HOME}/.local/bin"
FORCE=""
BASE_URL=""

usage() {
  cat <<USAGE
agent installer

Usage:
  agent-installer.sh [--tag <vX.Y.Z|latest>] [--bin-dir <path>] [--base-url <url>] [--force]

Options:
  --tag       Release tag (default: latest)
  --bin-dir   Install directory (default: ~/.local/bin)
  --base-url  Release directory that already holds the package and checksums.txt
  --force     Replace an existing install without asking
  -h, --help  Show this help
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --tag)
      [ "$#" -ge 2 ] || { echo "--tag requires a value" >&2; exit 1; }
      TAG="$2"
      shift 2
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || { echo "--bin-dir requires a value" >&2; exit 1; }
      BIN_DIR="$2"
      shift 2
      ;;
    --base-url)
      [ "$#" -ge 2 ] || { echo "--base-url requires a value" >&2; exit 1; }
      BASE_URL="$2"
      shift 2
      ;;
    --force)
      FORCE="yes"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo "tar is required" >&2; exit 1; }

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Linux) OS_NAME="linux" ;;
  Darwin) OS_NAME="darwin" ;;
  *)
    echo "Unsupported system: $OS" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH_NAME="amd64" ;;
  arm64|aarch64) ARCH_NAME="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

if [ -n "$BASE_URL" ]; then
  case "$BASE_URL" in
    http://*|https://*) ;;
    *)
      echo "Invalid base URL" >&2
      exit 1
      ;;
  esac
  BASE_URL="${BASE_URL%/}"
fi

if [ "$TAG" = "latest" ]; then
  if [ -n "$BASE_URL" ]; then
    echo "Pass --tag when using --base-url" >&2
    exit 1
  fi
  RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"
  TAG="$(printf '%s' "$RELEASE_JSON" | tr -d '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [ -n "$TAG" ] || { echo "Could not resolve the latest tag" >&2; exit 1; }
fi

printf '%s\n' "$TAG" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || {
  echo "Invalid stable release tag: $TAG" >&2
  exit 1
}

VERSION="${TAG#v}"
ASSET="${BIN_NAME}_${VERSION}_${OS_NAME}_${ARCH_NAME}.tar.gz"
if [ -z "$BASE_URL" ]; then
  BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
fi

TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t agent-install)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT HUP INT TERM

ASSET_FILE="${TMP_DIR}/${ASSET}"
CHECKSUM_FILE="${TMP_DIR}/checksums.txt"
echo "Downloading ${BASE_URL}/${ASSET}"
curl -fsSL "${BASE_URL}/${ASSET}" -o "$ASSET_FILE"
curl -fsSL "${BASE_URL}/checksums.txt" -o "$CHECKSUM_FILE"

EXPECTED_HASH="$(awk -v asset="$ASSET" '$2 == asset || $2 == "*" asset { print $1 }' "$CHECKSUM_FILE")"
case "$EXPECTED_HASH" in
  *[!0123456789abcdefABCDEF]*|"") echo "Checksum missing or invalid for $ASSET" >&2; exit 1 ;;
esac
[ "${#EXPECTED_HASH}" -eq 64 ] || { echo "Invalid checksum for $ASSET" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_HASH="$(sha256sum "$ASSET_FILE" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL_HASH="$(shasum -a 256 "$ASSET_FILE" | awk '{print $1}')"
else
  echo "sha256sum or shasum is required" >&2
  exit 1
fi
EXPECTED_HASH="$(printf '%s' "$EXPECTED_HASH" | tr 'A-F' 'a-f')"
ACTUAL_HASH="$(printf '%s' "$ACTUAL_HASH" | tr 'A-F' 'a-f')"
[ "$EXPECTED_HASH" = "$ACTUAL_HASH" ] || { echo "Checksum mismatch for $ASSET" >&2; exit 1; }
echo "Checksum verified"

tar -xf "$ASSET_FILE" -C "$TMP_DIR"
[ -f "${TMP_DIR}/${BIN_NAME}" ] || { echo "The package does not contain ${BIN_NAME}" >&2; exit 1; }
if [ -e "${BIN_DIR}/${BIN_NAME}" ] && [ "$FORCE" != "yes" ]; then
  if (printf 'Replace %s? [y/N] ' "${BIN_DIR}/${BIN_NAME}" >/dev/tty) 2>/dev/null; then
    read -r answer </dev/tty || answer=n
    case "$answer" in
      [yY]*) ;;
      *) echo "Install cancelled"; exit 1 ;;
    esac
  else
    echo "No terminal; the existing install was kept" >&2
    echo "Use --force to replace it" >&2
    exit 1
  fi
fi
mkdir -p "$BIN_DIR"
install -m 0755 "${TMP_DIR}/${BIN_NAME}" "${BIN_DIR}/${BIN_NAME}"

echo "Agent ${TAG} installed at ${BIN_DIR}/${BIN_NAME}"
case ":${PATH}:" in
  *:"${BIN_DIR}":*) ;;
  *) echo "Add ${BIN_DIR} to PATH: export PATH=\"${BIN_DIR}:\$PATH\"" ;;
esac
