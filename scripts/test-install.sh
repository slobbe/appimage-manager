#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)

cleanup() {
  rm -rf "$tmp_root"
}
trap cleanup EXIT INT TERM

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

assert_file_exists() {
  [ -f "$1" ] || fail "expected file to exist: $1"
}

assert_file_not_exists() {
  [ ! -e "$1" ] || fail "expected file not to exist: $1"
}

assert_file_contains() {
  grep -Fq "$2" "$1" || fail "expected $1 to contain: $2"
}

assert_file_not_contains() {
  ! grep -Fq "$2" "$1" || fail "expected $1 not to contain: $2"
}

assert_executable() {
  [ -x "$1" ] || fail "expected executable file: $1"
}

make_fake_tools() {
  fakebin="$tmp_root/fakebin"
  mkdir -p "$fakebin"

  cat >"$fakebin/uname" <<'EOF_UNAME'
#!/bin/sh
if [ "${1:-}" = "-m" ]; then
  printf '%s\n' "${AIM_TEST_UNAME:-x86_64}"
  exit 0
fi
printf '%s\n' "${AIM_TEST_UNAME:-x86_64}"
EOF_UNAME
  chmod +x "$fakebin/uname"

  cat >"$fakebin/curl" <<'EOF_CURL'
#!/bin/sh
set -eu

output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      shift
      [ "$#" -gt 0 ] || exit 2
      output="$1"
      ;;
    -*) ;;
    *) url="$1" ;;
  esac
  shift
done

[ -n "$output" ] || exit 2
[ -n "$url" ] || exit 2
[ -n "${AIM_TEST_FIXTURES:-}" ] || exit 2

printf '%s\n' "$url" >>"${AIM_TEST_CURL_LOG:-/dev/null}"

case "$url" in
  https://api.github.com/repos/slobbe/appimage-manager/releases/latest)
    cp "${AIM_TEST_RELEASE_JSON}" "$output"
    ;;
  https://api.github.com/repos/slobbe/appimage-manager/releases/tags/*)
    cp "${AIM_TEST_RELEASE_JSON}" "$output"
    ;;
  https://example.invalid/aim-v1.2.3-linux-amd64.tar.gz)
    cp "${AIM_TEST_FIXTURES}/aim-v1.2.3-linux-amd64.tar.gz" "$output"
    ;;
  https://example.invalid/checksums.txt)
    cp "${AIM_TEST_CHECKSUMS}" "$output"
    ;;
  *)
    printf 'unexpected curl URL: %s\n' "$url" >&2
    exit 22
    ;;
esac
EOF_CURL
  chmod +x "$fakebin/curl"
}

make_aim_script() {
  destination="$1"
  cat >"$destination" <<'EOF_AIM'
#!/bin/sh
set -eu

if [ "${1:-}" = "--version" ]; then
  printf 'aim v1.2.3\n'
  exit 0
fi

if [ "${1:-}" = "gen" ] && [ "${2:-}" = "man" ] && [ "${3:-}" = "--dir" ]; then
  mkdir -p "$4"
  printf '.TH AIM 1\n' >"$4/aim.1"
  exit 0
fi

if [ "${1:-}" = "gen" ] && [ "${2:-}" = "completion" ] && [ "${4:-}" = "--dir" ]; then
  mkdir -p "$5"
  case "$3" in
    bash) printf '# bash completion\n' >"$5/aim.bash" ;;
    zsh) printf '# zsh completion\n' >"$5/_aim" ;;
    fish) printf '# fish completion\n' >"$5/aim.fish" ;;
    *) exit 1 ;;
  esac
  exit 0
fi

printf 'unexpected aim invocation: %s\n' "$*" >&2
exit 1
EOF_AIM
  chmod +x "$destination"
}

