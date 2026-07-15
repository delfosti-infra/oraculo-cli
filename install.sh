#!/usr/bin/env bash
set -euo pipefail

REPO="delfosti-infra/oraculo-cli"
INSTALL_DIR="${ORACULO_INSTALL_DIR:-$HOME/.local/bin}"

need() { command -v "$1" >/dev/null 2>&1; }
if ! need curl; then
  echo "✗ Necesito 'curl' para descargar el binario." >&2
  exit 1
fi

os="$(uname -s)"
case "$os" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "✗ SO no soportado por este instalador: $os" >&2
    echo "  En Windows usa install.ps1. Para otros, pide el binario a tu contacto de DelfosTI." >&2
    exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64)  arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "✗ Arquitectura no soportada: $arch" >&2
    exit 1 ;;
esac

asset="oraculo_${os}_${arch}"

version="${ORACULO_VERSION:-}"
if [ -z "$version" ]; then
  echo "› Buscando la última versión…"
  version="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
    | head -n1 \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/')"
  if [ -z "$version" ]; then
    echo "✗ No pude resolver la última versión. Fija una con ORACULO_VERSION=vX.Y.Z" >&2
    exit 1
  fi
fi

url="https://github.com/${REPO}/releases/download/${version}/${asset}"

echo "› Instalando oraculo ${version} (${os}/${arch})…"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if ! curl -fSL "$url" -o "$tmp/oraculo"; then
  echo "✗ No pude descargar $url" >&2
  echo "  ¿Existe un release con binarios para ese tag? Revisa https://github.com/${REPO}/releases" >&2
  exit 1
fi

if curl -fsSL "https://github.com/${REPO}/releases/download/${version}/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
  sumtool=""
  if need sha256sum; then sumtool="sha256sum"; elif need shasum; then sumtool="shasum -a 256"; fi
  if [ -n "$sumtool" ]; then
    expected="$(grep " ${asset}$" "$tmp/checksums.txt" | awk '{print $1}' | head -n1)"
    if [ -n "$expected" ]; then
      actual="$($sumtool "$tmp/oraculo" | awk '{print $1}')"
      if [ "$expected" != "$actual" ]; then
        echo "✗ Checksum no coincide para $asset. Abortando por seguridad." >&2
        exit 1
      fi
      echo "✓ Checksum verificado"
    fi
  fi
fi

mkdir -p "$INSTALL_DIR"
chmod +x "$tmp/oraculo"
mv -f "$tmp/oraculo" "$INSTALL_DIR/oraculo"
echo "✓ Binario instalado en $INSTALL_DIR/oraculo"

case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*)
    echo "✓ $INSTALL_DIR ya está en tu PATH"
    ;;
  *)
    case "${SHELL:-}" in
      */zsh)  rc="$HOME/.zshrc" ;;
      */bash) rc="$HOME/.bashrc" ;;
      *)      rc="$HOME/.profile" ;;
    esac
    if ! grep -qsF "$INSTALL_DIR" "$rc" 2>/dev/null; then
      printf '\nexport PATH="$PATH:%s"\n' "$INSTALL_DIR" >> "$rc"
      echo "✓ Agregué $INSTALL_DIR a tu PATH en $rc"
    fi
    echo "→ Reinicia la terminal (o corre: source $rc) para que 'oraculo' quede disponible."
    ;;
esac

echo ""
echo "Listo. Verifica con:  oraculo --version"
