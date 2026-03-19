#!/bin/sh
set -eu

repo="slobbe/appimage-manager"
bin="aim"
inst="${HOME}/.local/bin"
data_home="${XDG_DATA_HOME:-${HOME}/.local/share}"
mandir="${data_home}/man/man1"
bashcompdir="${data_home}/bash-completion/completions"
zshcompdir="${data_home}/zsh/site-functions"
fishcompdir="${data_home}/fish/vendor_completions.d"
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

mkdir -p "$inst" "$mandir" "$bashcompdir" "$zshcompdir" "$fishcompdir"

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
found="$(find "$tmpdir" -type f -name "${bin}-*-linux-${goarch}" | head -n 1)"
if [ -z "$found" ]; then
  echo "error: expected ${bin}-*-linux-${goarch} inside tarball" >&2
  exit 1
fi

chmod +x "$found"
mv -f "$found" "${inst}/${bin}"

manpage="$(find "$tmpdir" -type f -path "*/share/man/man1/${bin}.1" | head -n 1)"
if [ -n "$manpage" ]; then
  chmod 0644 "$manpage"
  mv -f "$manpage" "${mandir}/${bin}.1"
  echo "installed man page to ${mandir}/${bin}.1"
  echo "run: man ${bin}"
else
  echo "warning: man page not found in release archive" >&2
fi

installed_completion=0

bash_completion="$(find "$tmpdir" -type f -path "*/share/bash-completion/completions/${bin}" | head -n 1)"
if [ -n "$bash_completion" ]; then
  chmod 0644 "$bash_completion"
  mv -f "$bash_completion" "${bashcompdir}/${bin}"
  echo "installed Bash completion to ${bashcompdir}/${bin}"
  installed_completion=1
else
  echo "warning: Bash completion not found in release archive" >&2
fi

zsh_completion="$(find "$tmpdir" -type f -path "*/share/zsh/site-functions/_${bin}" | head -n 1)"
if [ -n "$zsh_completion" ]; then
  chmod 0644 "$zsh_completion"
  mv -f "$zsh_completion" "${zshcompdir}/_${bin}"
  echo "installed Zsh completion to ${zshcompdir}/_${bin}"
  installed_completion=1
else
  echo "warning: Zsh completion not found in release archive" >&2
fi

fish_completion="$(find "$tmpdir" -type f -path "*/share/fish/vendor_completions.d/${bin}.fish" | head -n 1)"
if [ -n "$fish_completion" ]; then
  chmod 0644 "$fish_completion"
  mv -f "$fish_completion" "${fishcompdir}/${bin}.fish"
  echo "installed Fish completion to ${fishcompdir}/${bin}.fish"
  installed_completion=1
else
  echo "warning: Fish completion not found in release archive" >&2
fi

echo "installed to ${inst}/${bin}"
echo "run: ${bin} --version"

if [ "$installed_completion" -eq 1 ]; then
  echo "completion notes:"
  echo "  Bash/Zsh may require a new shell session before completions are available."
  echo "  Zsh may require ${zshcompdir} to be present in fpath."
  echo "  Fish usually auto-loads completions from ${fishcompdir}."
  echo "  no shell startup files were modified automatically."
fi
