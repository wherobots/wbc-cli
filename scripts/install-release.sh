#!/usr/bin/env bash
set -euo pipefail

REPO="${WHEROBOTS_CLI_REPO:-wherobots/wbc-cli}"
TAG="${WHEROBOTS_CLI_TAG:-latest}"
BINARY_NAME="${WHEROBOTS_CLI_BINARY:-wherobots}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
SKIP_CHECKSUM=0

usage() {
  cat <<'EOF'
Install wherobots CLI from a GitHub release.

Requirements:
  - curl

Usage:
  ./scripts/install-release.sh [options]

Options:
  --repo <owner/name>      GitHub repository (default: wherobots/wbc-cli)
  --tag <tag>              Release tag (default: latest)
  --install-dir <path>     Install directory (default: ~/.local/bin)
  --binary-name <name>     Binary name/asset prefix (default: wherobots)
  --skip-checksum          Skip checksum verification
  -h, --help               Show help

Environment overrides:
  WHEROBOTS_CLI_REPO
  WHEROBOTS_CLI_TAG
  WHEROBOTS_CLI_BINARY
  INSTALL_DIR
EOF
}

while (($# > 0)); do
  case "$1" in
    --repo)
      REPO="${2:?missing value for --repo}"
      shift 2
      ;;
    --tag)
      TAG="${2:?missing value for --tag}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:?missing value for --install-dir}"
      shift 2
      ;;
    --binary-name)
      BINARY_NAME="${2:?missing value for --binary-name}"
      shift 2
      ;;
    --skip-checksum)
      SKIP_CHECKSUM=1
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

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required." >&2
  exit 1
fi

if ! command -v install >/dev/null 2>&1; then
  echo "'install' command is required." >&2
  exit 1
fi

case "$(uname -s)" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

ASSET="${BINARY_NAME}_${OS}_${ARCH}"
TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

# "latest" is not a real tag — resolve it to the actual latest release tag.
if [[ "$TAG" == "latest" ]]; then
  API_RESPONSE="$(curl -fsSL -H "User-Agent: wherobots-cli" \
    "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null)" || true
  # Parse tag_name from JSON without requiring jq — grep for the field and
  # strip surrounding quotes/whitespace with sed.
  TAG="$(printf '%s' "$API_RESPONSE" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')" || true
  if [[ -z "$TAG" ]]; then
    echo "No release found in $REPO" >&2
    exit 1
  fi
fi

DOWNLOAD_BASE="https://github.com/${REPO}/releases/download/${TAG}"

echo "Downloading ${ASSET} from ${REPO}@${TAG}..."
curl -fsSL -o "$TMP_DIR/$ASSET" "${DOWNLOAD_BASE}/${ASSET}"

if [[ "$SKIP_CHECKSUM" -eq 0 ]]; then
  echo "Verifying checksum..."
  curl -fsSL -o "$TMP_DIR/checksums.txt" "${DOWNLOAD_BASE}/checksums.txt"
  EXPECTED="$(awk -v file="$ASSET" '$2 == file { print $1 }' "$TMP_DIR/checksums.txt" | head -n1)"
  if [[ -z "$EXPECTED" ]]; then
    echo "Could not find checksum entry for $ASSET" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    ACTUAL="$(shasum -a 256 "$TMP_DIR/$ASSET" | awk '{print $1}')"
  else
    echo "No SHA-256 tool available (need sha256sum or shasum)." >&2
    exit 1
  fi
  if [[ "$EXPECTED" != "$ACTUAL" ]]; then
    echo "Checksum mismatch for $ASSET" >&2
    exit 1
  fi
fi

TARGET="${INSTALL_DIR}/${BINARY_NAME}"

# Ensure the install directory exists; create without sudo when possible.
if [[ ! -d "$INSTALL_DIR" ]]; then
  if ! mkdir -m 0755 -p "$INSTALL_DIR" 2>/dev/null; then
    if command -v sudo >/dev/null 2>&1; then
      sudo mkdir -p "$INSTALL_DIR"
    else
      echo "Cannot create $INSTALL_DIR and sudo is unavailable." >&2
      exit 1
    fi
  fi
fi

if [[ -w "$INSTALL_DIR" ]]; then
  install -m 0755 "$TMP_DIR/$ASSET" "$TARGET"
else
  if command -v sudo >/dev/null 2>&1; then
    sudo install -m 0755 "$TMP_DIR/$ASSET" "$TARGET"
  else
    echo "No write access to $INSTALL_DIR and sudo is unavailable." >&2
    exit 1
  fi
fi

echo "Installed: $TARGET"
echo "If needed, add to PATH: export PATH=\"${INSTALL_DIR}:\$PATH\""
