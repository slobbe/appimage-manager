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

path_contains_dir() {
  case ":${PATH:-}:" in
    *:"$1":*) return 0 ;;
    *) return 1 ;;
  esac
}

shell_rc_file() {
  shell_name="${SHELL##*/}"

  case "$shell_name" in
    zsh) printf '%s/.zshrc\n' "$HOME" ;;
    bash) printf '%s/.bashrc\n' "$HOME" ;;
    fish) printf '%s/.config/fish/config.fish\n' "$HOME" ;;
    *) printf '%s/.profile\n' "$HOME" ;;
  esac
}

shell_path_export() {
  shell_name="${SHELL##*/}"

  case "$shell_name" in
    fish) printf "fish_add_path \$HOME/.local/bin\n" ;;
    *) printf "export PATH=\"\$HOME/.local/bin:\$PATH\"\n" ;;
  esac
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

json_release_tag_jq() {
  jq -r '.tag_name // empty' "$1"
}

json_release_tag_fallback() {
  awk -F'"' '
    $2 == "tag_name" {
      print $4
      exit
    }
  ' "$1"
}

json_release_tag() {
  if command -v jq >/dev/null 2>&1; then
    json_release_tag_jq "$1"
    return
  fi

  json_release_tag_fallback "$1"
}

json_release_asset_url_jq() {
  jq -r --arg regex "$2" '
    .assets[]
    | select((.name // "") | test($regex))
    | .browser_download_url // empty
  ' "$1" | sed -n '1p'
}

json_release_asset_url_fallback() {
  awk -F'"' -v regex="$2" '
    $2 == "name" {
      asset_name = $4
      next
    }
    $2 == "browser_download_url" {
      asset_url = $4
      if (asset_name ~ regex) {
        print asset_url
        exit
      }
    }
  ' "$1"
}

release_asset_url() {
  json_file="$1"
  asset_regex="$2"
  label="$3"

  if command -v jq >/dev/null 2>&1; then
    url="$(json_release_asset_url_jq "$json_file" "$asset_regex")"
  else
    url="$(json_release_asset_url_fallback "$json_file" "$asset_regex")"
  fi

  if [ -z "$url" ]; then
    case "$label" in
      "release archive") fail "no ${label} found for ${goarch}" ;;
      *) fail "no ${label} found" ;;
    esac
  fi

  printf '%s\n' "$url"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
    return
  fi

  fail "sha256sum or shasum is required to verify downloads"
}

checksum_for_archive() {
  checksums_file="$1"
  archive_name="$2"

  awk -v archive_name="$archive_name" '
    BEGIN {
      count = 0
    }
    {
      hash = $1
      name = $2
      sub(/^\*/, "", name)
      sub(/^.*\//, "", name)

      if (name == archive_name) {
        count++
        checksum = hash
      }
    }
    END {
      if (count == 1) {
        print checksum
        exit 0
      }
      if (count == 0) {
        exit 1
      }
      exit 2
    }
  ' "$checksums_file"
}

is_sha256_hex() {
  case "$1" in
    *[!0123456789abcdefABCDEF]* | "")
      return 1
      ;;
  esac

  [ "${#1}" -eq 64 ]
}

is_version_tag() {
  case "$1" in
    v*.*.*) ;;
    *) return 1 ;;
  esac

  printf '%s\n' "$1" | awk '
    /^v[0-9]+[.][0-9]+[.][0-9]+$/ {
      found = 1
    }
    END {
      exit found ? 0 : 1
    }
  '
}

lowercase() {
  printf '%s' "$1" | tr 'ABCDEFGHIJKLMNOPQRSTUVWXYZ' 'abcdefghijklmnopqrstuvwxyz'
}

