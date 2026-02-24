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
api_url="https://api.github.com/repos/${repo}/releases/latest"

asset_url="$({
  curl -fsSL "$api_url" | tr '{},' '\n' | awk -F'"' -v arch="$goarch" '
    $2 == "name" {
      asset_name = $4
      next
    }
    $2 == "browser_download_url" {
      asset_url = $4
      if (asset_name ~ ("^aim-.+-linux-" arch "\\.tar\\.gz$")) {
        found = 1
        print asset_url
        exit
      }
      if (asset_name == ("aim-linux-" arch ".tar.gz")) {
        legacy_url = asset_url
      }
    }
    END {
      if (!found && legacy_url != "") {
        print legacy_url
      }
    }
  '
})"

if [ -z "$asset_url" ]; then
  echo "error: could not find release archive for architecture ${goarch}" >&2
  exit 1
fi

curl -fL "$asset_url" -o "$tgz"

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