make_fixtures() {
  fixtures="$tmp_root/fixtures"
  archive_dir="$tmp_root/archive"
  mkdir -p "$fixtures" "$archive_dir"

  make_aim_script "$archive_dir/aim"
  tar -czf "$fixtures/aim-v1.2.3-linux-amd64.tar.gz" -C "$archive_dir" aim

  archive_hash=$(sha256sum "$fixtures/aim-v1.2.3-linux-amd64.tar.gz" | awk '{ print $1 }')
  printf '%s  aim-v1.2.3-linux-amd64.tar.gz\n' "$archive_hash" >"$fixtures/checksums.txt"
  printf '%064d  aim-v1.2.3-linux-amd64.tar.gz\n' 0 >"$fixtures/bad-checksums.txt"

  cat >"$fixtures/release.json" <<'EOF_RELEASE'
{
  "tag_name": "v1.2.3",
  "assets": [
    {
      "name": "aim-v1.2.3-linux-amd64.tar.gz",
      "browser_download_url": "https://example.invalid/aim-v1.2.3-linux-amd64.tar.gz"
    },
    {
      "name": "checksums.txt",
      "browser_download_url": "https://example.invalid/checksums.txt"
    }
  ]
}
EOF_RELEASE

  cat >"$fixtures/release-missing-archive.json" <<'EOF_MISSING_ARCHIVE'
{
  "tag_name": "v1.2.3",
  "assets": [
    {
      "name": "checksums.txt",
      "browser_download_url": "https://example.invalid/checksums.txt"
    }
  ]
}
EOF_MISSING_ARCHIVE

  cat >"$fixtures/release-missing-checksums.json" <<'EOF_MISSING_CHECKSUMS'
{
  "tag_name": "v1.2.3",
  "assets": [
    {
      "name": "aim-v1.2.3-linux-amd64.tar.gz",
      "browser_download_url": "https://example.invalid/aim-v1.2.3-linux-amd64.tar.gz"
    }
  ]
}
EOF_MISSING_CHECKSUMS
}

run_installer() {
  test_name="$1"
  release_json="$2"
  checksums_file="$3"
  arch="$4"
  version="$5"
  install_dir="$tmp_root/$test_name/install"
  data_home="$tmp_root/$test_name/data"
  home_dir="$tmp_root/$test_name/home"
  output_file="$tmp_root/$test_name/out"
  error_file="$tmp_root/$test_name/err"
  curl_log="$tmp_root/$test_name/curl.log"

  mkdir -p "$install_dir" "$data_home" "$home_dir"
  : >"$curl_log"

  set +e
  PATH="$fakebin:$PATH" \
  HOME="$home_dir" \
  SHELL=/bin/sh \
  AIM_INSTALL_DIR="$install_dir" \
  XDG_DATA_HOME="$data_home" \
  AIM_TEST_FIXTURES="$fixtures" \
  AIM_TEST_RELEASE_JSON="$release_json" \
  AIM_TEST_CHECKSUMS="$checksums_file" \
  AIM_TEST_UNAME="$arch" \
  AIM_TEST_CURL_LOG="$curl_log" \
  AIM_VERSION="$version" \
    sh "$repo_root/scripts/install.sh" >"$output_file" 2>"$error_file"
  status=$?
  set -e

  printf '%s\n' "$status" >"$tmp_root/$test_name/status"
}

status_for() {
  cat "$tmp_root/$1/status"
}

installed_aim() {
  printf '%s/%s/install/aim\n' "$tmp_root" "$1"
}

stdout_for() {
  printf '%s/%s/out\n' "$tmp_root" "$1"
}

stderr_for() {
  printf '%s/%s/err\n' "$tmp_root" "$1"
}

curl_log_for() {
  printf '%s/%s/curl.log\n' "$tmp_root" "$1"
}

