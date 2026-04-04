#!/usr/bin/env sh
set -e

REPO="viche-ai/owl"
INSTALL_DIR="${HOME}/.local/bin"
OWLD_SERVICE_URL="https://raw.githubusercontent.com/${REPO}/main/owld.service"
OWLD_PLIST_URL="https://raw.githubusercontent.com/${REPO}/main/ai.viche.owld.plist"

usage() {
    cat <<EOF
Usage: install.sh [OPTIONS]

Install owl and owld binaries.

OPTIONS:
    -d, --dir DIR       Install binaries to DIR (default: ~/.local/bin)
    -h, --help          Show this help message
    --no-service        Skip service setup


EXAMPLES:
    curl -fsSL https://owl.viche.ai/install.sh | sh
    curl -fsSL https://owl.viche.ai/install.sh | sh -s -- --dir /usr/local/bin

EOF
}

log() {
    echo "[owl] $1"
}

warn() {
    echo "[owl] WARNING: $1" >&2
}

err() {
    echo "[owl] ERROR: $1" >&2
    exit 1
}

detect_os() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$OS" in
        darwin) OS="darwin" ;;
        linux) OS="linux" ;;
        *)
            err "Unsupported OS: $OS"
            ;;
    esac
}

detect_arch() {
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7l) ARCH="arm" ;;
        *)
            err "Unsupported architecture: $ARCH"
            ;;
    esac
}

detect_shell() {
    SHELL_NAME="$(basename "${SHELL:-$(ps -p $$ -o comm= 2>/dev/null || echo sh)}")"
    case "$SHELL_NAME" in
        bash) RC_FILE="${HOME}/.bashrc" ;;
        zsh) RC_FILE="${HOME}/.zshrc" ;;
        fish) RC_FILE="${HOME}/.config/fish/config.fish" ;;
        *)
            RC_FILE="${HOME}/.profile"
            warn "Unknown shell ${SHELL_NAME}, defaulting to ~/.profile"
            ;;
    esac
}

check_dependencies() {
    for cmd in curl tar; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            err "Required command '$cmd' not found. Please install it first."
        fi
    done
}

fetch_latest_release() {
    log "Fetching latest release info from GitHub..."
    RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"
    case $? in
        0) ;;
        22) err "Failed to fetch release info. The repository or release may not exist." ;;
        *) err "Network error while fetching release info." ;;
    esac

    TAG_NAME="$(printf '%s' "$RELEASE_JSON" | grep '"tag_name"' | sed 's/.*"tag_name"[^"]*"v\?\([^"]*\)".*/\1/' | tr -d 'v')"
    if [ -z "$TAG_NAME" ]; then
        err "Could not parse release tag. Please ensure a release exists at https://github.com/${REPO}/releases/latest"
    fi
    log "Latest version: ${TAG_NAME}"
}

get_download_url() {
    ARCHIVE_NAME="owl_${TAG_NAME}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${TAG_NAME}/${ARCHIVE_NAME}"

    log "Download URL: ${DOWNLOAD_URL}"
}

download_and_install() {
    log "Downloading owl ${TAG_NAME} for ${OS}/${ARCH}..."
    ARCHIVE="/tmp/owl_install_${$}.tar.gz"

    if ! curl -fLo "$ARCHIVE" "$DOWNLOAD_URL"; then
        rm -f "$ARCHIVE"
        err "Download failed. Please check that the release assets are properly uploaded at https://github.com/${REPO}/releases/tag/v${TAG_NAME}"
    fi

    log "Installing to ${INSTALL_DIR}..."
    mkdir -p "$INSTALL_DIR"

    if ! tar -xzf "$ARCHIVE" -C "$INSTALL_DIR" 2>/dev/null; then
        rm -f "$ARCHIVE"
        err "Extraction failed. The archive may be corrupted."
    fi

    chmod +x "${INSTALL_DIR}/owl" "${INSTALL_DIR}/owld" 2>/dev/null || true

    rm -f "$ARCHIVE"
    log "Installed owl and owld to ${INSTALL_DIR}"
}

