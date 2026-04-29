#!/bin/sh
set -eu

is_tty_stdout() {
  [ -t 1 ]
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

remove_file() {
  path="$1"

  if [ -e "$path" ] || [ -L "$path" ]; then
    rm -f "$path"
    printf 'removed: %s\n' "$path"
  fi
}

bin="aim"
inst="${HOME}/.local/bin"
data_home="${XDG_DATA_HOME:-${HOME}/.local/share}"
mandir="${data_home}/man/man1"
bashcompdir="${data_home}/bash-completion/completions"
zshcompdir="${data_home}/zsh/site-functions"
fishcompdir="${data_home}/fish/vendor_completions.d"

section "Uninstalling ${bin}"

remove_file "${inst}/${bin}"
remove_file "${mandir}/${bin}.1"
remove_file "${bashcompdir}/${bin}"
remove_file "${zshcompdir}/_${bin}"
remove_file "${fishcompdir}/${bin}.fish"

if is_tty_stdout && supports_color; then
  printf '%s%s%s\n' "$(style_stdout '1;32')" "${bin} uninstalled" "$(style_stdout '0')"
else
  printf '%s uninstalled\n' "$bin"
fi
printf 'Managed AppImages, config, state, cache, and shell rc files were not removed.\n'