test_successful_install() {
  run_installer success "$fixtures/release.json" "$fixtures/checksums.txt" x86_64 ""
  [ "$(status_for success)" -eq 0 ] || fail "successful install exited $(status_for success): $(cat "$(stderr_for success)")"
  assert_file_exists "$(installed_aim success)"
  assert_executable "$(installed_aim success)"
  assert_file_contains "$(stdout_for success)" "aim installed"
  assert_file_exists "$tmp_root/success/data/man/man1/aim.1"
  assert_file_exists "$tmp_root/success/data/bash-completion/completions/aim"
  assert_file_exists "$tmp_root/success/data/zsh/site-functions/_aim"
  assert_file_exists "$tmp_root/success/data/fish/vendor_completions.d/aim.fish"
}

test_unsupported_architecture() {
  run_installer unsupported-arch "$fixtures/release.json" "$fixtures/checksums.txt" riscv64 ""
  [ "$(status_for unsupported-arch)" -ne 0 ] || fail "unsupported architecture unexpectedly succeeded"
  assert_file_contains "$(stderr_for unsupported-arch)" "unsupported architecture"
  assert_file_not_exists "$(installed_aim unsupported-arch)"
  assert_file_not_contains "$(curl_log_for unsupported-arch)" "https://api.github.com"
}

test_missing_archive_asset() {
  run_installer missing-archive "$fixtures/release-missing-archive.json" "$fixtures/checksums.txt" x86_64 ""
  [ "$(status_for missing-archive)" -ne 0 ] || fail "missing archive asset unexpectedly succeeded"
  assert_file_contains "$(stderr_for missing-archive)" "no release archive found"
  assert_file_not_exists "$(installed_aim missing-archive)"
}

test_missing_checksum_asset() {
  run_installer missing-checksums "$fixtures/release-missing-checksums.json" "$fixtures/checksums.txt" x86_64 ""
  [ "$(status_for missing-checksums)" -ne 0 ] || fail "missing checksum asset unexpectedly succeeded"
  assert_file_contains "$(stderr_for missing-checksums)" "no checksums.txt found"
  assert_file_not_exists "$(installed_aim missing-checksums)"
}

test_checksum_mismatch_does_not_replace() {
  mkdir -p "$tmp_root/checksum-mismatch/install"
  printf 'old aim\n' >"$tmp_root/checksum-mismatch/install/aim"

  run_installer checksum-mismatch "$fixtures/release.json" "$fixtures/bad-checksums.txt" x86_64 ""
  [ "$(status_for checksum-mismatch)" -ne 0 ] || fail "checksum mismatch unexpectedly succeeded"
  assert_file_contains "$(stderr_for checksum-mismatch)" "downloaded archive sha256 mismatch"
  assert_file_contains "$(installed_aim checksum-mismatch)" "old aim"
}

test_specific_version_uses_tag_api() {
  run_installer version-tag "$fixtures/release.json" "$fixtures/checksums.txt" x86_64 "1.2.3"
  [ "$(status_for version-tag)" -eq 0 ] || fail "specific version install exited $(status_for version-tag): $(cat "$(stderr_for version-tag)")"
  assert_file_contains "$(curl_log_for version-tag)" "https://api.github.com/repos/slobbe/appimage-manager/releases/tags/v1.2.3"
  assert_file_exists "$(installed_aim version-tag)"
}

test_invalid_version_is_rejected() {
  run_installer invalid-version "$fixtures/release.json" "$fixtures/checksums.txt" x86_64 "latest"
  [ "$(status_for invalid-version)" -ne 0 ] || fail "invalid AIM_VERSION unexpectedly succeeded"
  assert_file_contains "$(stderr_for invalid-version)" "AIM_VERSION must match"
  assert_file_not_contains "$(curl_log_for invalid-version)" "https://api.github.com"
}

make_fake_tools
make_fixtures

test_successful_install
test_unsupported_architecture
test_missing_archive_asset
test_missing_checksum_asset
test_checksum_mismatch_does_not_replace
test_specific_version_uses_tag_api
test_invalid_version_is_rejected

printf 'installer script behavior tests passed\n'
