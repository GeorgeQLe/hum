#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

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

# 6. Check if the binary is reachable in the current shell
if ! command -v "$BINARY_NAME" &>/dev/null; then
    SHELL_NAME="$(basename "$SHELL")"
    case "$SHELL_NAME" in
        zsh)  SOURCE_CMD="source ~/.zshrc" ;;
        bash) SOURCE_CMD="source ~/.bashrc" ;;
        fish) SOURCE_CMD="source ~/.config/fish/config.fish" ;;
        *)    SOURCE_CMD="source your shell profile" ;;
    esac
    echo ""
    echo "To start using $BINARY_NAME, run:"
    echo "  $SOURCE_CMD"
    echo ""
    echo "Or open a new terminal, then run:"
    echo "  $BINARY_NAME --help"
else
    echo ""
    echo "Run '$BINARY_NAME --help' to get started."
fi
echo ""
echo "To uninstall:"
echo "  rm $INSTALL_DIR/$BINARY_NAME"