configure_path() {
    if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
        return
    fi

    log "Adding ${INSTALL_DIR} to PATH in ${RC_FILE}..."

    case "$SHELL_NAME" in
        fish)
            printf '\n# owl\nset -gx PATH "%s" $PATH\n' "$INSTALL_DIR" >> "$RC_FILE"
            ;;
        *)
            printf '\n# owl\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >> "$RC_FILE"
            ;;
    esac

    log "Added ${INSTALL_DIR} to PATH. Please restart your shell or run:"
    log "  source ${RC_FILE}"
}

setup_systemd_service() {
    log "Setting up systemd user service..."

    SYSTEMD_DIR="${HOME}/.config/systemd/user"
    mkdir -p "$SYSTEMD_DIR"

    if ! curl -fsSL "$OWLD_SERVICE_URL" -o "${SYSTEMD_DIR}/owld.service"; then
        warn "Failed to download owld.service template."
        return 1
    fi

    sed -i "s|/home/USER|${HOME}|g" "${SYSTEMD_DIR}/owld.service"
    sed -i "s|INSTALLDIR|${INSTALL_DIR}|g" "${SYSTEMD_DIR}/owld.service"

    systemctl --user daemon-reload 2>/dev/null || {
        warn "Failed to reload systemd user daemon. Is systemd running?"
        warn "You may need to run: systemctl --user daemon-reload"
        return 0
    }

    systemctl --user enable --now owld.service 2>/dev/null || {
        warn "Failed to enable owld.service."
        warn "You can start it manually with: systemctl --user start owld.service"
        return 0
    }

    log "owld.service enabled and started."
}

setup_launchd_service() {
    log "Setting up launchd service..."

    PLIST_DIR="${HOME}/Library/LaunchAgents"
    mkdir -p "$PLIST_DIR"

    PLIST_PATH="${PLIST_DIR}/ai.viche.owld.plist"

    if ! curl -fsSL "$OWLD_PLIST_URL" -o "$PLIST_PATH"; then
        warn "Failed to download launchd plist template."
        return 1
    fi

    sed -i '' "s|INSTALLDIR|${INSTALL_DIR}|g" "$PLIST_PATH"

    launchctl load -w "$PLIST_PATH" 2>/dev/null || {
        warn "Failed to load owld launchd service."
        warn "You can start it manually with: launchctl load -w ${PLIST_PATH}"
        return 0
    }

    log "owld launchd service enabled and started."
}

setup_service() {
    case "$OS" in
        linux)
            if command -v systemctl >/dev/null 2>&1; then
                setup_systemd_service
            else
                warn "systemd not found. Skipping service setup."
            fi
            ;;
        darwin)
            setup_launchd_service
            ;;
    esac
}

main() {
    NO_SERVICE=0
    UPGRADE=0
    FORCE=0

    while [ $# -gt 0 ]; do
        case "$1" in
            -h|--help) usage; exit 0 ;;
            -d|--dir) INSTALL_DIR="$2"; shift 2 ;;
            --no-service) NO_SERVICE=1; shift ;;
            *) shift ;;
        esac
    done

    check_dependencies
    detect_os
    detect_arch
    detect_shell

    if [ -f "${INSTALL_DIR}/owl" ] || [ -f "${INSTALL_DIR}/owld" ]; then
        log "Upgrading owl and owld in ${INSTALL_DIR}..."
        UPGRADE=1
    fi

    fetch_latest_release
    get_download_url
    download_and_install

    configure_path

    if [ "$NO_SERVICE" = "0" ]; then
        setup_service
    fi

    log ""
    if [ "$UPGRADE" = "1" ]; then
        log "Upgrade complete!"
    else
        log "Installation complete!"
    fi
    log "Run 'owl' to start the TUI."
    log "If your shell does not pick up the new PATH, restart your shell or run:"
    log "  source ${RC_FILE}"
}

main "$@"