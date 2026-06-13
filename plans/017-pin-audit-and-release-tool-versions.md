# Plan 017: Pin audit and release tool versions for reproducible CI

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report — do not improvise. When done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat 0cee363..HEAD -- Makefile .github/workflows/ci.yml .github/workflows/release.yml README.md`

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `0cee363`, 2026-06-13

## Why this matters

CI installs `govulncheck@latest` and uses GoReleaser `~> v2`. Those tools can change behavior without a repo diff, making failures harder to reproduce and release behavior less predictable. Pinning versions makes CI/release changes intentional and reviewable.

## Current state

Relevant files:

- `Makefile` — installs audit tools.
- `.github/workflows/ci.yml` — installs audit tools and checks GoReleaser config.
- `.github/workflows/release.yml` — runs GoReleaser release.

Current tool references:

```make
# Makefile:6,60-62
GOVULNCHECK ?= $(shell go env GOPATH)/bin/govulncheck
install-tools:
	go install golang.org/x/vuln/cmd/govulncheck@latest
```

```yaml
# .github/workflows/ci.yml:90-97
- name: Install audit tools
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    sudo apt-get update
    sudo apt-get install -y shellcheck
```

```yaml
# .github/workflows/ci.yml:115-119 and release.yml:176-180
uses: goreleaser/goreleaser-action@v7
with:
  distribution: goreleaser
  version: "~> v2"
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Install pinned tools | `make install-tools` | exit 0; installs pinned govulncheck |
| Audit | `make audit` | exit 0, assuming Plan 006 fixed Go stdlib vulnerabilities |
| GoReleaser check | `goreleaser check` | exit 0 if installed locally; otherwise rely on CI config review |
| Full gate | `make verify` | exit 0 |

## Scope

**In scope**:

- `Makefile`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- Optional docs note in `README.md` or comments in `Makefile`
- `plans/README.md` status update only

**Out of scope**:

- Changing release workflow semantics.
- Pinning apt `shellcheck` package version; distro package pinning is a separate decision.
- Upgrading Go patch version; see Plan 006.

## Steps

### Step 1: Choose explicit versions

Choose current stable explicit versions compatible with the repo:

- `golang.org/x/vuln/cmd/govulncheck@<version>`: use a real module version, not `latest`.
- GoReleaser action `version`: use a concrete v2 version string, not `~> v2`.

If you cannot verify current versions from the environment or network, STOP and ask the operator which versions to pin.

**Verify**: record chosen versions in the PR/plan update; no command required.

### Step 2: Centralize govulncheck version in Makefile

Add a variable such as:

```make
GOVULNCHECK_VERSION ?= vX.Y.Z
```

Change `install-tools` to:

```make
go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
```

Change CI install step to call `make install-tools` instead of duplicating `go install ...@latest`, or use the same Make variable explicitly.

**Verify**: `make install-tools` → exit 0.

### Step 3: Pin GoReleaser action versions

In both CI and release workflows, replace:

```yaml
version: "~> v2"
```

with the chosen concrete version, for example:

```yaml
version: "v2.x.y"
```

Use the same version in both workflow files.

**Verify**: visually confirm `.github/workflows/ci.yml` and `.github/workflows/release.yml` match.

### Step 4: Run available checks

**Verify**:

- `make audit` → exit 0 if Plan 006 has landed; if it fails only due to known Go stdlib vulnerabilities, note that Plan 006 must land first.
- `make verify` → exit 0.
- If `goreleaser` is installed locally: `goreleaser check` → exit 0.

## Done criteria

- [ ] No `govulncheck@latest` remains in `Makefile` or workflows.
- [ ] CI and local `make install-tools` use the same pinned `GOVULNCHECK_VERSION`.
- [ ] CI and release workflows use the same concrete GoReleaser v2 version.
- [ ] `make install-tools` exits 0.
- [ ] `make verify` exits 0.
- [ ] `plans/README.md` status row updated.

## STOP conditions

Stop and report if:

- A concrete compatible tool version cannot be verified.
- Pinning GoReleaser changes release config validation behavior.
- `make audit` fails for new vulnerabilities unrelated to Plan 006.

## Maintenance notes

Pinned tools should be updated deliberately in maintenance PRs. Reviewers should reject future `@latest` tool installs in CI unless the repo explicitly chooses non-reproducible tool behavior.
