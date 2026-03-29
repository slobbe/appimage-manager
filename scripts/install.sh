#!/bin/sh
set -eu

is_tty_stdout() {
  [ -t 1 ]
}

is_tty_stderr() {
  [ -t 2 ]
}

supports_color() {
  [ -z "${NO_COLOR:-}" ]
}

style_stdout() {
  if is_tty_stdout && supports_color; then
    printf '\033[%sm' "$1"
  fi
}

section() {
  if is_tty_stdout && supports_color; then
    printf '%s%s%s\n' "$(style_stdout '1;36')" "$1" "$(style_stdout '0')"
    return
  fi

  printf '%s\n' "$1"
}

warn() {
  if is_tty_stderr && supports_color; then
    printf '%swarning:%s %s\n' "$(printf '\033[%sm' '1;33')" "$(printf '\033[%sm' '0')" "$1" >&2
    return
  fi

  printf 'warning: %s\n' "$1" >&2
}

fail() {
  if is_tty_stderr && supports_color; then
    printf '%serror:%s %s\n' "$(printf '\033[%sm' '1;31')" "$(printf '\033[%sm' '0')" "$1" >&2
  else
    printf 'error: %s\n' "$1" >&2
  fi
  exit 1
}

print_summary() {
  extras=""

  if [ "$man_installed" -eq 1 ]; then
    extras="man"
  fi
  if [ "$bash_completion_installed" -eq 1 ]; then
    if [ -n "$extras" ]; then
      extras="${extras}, "
    fi
    extras="${extras}bash"
  fi
  if [ "$zsh_completion_installed" -eq 1 ]; then
    if [ -n "$extras" ]; then
      extras="${extras}, "
    fi
    extras="${extras}zsh"
  fi
  if [ "$fish_completion_installed" -eq 1 ]; then
    if [ -n "$extras" ]; then
      extras="${extras}, "
    fi
    extras="${extras}fish"
  fi

  if is_tty_stdout && supports_color; then
    printf '%s%s%s\n' "$(style_stdout '1;32')" "${bin} installed" "$(style_stdout '0')"
    printf '  %sbinary:%s %s\n' "$(style_stdout '2')" "$(style_stdout '0')" "${inst}/${bin}"
    if [ -n "$extras" ]; then
      printf '  %sextras:%s %s\n' "$(style_stdout '2')" "$(style_stdout '0')" "$extras"
    fi
    printf '  %sverify:%s %s --version\n' "$(style_stdout '2')" "$(style_stdout '0')" "$bin"
    return
  fi

  printf '%s installed\n' "$bin"
  printf '  binary: %s\n' "${inst}/${bin}"
  if [ -n "$extras" ]; then
    printf '  extras: %s\n' "$extras"
  fi
  printf '  verify: %s --version\n' "$bin"
}

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
    fail "unsupported architecture: $arch"
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
      if (asset_name ~ ("^aim-[0-9].+-linux-" arch "\\.tar\\.gz$")) {
        found = 1
        print asset_url
        exit
      }
      if (asset_name ~ ("^aim-v.+-linux-" arch "\\.tar\\.gz$")) {
        legacy_versioned_url = asset_url
      }
      if (asset_name == ("aim-linux-" arch ".tar.gz")) {
        legacy_url = asset_url
      }
    }
    END {
      if (!found && legacy_versioned_url != "") {
        print legacy_versioned_url
        exit
      }
      if (!found && legacy_url != "") {
        print legacy_url
      }
    }
  '
})"

if [ -z "$asset_url" ]; then
  fail "no release archive found for ${goarch}"
fi

section "Installing ${bin}"
curl -fL "$asset_url" -o "$tgz"

tar -xzf "$tgz" -C "$tmpdir"

# The tarball contains a versioned filename: aim-<version>-linux-<arch>
found="$(find "$tmpdir" -type f -name "${bin}-*-linux-${goarch}" | head -n 1)"
if [ -z "$found" ]; then
  fail "expected ${bin}-*-linux-${goarch} inside release archive"
fi

chmod +x "$found"
mv -f "$found" "${inst}/${bin}"

man_installed=0
manpage="$(find "$tmpdir" -type f -path "*/share/man/man1/${bin}.1" | head -n 1)"
if [ -n "$manpage" ]; then
  chmod 0644 "$manpage"
  mv -f "$manpage" "${mandir}/${bin}.1"
  man_installed=1
else
  warn "man page not included in release archive"
fi

bash_completion_installed=0
bash_completion="$(find "$tmpdir" -type f -path "*/share/bash-completion/completions/${bin}" | head -n 1)"
if [ -n "$bash_completion" ]; then
  chmod 0644 "$bash_completion"
  mv -f "$bash_completion" "${bashcompdir}/${bin}"
  bash_completion_installed=1
else
  warn "bash completion not included in release archive"
fi

zsh_completion_installed=0
zsh_completion="$(find "$tmpdir" -type f -path "*/share/zsh/site-functions/_${bin}" | head -n 1)"
if [ -n "$zsh_completion" ]; then
  chmod 0644 "$zsh_completion"
  mv -f "$zsh_completion" "${zshcompdir}/_${bin}"
  zsh_completion_installed=1
else
  warn "zsh completion not included in release archive"
fi

fish_completion_installed=0
fish_completion="$(find "$tmpdir" -type f -path "*/share/fish/vendor_completions.d/${bin}.fish" | head -n 1)"
if [ -n "$fish_completion" ]; then
  chmod 0644 "$fish_completion"
  mv -f "$fish_completion" "${fishcompdir}/${bin}.fish"
  fish_completion_installed=1
else
  warn "fish completion not included in release archive"
fi

print_summary
