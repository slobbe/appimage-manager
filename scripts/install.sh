#!/bin/sh
set -eu

repo="slobbe/appimage-manager"
bin="aim"
inst="${HOME}/.local/bin"
arch="$(uname -m)"

case "$arch" in
  x86_64 | amd64) goarch="amd64" ;;
  aarch64 | arm64) goarch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    echo "supported: x86_64 (amd64), aarch64 (arm64)" >&2
    exit 1
    ;;
esac

mkdir -p "$inst"

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT INT TERM

tgz="${tmpdir}/aim.tgz"
url="https://github.com/${repo}/releases/latest/download/${bin}-linux-${goarch}.tar.gz"

curl -fL "$url" -o "$tgz"

tar -xzf "$tgz" -C "$tmpdir"

# The tarball contains a versioned filename: aim-<version>-linux-<arch>
found="$(find "$tmpdir" -maxdepth 1 -type f -name "${bin}-*-linux-${goarch}" | head -n 1)"
if [ -z "$found" ]; then
  echo "error: expected ${bin}-*-linux-${goarch} inside tarball" >&2
  exit 1
fi

chmod +x "$found"
mv -f "$found" "${inst}/${bin}"

echo "installed to ${inst}/${bin}"
echo "run: ${bin} --version"
