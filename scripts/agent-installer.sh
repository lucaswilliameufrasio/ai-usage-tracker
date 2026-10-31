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
instalador do agente

Uso:
  agent-installer.sh [--tag <vX.Y.Z|latest>] [--bin-dir <caminho>] [--base-url <url>] [--force]

Opções:
  --tag       Tag da release (padrão: latest)
  --bin-dir   Diretório de instalação (padrão: ~/.local/bin)
  --base-url  Diretório da release que já contém o pacote e checksums.txt
  --force     Substitui uma instalação existente sem perguntar
  -h, --help  Mostra esta ajuda
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --tag)
      [ "$#" -ge 2 ] || { echo "--tag precisa de um valor" >&2; exit 1; }
      TAG="$2"
      shift 2
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || { echo "--bin-dir precisa de um valor" >&2; exit 1; }
      BIN_DIR="$2"
      shift 2
      ;;
    --base-url)
      [ "$#" -ge 2 ] || { echo "--base-url precisa de um valor" >&2; exit 1; }
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
      echo "Argumento desconhecido: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

command -v curl >/dev/null 2>&1 || { echo "curl é obrigatório" >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo "tar é obrigatório" >&2; exit 1; }

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Linux) OS_NAME="linux" ;;
  Darwin) OS_NAME="darwin" ;;
  *)
    echo "Sistema não suportado: $OS" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH_NAME="amd64" ;;
  arm64|aarch64) ARCH_NAME="arm64" ;;
  *)
    echo "Arquitetura não suportada: $ARCH" >&2
    exit 1
    ;;
esac

if [ -n "$BASE_URL" ]; then
  case "$BASE_URL" in
    http://*|https://*) ;;
    *)
      echo "URL base inválida" >&2
      exit 1
      ;;
  esac
  BASE_URL="${BASE_URL%/}"
fi

if [ "$TAG" = "latest" ]; then
  if [ -n "$BASE_URL" ]; then
    echo "Informe --tag ao usar --base-url" >&2
    exit 1
  fi
  RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"
  TAG="$(printf '%s' "$RELEASE_JSON" | tr -d '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [ -n "$TAG" ] || { echo "Não foi possível resolver a tag latest" >&2; exit 1; }
fi

printf '%s\n' "$TAG" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || {
  echo "Tag de release estável inválida: $TAG" >&2
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
echo "Baixando ${BASE_URL}/${ASSET}"
curl -fsSL "${BASE_URL}/${ASSET}" -o "$ASSET_FILE"
curl -fsSL "${BASE_URL}/checksums.txt" -o "$CHECKSUM_FILE"

EXPECTED_HASH="$(awk -v asset="$ASSET" '$2 == asset || $2 == "*" asset { print $1 }' "$CHECKSUM_FILE")"
case "$EXPECTED_HASH" in
  *[!0123456789abcdefABCDEF]*|"") echo "Checksum ausente ou inválido para $ASSET" >&2; exit 1 ;;
esac
[ "${#EXPECTED_HASH}" -eq 64 ] || { echo "Checksum inválido para $ASSET" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_HASH="$(sha256sum "$ASSET_FILE" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL_HASH="$(shasum -a 256 "$ASSET_FILE" | awk '{print $1}')"
else
  echo "sha256sum ou shasum é obrigatório" >&2
  exit 1
fi
EXPECTED_HASH="$(printf '%s' "$EXPECTED_HASH" | tr 'A-F' 'a-f')"
ACTUAL_HASH="$(printf '%s' "$ACTUAL_HASH" | tr 'A-F' 'a-f')"
[ "$EXPECTED_HASH" = "$ACTUAL_HASH" ] || { echo "Checksum não confere para $ASSET" >&2; exit 1; }
echo "Checksum conferido"

tar -xf "$ASSET_FILE" -C "$TMP_DIR"
[ -f "${TMP_DIR}/${BIN_NAME}" ] || { echo "O pacote não contém ${BIN_NAME}" >&2; exit 1; }
if [ -e "${BIN_DIR}/${BIN_NAME}" ] && [ "$FORCE" != "yes" ]; then
  if (printf 'Substituir %s? [s/N] ' "${BIN_DIR}/${BIN_NAME}" >/dev/tty) 2>/dev/null; then
    read -r answer </dev/tty || answer=n
    case "$answer" in
      [sSyY]*) ;;
      *) echo "Instalação cancelada"; exit 1 ;;
    esac
  else
    echo "Sem terminal; a instalação existente foi mantida" >&2
    echo "Use --force para substituir" >&2
    exit 1
  fi
fi
mkdir -p "$BIN_DIR"
install -m 0755 "${TMP_DIR}/${BIN_NAME}" "${BIN_DIR}/${BIN_NAME}"

echo "Agente ${TAG} instalado em ${BIN_DIR}/${BIN_NAME}"
case ":${PATH}:" in
  *:"${BIN_DIR}":*) ;;
  *) echo "Coloque ${BIN_DIR} no PATH: export PATH=\"${BIN_DIR}:\$PATH\"" ;;
esac