verify_archive_checksum() {
  archive="$1"
  checksums_file="$2"
  archive_url="$3"

  archive_name="${archive_url##*/}"
  archive_name="${archive_name%%\?*}"

  checksum_status=0
  expected="$(checksum_for_archive "$checksums_file" "$archive_name")" || checksum_status=$?
  case "$checksum_status" in
    0) ;;
    1) fail "checksums.txt does not contain ${archive_name}" ;;
    2) fail "checksums.txt contains multiple entries for ${archive_name}" ;;
    *) fail "failed to read checksums.txt" ;;
  esac

  if ! is_sha256_hex "$expected"; then
    fail "checksums.txt contains malformed SHA-256 for ${archive_name}"
  fi

  expected="$(lowercase "$expected")"
  actual="$(lowercase "$(sha256_file "$archive")")"

  if [ "$actual" != "$expected" ]; then
    printf 'expected: %s\n' "$expected" >&2
    printf 'actual:   %s\n' "$actual" >&2
    fail "downloaded archive sha256 mismatch"
  fi
}

print_summary() {
  extras=""
  verify_cmd="${bin} --version"

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

  if ! path_contains_dir "$inst"; then
    verify_cmd="${inst}/${bin} --version"
  fi

  if is_tty_stdout && supports_color; then
    printf '%s%s%s\n' "$(style_stdout '1;32')" "${bin} installed" "$(style_stdout '0')"
    printf '  %sbinary:%s %s\n' "$(style_stdout '2')" "$(style_stdout '0')" "${inst}/${bin}"
    if [ -n "${release_tag:-}" ]; then
      printf '  %sversion:%s %s\n' "$(style_stdout '2')" "$(style_stdout '0')" "$release_tag"
    fi
    if [ -n "$extras" ]; then
      printf '  %sextras:%s %s\n' "$(style_stdout '2')" "$(style_stdout '0')" "$extras"
    fi
    printf '  %sverify:%s %s\n' "$(style_stdout '2')" "$(style_stdout '0')" "$verify_cmd"
    return
  fi

  printf '%s installed\n' "$bin"
  printf '  binary: %s\n' "${inst}/${bin}"
  if [ -n "${release_tag:-}" ]; then
    printf '  version: %s\n' "$release_tag"
  fi
  if [ -n "$extras" ]; then
    printf '  extras: %s\n' "$extras"
  fi
  printf '  verify: %s\n' "$verify_cmd"
}

print_path_hint() {
  if path_contains_dir "$inst"; then
    return
  fi

  rc_file="$(shell_rc_file)"
  export_line="$(shell_path_export)"

  warn "${inst} is not on your PATH"
  printf '  add to %s: %s\n' "$rc_file" "$export_line"
  printf '  then open a new shell and run: %s --version\n' "$bin"
}

shell_completion_rc_file() {
  shell_name="${SHELL##*/}"

  case "$shell_name" in
    zsh) printf '%s/.zshrc\n' "$HOME" ;;
    bash) printf '%s/.bashrc\n' "$HOME" ;;
    fish) printf '%s/.config/fish/config.fish\n' "$HOME" ;;
    *) return 1 ;;
  esac
}

print_completion_snippet() {
  shell_name="${SHELL##*/}"

  case "$shell_name" in
    zsh)
      printf "fpath=(\"%s\" \$fpath)\n" "$zshcompdir"
      printf 'autoload -U compinit && compinit\n'
      ;;
    bash)
      printf 'if [ -f /usr/share/bash-completion/bash_completion ]; then\n'
      printf '  . /usr/share/bash-completion/bash_completion\n'
      printf 'fi\n'
      ;;
    fish)
      printf 'set -Ua fish_complete_path "%s"\n' "$fishcompdir"
      ;;
    *)
      return 1
      ;;
  esac
}

shell_completion_works() {
  shell_name="${SHELL##*/}"

  case "$shell_name" in
    zsh)
      command -v zsh >/dev/null 2>&1 || return 1
      zsh -ic 'command -v aim >/dev/null 2>&1 || exit 1; autoload -U compinit; compinit >/dev/null 2>&1; [ "${_comps[aim]-}" = "_aim" ]' >/dev/null 2>&1
      ;;
    bash)
      command -v bash >/dev/null 2>&1 || return 1
      bash -ic 'command -v aim >/dev/null 2>&1 || exit 1; type _completion_loader >/dev/null 2>&1 || exit 1; _completion_loader aim >/dev/null 2>&1 || true; complete -p aim >/dev/null 2>&1' >/dev/null 2>&1
      ;;
    fish)
      command -v fish >/dev/null 2>&1 || return 1
      fish -ic 'command -q aim; and test (count (complete -C "aim ")) -gt 0' >/dev/null 2>&1
      ;;
    *)
      return 1
      ;;
  esac
}

