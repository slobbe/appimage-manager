## Roadmap

The roadmap focuses on making aim a trustworthy CLI package manager for AppImages. Priorities are listed in order; a graphical interface is out of scope.

### 1. Broader update support

- support embedded static-URL update sources and `zsync` updates
- distinguish resolving `zsync` metadata from performing differential downloads
- expand beyond GitHub with additional release providers
- preserve explicit source selection and report unsupported sources clearly

### 2. Previous-version retention, rollback, and pinning

- retain the previous working version after an update
- add an explicit rollback workflow that restores the AppImage and its integration
- support version pinning to exclude selected apps from routine updates
- define retention and cleanup behavior for previous versions

### 3. Download verification and publisher authenticity

- define, persist, and display verification policy for update sources
- support digest verification for download integrity
- support signature verification for publisher authenticity where available
- report verification outcomes and distinguish unverified downloads from verified ones
- apply verification to aim's self-update workflow

### 4. Repair, cleanup, and history

- expand `diagnose` to detect missing files, broken records, orphaned artifacts,
  invalid update metadata, and stale staging files
- add `repair` and `clean` workflows with previews for destructive changes
- record update history and make recovery actions discoverable
- make common partial failures recoverable without manually editing `apps.json`

### 5. Stable machine-readable contracts

- version and stabilize JSON schemas for inspection and mutation results
- stabilize exit codes and statuses for no-op, cancellation, skipped work,
  partial failure, and failure
- preserve predictable stdout/stderr separation and non-interactive behavior
- define compatibility and migration guarantees for persisted state
- verify these contracts with black-box CLI tests
