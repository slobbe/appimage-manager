# Local AppImage inspection design

Plan: [018-design-local-appimage-inspection.md](018-design-local-appimage-inspection.md)

## Command shape

Local inspection should be implemented as `aim info <path>` rather than a new `aim inspect <path>` command for now.

Rationale:

- `info` already represents the user goal: show metadata about an AppImage or managed app.
- The README previously described `aim info ./Example.AppImage`, so restoring that behavior resolves the documented contract without adding another command.
- A future `inspect` command can still be added if richer previews, diffs, or asset browsing outgrow `info`.

## Output shape

Text output should include the same core fields for installed and local targets:

- title: `[app-id] Name (vVersion)` when an app ID can be derived, otherwise `Name (vVersion)`
- `Status: installed|not installed`
- `Exec path: <installed path or local path>`
- source fields (`Source`, `Original file`, `Integrated at` when applicable)
- update source fields (`Update source`, `Embedded update`, raw/parsed details when available)

JSON output should include:

- `id` when metadata can derive one
- `name`
- `version`
- `exec_path`
- `installed` boolean
- `target_kind`, currently `installed` or `local_path`
- `source`
- `update_source`

Local inspection returns `installed: false` and `target_kind: "local_path"`. The local file path is reported as `exec_path` and `source.local_file.path`. `source.local_file.integrated_at` is omitted because the file has not been integrated.

## Security wording

Local inspection is not a static parse or sandbox. It reuses the AppImage extractor, which executes the AppImage's extraction and update-info modes to obtain desktop metadata, icons, and embedded update information. User-facing help/docs must say to inspect only AppImages the user trusts.

## App-layer ports and no-write guarantee

Local inspection reuses app-defined ports already used during integration:

- `WorkspaceProvider`
- `AppImageStager`
- `AppImageExtractor`
- `DesktopEntryDiscoverer`
- `IconDiscoverer`

The app workflow stops after metadata discovery. It must not call:

- `AppImageInstaller`
- `IconInstaller`
- `DesktopEntryInstaller`
- remover rollback paths for installed artifacts
- `AppRepository.Save`

It also performs no network calls. Repository lookup is skipped for targets that conservatively look like local AppImage paths, including absolute paths, `./` or `../` paths, paths containing a path separator, and names ending in `.AppImage`.