print_completion_hint() {
  if [ "$bash_completion_installed" -eq 0 ] &&
     [ "$zsh_completion_installed" -eq 0 ] &&
     [ "$fish_completion_installed" -eq 0 ]; then
    return
  fi

  if shell_completion_works; then
    return
  fi

  shell_name="${SHELL##*/}"
  rc_file="$(shell_completion_rc_file 2>/dev/null || true)"

  case "$shell_name" in
    zsh)
      if [ "$zsh_completion_installed" -eq 1 ]; then
        printf '  zsh completion: %s\n' "${zshcompdir}/_${bin}"
        printf '  if completion is not available, add to %s:\n' "$rc_file"
        print_completion_snippet | sed 's/^/    /'
        printf '  then open a new shell.\n'
      fi
      ;;
    bash)
      if [ "$bash_completion_installed" -eq 1 ]; then
        printf '  bash completion: %s\n' "${bashcompdir}/${bin}"
        printf '  if completion is not available, add to %s:\n' "$rc_file"
        print_completion_snippet | sed 's/^/    /'
        printf '  then open a new shell.\n'
      fi
      ;;
    fish)
      if [ "$fish_completion_installed" -eq 1 ]; then
        printf '  fish completion: %s\n' "${fishcompdir}/${bin}.fish"
        printf '  if completion is not available, add to %s:\n' "$rc_file"
        print_completion_snippet | sed 's/^/    /'
        printf '  then open a new shell.\n'
      fi
      ;;
  esac
}

configure_shell_completion() {
  if [ "${AIM_INSTALL_SHELL_RC:-0}" != "1" ]; then
    return
  fi

  shell_name="${SHELL##*/}"
  rc_file="$(shell_completion_rc_file 2>/dev/null || true)"

  if [ -z "$rc_file" ]; then
    warn "AIM_INSTALL_SHELL_RC=1 is set, but shell ${shell_name:-unknown} is not supported for auto-configuration"
    return
  fi

  case "$shell_name" in
    zsh) [ "$zsh_completion_installed" -eq 1 ] || return ;;
    bash) [ "$bash_completion_installed" -eq 1 ] || return ;;
    fish) [ "$fish_completion_installed" -eq 1 ] || return ;;
    *) return ;;
  esac

  marker="# aim shell completion"
  if [ -f "$rc_file" ] && grep -Fq "$marker" "$rc_file"; then
    printf '  shell completion config already present in %s\n' "$rc_file"
    return
  fi

  mkdir -p "$(dirname "$rc_file")"
  {
    printf '\n%s\n' "$marker"
    print_completion_snippet
    printf '# end aim shell completion\n'
  } >> "$rc_file"

  printf '  added shell completion config to %s\n' "$rc_file"
}

repo="slobbe/appimage-manager"
bin="aim"
version="${AIM_VERSION:-}"
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
checksums="${tmpdir}/checksums.txt"
release_json="${tmpdir}/release.json"

if [ -z "$version" ]; then
  api_url="https://api.github.com/repos/${repo}/releases/latest"
  install_label="latest"
else
  if ! is_version_tag "$version"; then
    fail "AIM_VERSION must match v<major>.<minor>.<patch>"
  fi

  api_url="https://api.github.com/repos/${repo}/releases/tags/${version}"
  install_label="$version"
fi

section "Installing ${bin} ${install_label}"
curl -fsSL "$api_url" -o "$release_json"
release_tag="$(json_release_tag "$release_json")"
if [ -z "$release_tag" ]; then
  release_tag="$install_label"
fi

archive_url="$(release_asset_url "$release_json" "^aim-[0-9].*-linux-${goarch}[.]tar[.]gz$" "release archive")"
checksums_url="$(release_asset_url "$release_json" "^checksums[.]txt$" "checksums.txt")"

curl -fL "$archive_url" -o "$tgz"
curl -fL "$checksums_url" -o "$checksums"

verify_archive_checksum "$tgz" "$checksums" "$archive_url"

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
print_path_hint
configure_shell_completion
print_completion_hint
