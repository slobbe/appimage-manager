# Hub-Backed Search and Catalog Plan

## Summary

`aim search` is intentionally deferred until it can be backed by a reliable discovery source instead of raw provider search APIs.

Candidate discovery sources:

- Nostr / Zapstore
- another hub, index, or catalog
- a maintained manifest or registry

Until that exists, shipped package acquisition remains:

- `aim show github:owner/repo`
- `aim show gitlab:namespace/project`
- `aim install github:owner/repo`
- `aim install gitlab:namespace/project`

## Problem Statement

The first `search` implementation queried GitHub and GitLab search APIs directly, then filtered those results to repositories and projects with installable AppImage release assets.

That approach was rolled back because it does not meet package-manager expectations:

- direct provider refs are installable and resolvable
- provider search APIs are unreliable for package discovery UX
- GitHub search can rate limit or deny requests
- raw provider repo search is not the same thing as a package catalog

Package-manager-style search should query a source that is designed to behave like an index.

## Future CLI

Planned future surface:

```sh
aim search <query>
aim show <result-id|hub-ref|provider-ref>
aim install <result-id|hub-ref|provider-ref>
```

Rules:

- numeric result IDs come only from the latest cached hub-backed search results
- provider refs remain valid direct inputs
- `show` and `install` must continue to work without prior search when given a direct provider ref

## Discovery Model

Introduce a provider-neutral catalog layer.

Recommended types:

- `CatalogRef`
- `CatalogResult`
- `PackageMetadata`
- `CatalogBackend`

Recommended shape:

- `CatalogRef`
  - hub kind
  - stable hub-specific identifier
  - optional provider ref
- `CatalogResult`
  - numeric display ID
  - display name
  - catalog source
  - provider ref
  - latest version
  - selected asset name
  - installable flag
  - score
- `PackageMetadata`
  - existing provider-ref metadata plus catalog provenance
- `CatalogBackend`
  - `Search(ctx, query, limit)`
  - `Resolve(ctx, ref, assetOverride)`

Initial backend candidates:

- Nostr / Zapstore
- a maintained manifest backend

Optional future backend:

- provider fallback discovery, but only if it satisfies reliability and package-quality requirements

## Installability Rules

`aim search` should return installable results only.

A result is installable only when all of the following are true:

- the current release asset can be resolved
- the selected asset is an AppImage
- the selected asset matches the current machine architecture
- the provider/update provenance is sufficient to configure updates after install
- required metadata can be rendered by `aim show`

Results that cannot be installed immediately should be filtered out instead of shown as dead ends.

## Trust and Metadata Fields

Future search/show metadata should include:

- display name
- source hub or catalog
- provider ref
- selected AppImage asset name
- latest version or release tag
- download URL
- update-source type that will be configured after install
- trust notes or provenance notes

Recommended trust notes:

- which hub produced the result
- which provider ref will actually be installed
- which release and asset were selected
- whether asset selection came from defaults or an explicit override

## Cache and Result IDs

When search returns, it should cache the latest result set under the XDG cache tree.

Recommended behavior:

- store the latest search result set under `${XDG_CACHE_HOME:-~/.cache}/appimage-manager`
- assign simple numeric IDs like `1`, `2`, `3`
- scope those IDs only to the latest cached search output
- overwrite the prior cache on each new search

Error behavior:

- `aim show 3` should fail cleanly if no cached search exists
- `aim install 3` should fail cleanly if ID `3` is not in the latest cached search results

## Implementation Direction

1. Add a new hub-backed discovery package or extend `internal/discovery` with catalog backends.
2. Keep provider-ref resolution separate from search-result resolution.
3. Resolve search results to concrete provider refs before installation.
4. Reuse existing GitHub and GitLab release resolution for final installability checks.
5. Keep `aim install github:...` and `aim install gitlab:...` as direct, non-catalog fallback flows.

## Acceptance Criteria

- search returns installable results only
- a query like `obsidian` works when catalog data exists
- `show <id>` resolves metadata from the latest cached hub-backed search results
- `install <id>` installs from the latest cached hub-backed search results
- direct provider refs still work without prior search
- provider API outages do not break hub-backed search if the hub remains healthy
- `show` output includes installability and provenance details
- `install` reuses the existing remote add and update-source persistence flow

## Testing

Required future tests:

- search returns installable results from a hub backend
- non-installable hub results are filtered out
- search cache writes and overwrites correctly
- `show` resolves both cached IDs and direct provider refs
- `install` resolves both cached IDs and direct provider refs
- hub result IDs are stable within the latest search output
- provider outages do not fail hub-backed search when hub data is still available

## Assumptions and Defaults

- future `search` should behave like package-manager catalog search, not raw provider repo search
- direct provider refs remain supported even after hub-backed search exists
- result IDs are intentionally scoped to the latest cached search output, not globally stable
- Nostr / Zapstore is a likely first backend, but the architecture should not depend on it being the only one
