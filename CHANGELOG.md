# Changelog

Notable changes to AppImage Manager are documented here, starting with v0.20.0.

## v0.20.0 (unreleased)

### New

- Added `--tag-pattern` to GitHub add and update-source commands to persist release-tag filters for updates.
- Added `minimum_release_age_hours` and `aim update --ignore-release-age` to configure or bypass the wait, with deferred releases shown in human and JSON output.

### Changed

- Made update checks and AppImage downloads concurrent, with individual download progress indicators.
- Delayed stable-only AppImage updates until releases were one hour old by default, without falling back to older releases.
