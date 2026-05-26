#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
ORANGE='\033[38;2;255;140;0m'
NC='\033[0m' # No Color

function print_message() {
  local level=$1
  local message=$2
  local color=""

  case $level in
  info) color="${GREEN}" ;;
  warning) color="${YELLOW}" ;;
  error) color="${RED}" ;;
  esac

  printf "%b\n" "${color}${message}${NC}"
}

function log_error() {
  print_message error "$1" >&2
  exit "$2"
}

function detect_client_info() {
  # Detect WSL or Cygwin as windows
  if [[ "${CLIENT_PLATFORM}" =~ ^(mingw|cygwin|msys)_nt* ]] || grep -qi microsoft /proc/version 2>/dev/null; then
    CLIENT_PLATFORM="windows"
  fi

  case "${CLIENT_PLATFORM}" in
  darwin | linux | windows | freebsd) ;;
  *) log_error "Unknown or unsupported platform: ${CLIENT_PLATFORM}. Supported platforms are Linux, Darwin, FreeBSD, and Windows." 2 ;;
  esac

  case "${CLIENT_ARCH}" in
  x86_64* | i?86_64* | amd64*) CLIENT_ARCH="amd64" ;;
  aarch64* | arm64*) CLIENT_ARCH="arm64" ;;
  armv7l*) CLIENT_ARCH="arm-7" ;;
  armv6l*) CLIENT_ARCH="arm-6" ;;
  armv5l*) CLIENT_ARCH="arm-5" ;;
  *) log_error "Unknown or unsupported architecture: ${CLIENT_ARCH}. Supported architectures are x86_64, arm64, armv5, armv6, armv7." 3 ;;
  esac
}

function verify_checksum() {
  local file=$1
  local name=$2
  local checksums="${TMPDIR}/checksums.txt"

  # checksums.txt is published with every release; fetch it best-effort.
  if ! curl -fsSL --retry 5 ${INSECURE_ARG} "${DOWNLOAD_URL_PREFIX}/checksums.txt" -o "${checksums}" 2>/dev/null; then
    print_message warning "WARNING: could not download checksums.txt; skipping SHA256 verification."
    return 0
  fi

  local expected
  expected=$(grep " ${name}\$" "${checksums}" 2>/dev/null | awk '{print $1}' | head -n1)
  if [[ -z "${expected}" ]]; then
    print_message warning "WARNING: no checksum entry for ${name}; skipping SHA256 verification."
    return 0
  fi

  local actual=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "${file}" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "${file}" | awk '{print $1}')
  else
    print_message warning "WARNING: no sha256sum/shasum tool found; skipping SHA256 verification."
    return 0
  fi

  if [[ "${actual}" != "${expected}" ]]; then
    log_error "Checksum mismatch for ${name}: expected ${expected}, got ${actual}" 9
  fi
  print_message info "Checksum verified for ${ORANGE}${name}${NC}"
}

function download_and_install() {
  DOWNLOAD_URL_PREFIX="${RELEASE_URL}/v${VERSION}"
  CLIENT_BINARY="go-jira-${VERSION}-${CLIENT_PLATFORM}-${CLIENT_ARCH}"
  # Windows release assets carry a .exe suffix.
  if [[ "${CLIENT_PLATFORM}" == "windows" ]]; then
    CLIENT_BINARY="${CLIENT_BINARY}.exe"
    INSTALL_NAME="go-jira.exe"
  else
    INSTALL_NAME="go-jira"
  fi

  print_message info "Downloading ${CLIENT_BINARY} from ${DOWNLOAD_URL_PREFIX}"
  mkdir -p "$INSTALL_DIR" || log_error "Failed to create directory: $INSTALL_DIR" 5

  # Use temp dir for download
  TARGET="${TMPDIR}/${CLIENT_BINARY}"

  curl -# -fSL --retry 5 --keepalive-time 2 ${INSECURE_ARG} "${DOWNLOAD_URL_PREFIX}/${CLIENT_BINARY}" -o "${TARGET}" \
    || log_error "Failed to download ${CLIENT_BINARY}" 6

  verify_checksum "${TARGET}" "${CLIENT_BINARY}"

  chmod +x "${TARGET}" || log_error "Failed to set executable permission on: ${TARGET}" 7
  # Move the binary to install dir and rename to go-jira
  mv "${TARGET}" "${INSTALL_DIR}/${INSTALL_NAME}" || log_error "Failed to move ${TARGET} to ${INSTALL_DIR}/${INSTALL_NAME}" 8
  # show the version
  print_message info "Installed ${ORANGE}${CLIENT_BINARY}${NC} to ${GREEN}${INSTALL_DIR}${NC}"
  print_message info "Run ${ORANGE}go-jira --version${NC} to show the version"
  print_message info ""
  print_message info "==============================="
  "${INSTALL_DIR}/${INSTALL_NAME}" --version
  print_message info "==============================="
  print_message info ""
  print_message info "✅ Installation completed successfully!"
}

function add_to_path() {
  local config_file=$1
  local command=$2

  if grep -Fxq "$command" "$config_file"; then
    print_message info "Configuration already exists in $config_file, skipping"
    return 0
  fi

  if [[ -w $config_file ]]; then
    printf "\n# go-jira\n" >>"$config_file"
    echo "$command" >>"$config_file"
    print_message info "Successfully added ${ORANGE}go-jira ${GREEN}to \$PATH in $config_file"
  else
    print_message warning "Manually add the directory to $config_file (or similar):"
    print_message info "  $command"
  fi
}

