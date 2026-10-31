#!/bin/sh
set -eu

VERSION=""
OUT=""
TARGET=""

usage() {
  echo "usage: package-agent.sh --version <X.Y.Z> --out <dir> [--target os/arch]" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || { echo "--version requires a value" >&2; exit 1; }
      VERSION="$2"
      shift 2
      ;;
    --out)
      [ "$#" -ge 2 ] || { echo "--out requires a value" >&2; exit 1; }
      OUT="$2"
      shift 2
      ;;
    --target)
      [ "$#" -ge 2 ] || { echo "--target requires a value" >&2; exit 1; }
      TARGET="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ -z "$VERSION" ] || [ -z "$OUT" ]; then
  usage
  exit 1
fi

printf '%s\n' "$VERSION" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || {
  echo "Invalid version: $VERSION" >&2
  exit 1
}

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
mkdir -p "$OUT"
OUT="$(CDPATH= cd -- "$OUT" && pwd)"

build_one() {
  os="$1"
  arch="$2"
  work="$(mktemp -d)"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X ai-usage-tracker/internal/agent.Version=${VERSION}" -o "$work/ai-usage" ./cmd/agent
  tar -C "$work" -czf "$OUT/ai-usage_${VERSION}_${os}_${arch}.tar.gz" ai-usage
  rm -rf "$work"
}

cd "$ROOT"
if [ -n "$TARGET" ]; then
  os="${TARGET%%/*}"
  arch="${TARGET#*/}"
  if [ -z "$os" ] || [ -z "$arch" ] || [ "$os" = "$TARGET" ]; then
    echo "Invalid target: $TARGET" >&2
    exit 1
  fi
  build_one "$os" "$arch"
else
  build_one linux amd64
  build_one linux arm64
  build_one darwin amd64
  build_one darwin arm64
fi

(
  cd "$OUT"
  sha256sum ai-usage_"${VERSION}"_*.tar.gz > checksums.txt
)
cp "$ROOT/scripts/agent-installer.sh" "$OUT/agent-installer.sh"
cat > "$OUT/notes.md" <<EOF
## Instalação

O script instala só o comando \`ai-usage\` em \`~/.local/bin\`. O painel não entra neste pacote. Ele não grava um comando chamado \`agent\`.

\`\`\`sh
curl -fsSL https://github.com/lucaswilliameufrasio/ai-usage-tracker/releases/download/v${VERSION}/agent-installer.sh | sh -s -- --tag v${VERSION}
\`\`\`

O script confere o SHA256 do pacote antes de instalar. Isso protege contra arquivo corrompido no download, não contra uma release alterada.
EOF
