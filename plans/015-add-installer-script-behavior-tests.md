# Plan 015: Add behavioral tests for `scripts/install.sh`

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- scripts/install.sh scripts Makefile .github/workflows/ci.yml`

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

The README’s primary install path is `curl -fsSL .../scripts/install.sh | sh`. The script performs release metadata lookup, asset selection, checksum verification, extraction, binary install, and optional manpage/completion generation. CI currently only runs `shellcheck`; static checks cannot catch broken JSON parsing, checksum lookup, archive layout assumptions, or install-directory regressions.

## Current state

Relevant files:

- `scripts/install.sh` — release installer script.
- `Makefile` — has `shellcheck` but no script behavior test target.
- `.github/workflows/ci.yml` — `make audit` installs shellcheck and runs static script audit.

Installer main flow excerpt:

```sh
# scripts/install.sh:549-572
section "Installing ${bin} ${install_label}"
curl -fsSL "$api_url" -o "$release_json"
release_tag="$(json_release_tag "$release_json")"
archive_url="$(release_asset_url "$release_json" "^aim-v?[0-9].*-linux-${goarch}[.]tar[.]gz$" "release archive")"
checksums_url="$(release_asset_url "$release_json" "^checksums[.]txt$" "checksums.txt")"
curl -fL "$archive_url" -o "$tgz"
curl -fL "$checksums_url" -o "$checksums"
verify_archive_checksum "$tgz" "$checksums" "$archive_url"
tar -xzf "$tgz" -C "$tmpdir"
found="$(find "$tmpdir" -type f \( -name "${bin}" -o -name "${bin}-*-linux-${goarch}" \) | head -n 1)"
chmod +x "$found"
mv -f "$found" "${inst}/${bin}"
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Script behavior tests | `make test-installer-script` | exit 0 |
| Shellcheck | `make shellcheck` | exit 0 |
| Audit | `make audit` | exit 0, assuming Plan 006 fixed govulncheck |
| Full tests | `go test ./...` | exit 0 |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- New shell test harness under `scripts/` or `scripts/testdata/`
- `Makefile` target such as `test-installer-script`
- `.github/workflows/ci.yml` only if wiring the new target into CI/audit
- `scripts/install.sh` only for small testability fixes discovered by tests
- `plans/README.md` status update only

**Out of scope**:

- Rewriting the installer in Go.
- Changing public installer environment variables unless a test proves they are broken.
- Networked tests; tests must be hermetic.

## Steps

### Step 1: Create a hermetic shell test harness

Add a script such as `scripts/test-install.sh` that:

- Runs with `set -eu`.
- Creates a temp directory for fake `PATH`, fake release assets, install dir, and XDG data home.
- Provides fake `curl` that maps requested URLs to local fixture files.
- Provides fake or real `uname` behavior for `x86_64`/`aarch64` tests if practical.
- Executes `scripts/install.sh` in a subprocess with `AIM_INSTALL_DIR` and `XDG_DATA_HOME` pointing into temp dirs.

Keep output assertions stable; prefer checking files/exits over exact prose.

**Verify**: `sh scripts/test-install.sh` → may fail before fixtures are complete.

### Step 2: Add successful install fixture test

Create local fixtures for:

- Release JSON containing an archive asset URL and `checksums.txt` URL.
- A tar.gz containing an executable-like `aim` file.
- A checksums file matching the archive hash.

Assert successful run installs `${AIM_INSTALL_DIR}/aim`, makes it executable, and exits 0. If manpage/completion generation invokes the installed binary and complicates hermetic tests, provide a fake binary that supports the needed completion/man commands or document why those paths are skipped/faked.

**Verify**: `sh scripts/test-install.sh` → success case passes.

### Step 3: Add failure cases

Add tests for:

- Unsupported architecture returns non-zero and prints `unsupported architecture`.
- Missing archive asset returns non-zero.
- Missing checksum asset returns non-zero.
- Checksum mismatch returns non-zero and does not install/replace `aim`.
- Specific `AIM_VERSION` uses the tag release API path and validates semver input.

**Verify**: `sh scripts/test-install.sh` → all cases pass.

### Step 4: Wire into Makefile and CI

Add a `Makefile` target:

```make
.PHONY: test-installer-script
test-installer-script:
	sh scripts/test-install.sh
```

Decide whether to include it in `audit` or a new CI step. Prefer adding it to `audit` after `shellcheck` if runtime is short and hermetic.

**Verify**: `make test-installer-script` → exit 0.

### Step 5: Run full verification

**Verify**:

- `make shellcheck` → exit 0.
- `make test-installer-script` → exit 0.
- `go test ./...` → exit 0.
- `make verify` → exit 0.

## Done criteria

- [ ] A hermetic installer behavior test target exists.
- [ ] Tests cover success, unsupported architecture, missing assets, checksum mismatch, and version input behavior.
- [ ] Tests do not call the real network or write outside temp dirs.
- [ ] `make shellcheck`, `make test-installer-script`, `go test ./...`, and `make verify` pass.
- [ ] CI runs the new target or documentation explains why it remains local-only.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- Hermetic tests require extensive installer rewrites beyond small testability fixes.
- Tests would need real GitHub/network calls.
- Shell portability constraints conflict with the current `/bin/sh` script style.

## Maintenance notes

Keep installer tests focused on behavior and file effects, not exact wording. Future installer features should add fixtures before changing `scripts/install.sh`.