# Fetch latest release version from GitHub if VERSION is not set
function get_latest_version() {
  local latest http_code
  local api="https://api.github.com/repos/appleboy/go-jira/releases/latest"
  local body="${TMPDIR}/latest.json"

  # Authenticate when a token is available. Unauthenticated requests share a
  # 60 req/hour-per-IP budget that is easily exhausted behind a shared NAT;
  # a token raises it to 5000 req/hour. (`${arr[@]+...}` keeps the empty-array
  # expansion safe under `set -u` on bash 3.2, e.g. stock macOS.)
  local -a auth=()
  local token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
  if [[ -n "$token" ]]; then
    auth=(-H "Authorization: Bearer ${token}")
  fi

  # Capture the body and status separately so we can explain HTTP errors
  # instead of letting `-f` swallow them.
  http_code=$(curl $INSECURE_ARG -sS --retry 5 ${auth[@]+"${auth[@]}"} -o "${body}" -w '%{http_code}' "${api}") \
    || log_error "Failed to reach the GitHub API. Check your network, or pin a version with VERSION=x.y.z." 6

  if [[ "$http_code" == "403" || "$http_code" == "429" ]] && grep -q "rate limit" "${body}" 2>/dev/null; then
    log_error "GitHub API rate limit exceeded. Set GITHUB_TOKEN to raise the limit, or pin a version, e.g. VERSION=0.10.0." 6
  fi
  if [[ "$http_code" != "200" ]]; then
    log_error "GitHub API returned HTTP ${http_code}. Pin a version with VERSION=x.y.z to skip this lookup." 6
  fi

  if command -v jq >/dev/null 2>&1; then
    latest=$(jq -r '.tag_name // empty' "${body}")
  else
    latest=$(grep '"tag_name":' "${body}" | sed -E 's/.*"tag_name": ?"v?([^"]+)".*/\1/')
  fi
  # Remove leading 'v' if present
  latest="${latest#v}"
  if [[ -z "$latest" ]]; then
    log_error "Could not parse the latest version from the GitHub API response. Pin a version with VERSION=x.y.z." 6
  fi
  echo "$latest"
}

# Check for required commands
for cmd in curl mktemp; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    log_error "Error: $cmd is not installed. Please install $cmd to proceed." 1
  fi
done

# Create temp directory for downloads.
TMPDIR="$(mktemp -d)"
function cleanup() {
  if [ -n "${TMPDIR:-}" ] && [ -d "$TMPDIR" ]; then
    rm -rf "$TMPDIR"
  fi
}
trap cleanup EXIT INT TERM

# If INSECURE is set to any value, enable curl --insecure
INSECURE_ARG=""
if [[ -n "${INSECURE:-}" ]]; then
  INSECURE_ARG="--insecure"
  print_message warning "WARNING: INSECURE mode is enabled. Proceeding with insecure download."
  print_message warning "WARNING: You are bypassing SSL certificate verification. This is insecure and may expose you to man-in-the-middle attacks."
fi

if [[ -z "${VERSION:-}" ]]; then
  VERSION=$(get_latest_version)
fi

# Check if VERSION is a valid semantic version
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  log_error "Invalid version format: $VERSION. Expected format: x.y.z" 1
fi

RELEASE_URL="${RELEASE_URL:-https://github.com/appleboy/go-jira/releases/download}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.go-jira/bin}"
CLIENT_PLATFORM="${CLIENT_PLATFORM:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
CLIENT_ARCH="${CLIENT_ARCH:-$(uname -m)}"

detect_client_info
download_and_install

XDG_CONFIG_HOME=${XDG_CONFIG_HOME:-$HOME/.config}

current_shell=$(basename "$SHELL")
case $current_shell in
fish)
  config_files="$HOME/.config/fish/config.fish"
  ;;
zsh)
  config_files="$HOME/.zshrc $HOME/.zshenv $XDG_CONFIG_HOME/zsh/.zshrc $XDG_CONFIG_HOME/zsh/.zshenv"
  ;;
bash)
  config_files="$HOME/.bashrc $HOME/.bash_profile $HOME/.profile $XDG_CONFIG_HOME/bash/.bashrc $XDG_CONFIG_HOME/bash/.bash_profile"
  ;;
ash)
  config_files="$HOME/.ashrc $HOME/.profile /etc/profile"
  ;;
sh)
  config_files="$HOME/.ashrc $HOME/.profile /etc/profile"
  ;;
*)
  # Default case if none of the above matches
  config_files="$HOME/.bashrc $HOME/.bash_profile $XDG_CONFIG_HOME/bash/.bashrc $XDG_CONFIG_HOME/bash/.bash_profile"
  ;;
esac

config_file=""
for file in $config_files; do
  if [[ -f $file ]]; then
    config_file=$file
    break
  fi
done

if [[ -z $config_file ]]; then
  log_error "No config file found for $current_shell. Checked files: $config_files" 1
fi

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
  case $current_shell in
  fish)
    add_to_path "$config_file" "fish_add_path $INSTALL_DIR"
    ;;
  zsh | bash | ash | sh)
    add_to_path "$config_file" "export PATH=$INSTALL_DIR:\$PATH"
    ;;
  *)
    print_message warning "Manually add the directory to $config_file (or similar):"
    print_message info "  export PATH=$INSTALL_DIR:\$PATH"
    ;;
  esac
fi

print_message info "To use the command, please restart your terminal or run:"
print_message info "  source $config_file"

if [ -n "${GITHUB_ACTIONS-}" ] && [ "${GITHUB_ACTIONS}" == "true" ]; then
  echo "$INSTALL_DIR" >>"$GITHUB_PATH"
  print_message info "Added $INSTALL_DIR to \$GITHUB_PATH"
fi
