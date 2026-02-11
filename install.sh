#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="$HOME/.local/bin"
BINARY_NAME="devctl"

# 1. Check Go is installed
if ! command -v go &>/dev/null; then
    echo "Error: Go is not installed. Install it from https://go.dev/dl/" >&2
    exit 1
fi

# 2. Build the binary
echo "Building $BINARY_NAME..."
go build -o "$BINARY_NAME" .

# 3. Install to ~/.local/bin
mkdir -p "$INSTALL_DIR"
mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
echo "Installed $BINARY_NAME to $INSTALL_DIR/$BINARY_NAME"

# 4. Ensure ~/.local/bin is on PATH
if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    echo "$INSTALL_DIR is already on PATH"
else
    SHELL_NAME="$(basename "$SHELL")"
    case "$SHELL_NAME" in
        zsh)
            PROFILE="$HOME/.zshrc"
            LINE='export PATH="$HOME/.local/bin:$PATH"'
            ;;
        bash)
            PROFILE="$HOME/.bashrc"
            LINE='export PATH="$HOME/.local/bin:$PATH"'
            ;;
        fish)
            PROFILE="$HOME/.config/fish/config.fish"
            LINE='fish_add_path $HOME/.local/bin'
            ;;
        *)
            echo "Warning: Unknown shell '$SHELL_NAME'. Add $INSTALL_DIR to your PATH manually." >&2
            PROFILE=""
            LINE=""
            ;;
    esac

    if [[ -n "$PROFILE" ]]; then
        if [[ -f "$PROFILE" ]] && grep -qF "$LINE" "$PROFILE"; then
            echo "PATH entry already exists in $PROFILE"
        else
            echo "" >> "$PROFILE"
            echo "$LINE" >> "$PROFILE"
            echo "Added $INSTALL_DIR to PATH in $PROFILE"
        fi
    fi
fi

# 5. Print success message
echo ""
echo "Successfully installed $BINARY_NAME!"
echo ""
echo "Next steps:"
echo "  source your shell profile or open a new terminal, then run:"
echo "    $BINARY_NAME --help"
echo ""
echo "To uninstall:"
echo "  rm $INSTALL_DIR/$BINARY_NAME"
