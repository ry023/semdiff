# semdiff

`semdiff` reorganizes a Git range into semantic review groups without changing Git history. A fragment is defined by file line ranges, not by Git hunk boundaries.

## Install

```sh
go install .
```

## Usage

```sh
semdiff commits main..HEAD --json
semdiff fragments main..HEAD --json
semdiff classify main..HEAD --json
semdiff grouping init main..HEAD --json
semdiff grouping inspect --suggestions --json
semdiff grouping status --json
semdiff grouping inspect --unassigned --json
semdiff grouping apply decisions.json --json
semdiff grouping finalize groups.json --json
semdiff show --draft .semdiff/grouping-draft.json F-0123456789ab --json
semdiff show groups.json F-0123456789ab --json
semdiff validate groups.json
semdiff view groups.json --addr 127.0.0.1:8080
```

`fragments` emits Git-derived zero-context ranges as initial suggestions. They are not canonical fragment boundaries. `grouping init` stores suggestions separately from authored fragments in `.semdiff/grouping-draft.json`, so each Git span does not become an assignment obligation. Inspect them with `grouping inspect --suggestions`, then use `merge_fragments`, `add_fragment`, `update_fragment`, and `delete_fragments` to compose semantic fragments before finalizing.

`classify` suggests the standard categories `logic`, `component`, `config`, `implementation`, `test`, `docs`, and `unknown` from paths. Documentation extensions such as Markdown and reStructuredText, conventional documentation filenames such as `README` and `CHANGELOG`, and files under documentation directories such as `docs/` and `guides/` are classified as `docs`.

This workflow uses draft schema version 2. Re-run `grouping init --force` to replace a version 1 draft. The final `groups.json` schema remains version 1.

`groups.json` is the source of truth. Every fragment contains its path, one or more old/new ranges, and its semantic description:

```json
{
  "version": 1,
  "base_sha": "<full base SHA>",
  "head_sha": "<full head SHA>",
  "groups": [
    {
      "id": "domain-change",
      "title": "Introduce domain change",
      "summary": "Introduces the shared domain boundary and its validation.",
      "order": 1,
      "file_categories": [
        {"path": "src/domain.ts", "category": "logic"}
      ],
      "fragments": [
        {
          "id": "domain-contract",
          "path": "src/domain.ts",
          "ranges": [
            {
              "old": {"start": 10, "lines": 4},
              "new": {"start": 10, "lines": 7}
            },
            {
              "old": {"start": 80, "lines": 2},
              "new": {"start": 83, "lines": 4}
            }
          ],
          "description": "Defines the domain contract and wires its validation."
        }
      ]
    }
  ]
}
```

Line numbers are one-based and `lines` must be positive. Omit `old` for a pure addition and omit `new` for a pure deletion. Multiple ranges let one semantic fragment select discontiguous edits. Set `file_metadata: true` on the fragment that owns a rename, mode, binary, file-creation, or file-deletion metadata change.

Validation compares the ranges with the current `base_sha..head_sha` diff. Every added line, deleted line, and file metadata change must be selected exactly once. Unchanged lines may fall inside a range and do not affect coverage.

## Draft operations

Draft operations are atomic and can be read from a file or standard input. Fragment IDs are local handles inside the draft and final file; ranges remain the fragment identity and source of truth.

```json
{
  "operations": [
    {
      "op": "merge_fragments",
      "members": ["F-candidate-1", "F-candidate-2"],
      "fragment": {
        "id": "domain-contract",
        "description": "Defines the domain contract and connects its validation."
      }
    },
    {
      "op": "upsert_group",
      "group_id": "domain-change",
      "title": "Introduce domain change",
      "summary": "Introduces the shared domain boundary.",
      "order": 1
    },
    {"op":"assign_fragments","group_id":"domain-change","members":["domain-contract"]},
    {"op":"set_file_categories","group_id":"domain-change","categories":{"src/domain.ts":"logic"}}
  ]
}
```

`merge_fragments` derives one multi-range fragment from one or more same-path suggestions or authored fragments. Available explicit definition operations are `add_fragment`, `update_fragment`, and `delete_fragments`. Group membership operations are `assign_fragments`, `move_fragments`, and `unassign_fragments`.

A fragment should be independently understandable, not merely contiguous. Do not create standalone fragments for closing delimiters, punctuation-only edits, dangling structural counterparts, or imports that only support a neighboring change. Attach those ranges to the fragment that owns the complete construct or behavior.
