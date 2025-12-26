#!/bin/sh
set -e
repo="slobbe/appimage-manager"
bin="aim"
inst="$HOME/.local/bin"

curl -sL "https://github.com/$repo/releases/latest/download/${bin}-linux-amd64" \
-o "$inst/$bin" #| tar -xz -C "$inst" "$bin"
chmod +x "$inst/$bin"
echo "installed to $inst/$bin"
