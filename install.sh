#!/usr/bin/env bash
#
# Instalador de la CLI de Oráculo (DelfosTI).
#
#   curl -sSL https://raw.githubusercontent.com/delfosti-infra/oraculo-cli/main/install.sh | bash
#
# Hace `go install`, deja el binario disponible como `oraculo` y se asegura de
# que el directorio de binarios de Go esté en tu PATH. No necesita sudo.
set -euo pipefail

MODULE="github.com/delfosti-infra/oraculo-cli"
VERSION="${ORACULO_VERSION:-latest}"

if ! command -v go >/dev/null 2>&1; then
  echo "✗ Necesitas Go instalado (https://go.dev/dl/) para instalar la CLI de Oráculo." >&2
  echo "  ¿No usas Go? Pídele a tu contacto de DelfosTI el binario precompilado." >&2
  exit 1
fi

echo "› Instalando la CLI de Oráculo (${MODULE}@${VERSION})…"
go install "${MODULE}@${VERSION}"

GOBIN="$(go env GOBIN)"
[ -n "$GOBIN" ] || GOBIN="$(go env GOPATH)/bin"

SRC="$GOBIN/oraculo-cli"
if [ ! -x "$SRC" ]; then
  echo "✗ No encontré el binario en $SRC tras 'go install'." >&2
  exit 1
fi

ln -sf "$SRC" "$GOBIN/oraculo"
echo "✓ Binario disponible como 'oraculo' en $GOBIN"

# Asegurar que GOBIN esté en el PATH.
case ":${PATH:-}:" in
  *":$GOBIN:"*)
    echo "✓ $GOBIN ya está en tu PATH"
    ;;
  *)
    case "${SHELL:-}" in
      */zsh)  RC="$HOME/.zshrc" ;;
      */bash) RC="$HOME/.bashrc" ;;
      *)      RC="$HOME/.profile" ;;
    esac
    if ! grep -qsF "$GOBIN" "$RC"; then
      printf '\n# Añadido por el instalador de Oráculo\nexport PATH="$PATH:%s"\n' "$GOBIN" >> "$RC"
      echo "✓ Agregué $GOBIN a tu PATH en $RC"
    fi
    echo "→ Reinicia la terminal (o corre: source $RC) para que 'oraculo' quede disponible."
    ;;
esac

echo ""
echo "Listo. Verifica con:  oraculo --version"
