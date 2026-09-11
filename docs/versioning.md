# Versioning

semdiff has separate product and artifact versions. The CLI and bundled plugin are one release unit; `groups.json` has an independent schema version because artifacts may outlive the binary that created them.

## Product releases

The root `VERSION` file is the source of truth for the CLI and plugin version. The Codex manifest, Claude manifest, and Claude marketplace entry must match it. Git release tags add a `v` prefix, so product version `0.3.0` is tagged `v0.3.0`.

While the product is below `1.0.0`, patch releases contain compatible fixes and minor releases may add features or break compatibility. Starting at `1.0.0`, releases follow the standard SemVer major/minor/patch compatibility rules. semdiff currently publishes release-only `MAJOR.MINOR.PATCH` versions without prerelease or build metadata.

The plugin and CLI use the same version at release time. A plugin may use any CLI patch release in the same minor line; plugin `0.3.x` therefore accepts CLI versions `>=0.3.0, <0.4.0`.

## Automated product releases

The release workflow on `main` uses [tagpr](https://github.com/Songmu/tagpr) to keep a release pull request open for unreleased changes. `.tagpr` lists `VERSION` and all three plugin manifests as version files, so the release pull request keeps the CLI and bundled plugins synchronized. Pull request labels can select a minor or major bump; otherwise tagpr proposes a patch release.

Merging the release pull request creates the `v<VERSION>` tag and a draft GitHub Release with tagpr's generated release notes. The same workflow passes tagpr's tag output to GoReleaser, which uses that draft, uploads the cross-platform archives and `checksums.txt`, and publishes it. Release tags should therefore be created through the tagpr release pull request so the draft Release and its notes exist before GoReleaser runs.

Homebrew publishing is not configured yet and will be added separately.

## `groups.json` schema

Finalized reviews declare both their format and schema version:

```json
{
  "format": "semdiff.groups",
  "format_version": "1.0.0"
}
```

The schema version is independent of the product version. A schema major release is incompatible, a minor release is a backward-compatible format addition, and a patch release does not change the wire contract. Readers accept only explicitly tested minor lines; they do not assume an unknown future minor is safe. The CLI checks this envelope before decoding the full artifact or reading Git state.

| CLI/plugin | Readable `groups.json` | Written `groups.json` |
|---|---|---|
| `0.3.x` | `semdiff.groups >=1.0.0, <1.1.0` | `semdiff.groups 1.0.0` |

Numeric `version` fields belong to the legacy format. CLI `0.3.x` does not migrate or read those artifacts; regenerate them or use the older unversioned CLI that created them.

Draft schema version 4, question file version 2, and answer-session version 1 are local state formats. They remain independent of both the product version and finalized artifact schema.

## Release checklist

1. Update the compatibility table and the CLI's schema read/write constants when schema support changes.
2. Update both bundled skills when their accepted CLI minor line changes.
3. Run `gofmt`, `go test ./...`, plugin validation, and `git diff --check`.
4. Merge the tagpr release pull request. It updates `VERSION` and the plugin manifests, creates `v<VERSION>` and a draft Release with generated notes, then GoReleaser uploads the artifacts and publishes it.
