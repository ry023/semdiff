# semdiff design

## Goal

`semdiff` provides a semantic review layer over a fixed Git `base..head` range. It does not rewrite commits. Review authors define meaningful fragments and collect them into ordered semantic groups.

## Core distinction

Git hunks and semantic fragments are different concepts:

- Git supplies a mechanical change map: added lines, deleted lines, and file metadata changes.
- A fragment supplies semantic ownership: a path and one or more ranges selecting records from that change map.
- A semantic group connects fragments across files into one review concern.

Git zero-context hunks are stored as draft suggestions, but they never populate the authored fragment collection or determine final boundaries. A new file can be split into several fragments, and discontiguous edits in one file can be represented by one fragment.

## Data model

The only supported `groups.json` schema is version 1:

```json
{
  "version": 1,
  "base_sha": "<full SHA>",
  "head_sha": "<full SHA>",
  "groups": [
    {
      "id": "command-boundary",
      "title": "Centralize command execution",
      "summary": "Explains the motivation, approach, and result.",
      "order": 1,
      "file_categories": [
        {"path": "src/commands.ts", "category": "logic"}
      ],
      "fragments": [
        {
          "id": "command-contract",
          "path": "src/commands.ts",
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
          "description": "Defines the shared command contract and routes execution through it."
        }
      ]
    }
  ]
}
```

### Range semantics

`start` is a one-based file line number and `lines` is a positive count. A range may contain:

- both `old` and `new` for a replacement;
- only `new` for an addition;
- only `old` for a deletion.

The old and new sides select changed lines independently. Unchanged lines inside a declared range provide semantic context but are not owned and do not affect coverage. A fragment may contain multiple ranges, but remains local to one path.

`file_metadata: true` selects the path's non-line change, such as creation, deletion, rename, mode, symlink, or binary metadata. It can coexist with ranges on the same fragment.

### Fragment IDs

A fragment ID is unique within one draft or groups file. It is a stable editing and lookup handle, not a hash-derived identity. The fragment's path and ranges are its actual definition. IDs do not need to persist when `base_sha` or `head_sha` changes.

## Validation

Validation resolves the recorded SHAs, computes `git diff --unified=0`, and treats every added line, deleted line, and file metadata change as a coverage atom.

A valid groups file satisfies all of the following:

- SHAs match the computed change map.
- Group and fragment IDs are unique and non-empty.
- Every group has a title and summary.
- Every fragment has a path, description, and at least one range or `file_metadata` selection.
- Every range has positive coordinates.
- Every coverage atom is selected by exactly one fragment.
- Every path used by a group has exactly one file category in that group.

Ranges that select no changed lines are rejected. Gaps and overlaps are reported at the changed-line coordinate.

## Patch materialization

Patches are derived data and are not stored in `groups.json`. `show` and `view` recompute the Git change map and select diff rows using each fragment's ranges. Discontiguous ranges produce multiple zero-context hunk sections in the materialized patch. The viewer loads file contents separately to offer expandable context.

This makes review output reproducible from:

```text
base SHA + head SHA + path + ranges + metadata ownership
```

## CLI workflow

```text
commits <base>..<head>             inspect history
fragments <base>..<head>           inspect Git-derived navigation suggestions
classify <base>..<head>            inspect path-based category suggestions
grouping init <base>..<head>       create a resumable draft
grouping inspect --suggestions     inspect Git-derived navigation candidates
grouping inspect/status            inspect authored draft work
grouping apply <operations>        edit definitions and decisions atomically
grouping finalize <groups-file>    validate coverage and write groups.json
show --draft <path> <fragment-id>  materialize an editable draft fragment
show <groups-file> <fragment-id>   materialize a finalized fragment
validate <groups-file>             recompute and validate coverage
view <groups-file>                 render semantic groups
```

`show` always names its groups file or draft, so it does not depend on mutable “latest inventory” state.

`classify` uses the standard vocabulary `logic`, `component`, `config`, `implementation`, `test`, `docs`, and `unknown`. The path-only heuristic recognizes documentation extensions, conventional names such as `README` and `CHANGELOG`, and documentation directories such as `docs/` and `guides/`. Categories remain free-form in authored groups.

## Draft model

Draft schema version 2 separates suggestions from authored fragments. Version 1 drafts must be recreated; the final `groups.json` schema remains version 1. The draft contains:

- resolved base/head SHAs;
- a lightweight mechanical change map without patches;
- immutable Git-derived suggestions used only for navigation and composition;
- authored fragment definitions, initially empty;
- groups whose `members` refer to local fragment IDs;
- category suggestions and revision metadata.

`merge_fragments` composes one authored multi-range fragment from same-path suggestions or authored fragments. Supported explicit definition operations are `add_fragment`, `update_fragment`, and `delete_fragments`. Supported membership operations are `assign_fragments`, `move_fragments`, and `unassign_fragments`. Group and category operations remain independent so partial work is resumable. Each apply batch is atomic and can use `expected_revision` for optimistic concurrency.

Suggestions are deliberately excluded from assignment and description counts. This prevents Git hunk granularity from becoming a completion checklist. Semantic fragments should be independently reviewable; delimiter-only, punctuation-only, and other structurally dependent ranges belong to the fragment that owns the complete construct.

Finalization refreshes the Git change map before writing output. A stale or incomplete draft therefore cannot silently produce a groups file with incorrect coverage.

## File metadata and binary changes

The diff parser emits metadata coverage independently from textual hunks when Git reports file creation/deletion, mode changes, renames, or binary changes. Draft initialization attaches that ownership to the first suggestion for the path. Composing that suggestion with `merge_fragments` carries metadata ownership to the authored fragment; an explicit definition can instead select it with `file_metadata: true`. Metadata-only files receive a suggestion with `file_metadata: true` and no ranges.
