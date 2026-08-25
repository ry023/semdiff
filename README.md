# semdiff

`semdiff` reorganizes a Git range into semantic review groups without changing Git history.

## Install

From a local checkout:

```sh
go install .
```

This installs `semdiff` into `GOBIN`, or into `$(go env GOPATH)/bin` when `GOBIN` is unset. To install directly from the repository after a release is available:

```sh
go install github.com/ry023/semdiff@latest
```

Make sure the install directory is included in `PATH`.

## Usage

```sh
semdiff commits main..HEAD --json
semdiff fragments main..HEAD --json
semdiff classify main..HEAD --json
semdiff show F-0123456789ab --json
semdiff validate groups.json
semdiff view groups.json --addr 127.0.0.1:8080
```

`fragments` caches the latest complete inventory under `.semdiff/`; its JSON output omits patches so an agent can inspect individual changes with `show`. A `groups.json` records full resolved commit SHAs and version `1`. See [the grouping skill](skills/semantic-grouping/SKILL.md) for the staged agent workflow.

`classify` provides a deterministic draft based only on changed file paths, names, extensions, and directory structure. The grouping agent should review and finalize those suggestions per Group.

New `groups.json` files attach a review explanation to every fragment:

```json
{
  "id": "domain-change",
  "title": "Introduce domain change",
  "summary": "Introduces the new domain behavior.",
  "file_categories": [
    {"path": "src/domain.ts", "category": "logic"}
  ],
  "fragments": [
    {
      "id": "F-0123456789ab",
      "description": "Defines the domain type used by the new workflow."
    }
  ]
}
```

Legacy `fragment_ids` arrays remain supported for existing files.
