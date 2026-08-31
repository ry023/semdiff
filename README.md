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
semdiff grouping init --json
semdiff grouping inspect --suggestions --json
semdiff grouping status --json
semdiff grouping inspect --unassigned --json
semdiff grouping apply decisions.json --json
semdiff grouping finalize --json
semdiff questions session start --json
semdiff questions wait --session S-... --json
semdiff questions answer Q-... --stdin
semdiff show --draft .semdiff/grouping-draft.json F-0123456789ab --json
semdiff show F-0123456789ab --json
semdiff validate
semdiff view --addr 127.0.0.1:7363
semdiff publish
semdiff reviews view --addr 127.0.0.1:7363
```

## Sharing reviews

With no range, `grouping init` uses the current pull request's base branch when `gh` can identify one, otherwise the Git remote's default branch. It compares the merge base with `HEAD`; pass `<base>..<head>` to override this. With no output argument, `grouping finalize` writes to the Git-ignored `.semdiff/reviews/<base-sha>...<head-sha>/groups.json`. An explicit groups file remains supported. Commands that consume a finalized groups file—`show`, `validate`, `questions`, `view`, and `publish`—locate this file from the current grouping draft when it is omitted.

`publish` stores only the review artifact, `groups.json`, on a Git artifact branch. Question threads remain local and are never uploaded. With no configuration, semdiff uses the current repository's `origin` and the `semdiff/reviews` branch; the first publish creates that branch as an orphan branch.

Artifacts are stored at `<base-sha>...<head-sha>/groups.json` using full SHAs. `semdiff reviews view` lists the branch and its Refresh button fetches updates.

An optional repository-shared `semdiff.yaml` can specify the store:

```yaml
review_store:
  remote: origin
  branch: semdiff/reviews
```

For a separate artifact repository, place its URL in the Git-ignored local `.semdiff/config.local.yaml`:

```yaml
review_store:
  repository: git@github.com:org/semdiff-reviews.git
  branch: semdiff/reviews
```

CLI flags override local configuration, which overrides `semdiff.yaml`, which overrides the defaults. `remote` and `repository` are mutually exclusive.

The viewer can attach question threads to a semantic Group or Fragment. A follow-up continues with the answered turns from that thread, while a new Ask starts an independent context. Keep `semdiff view` running, then ask an AI agent to start the `answer-semdiff` skill. The skill starts an answer session, claims pending turns one at a time, answers them, and waits again. Ending answer mode in the viewer stops the session and lets the skill finish. Outside answer mode, Ask buttons are hidden and the viewer shows instructions for starting the skill. Thread state lives under `.semdiff/questions/`; the current answer session lives separately under `.semdiff/sessions/`.

`fragments` emits Git-derived zero-context ranges as initial suggestions. They are not canonical fragment boundaries. `grouping init` stores suggestions separately from authored fragments in `.semdiff/grouping-draft.json`, so each Git span does not become an assignment obligation. Inspect them with `grouping inspect --suggestions`, then use `merge_fragments`, `add_fragment`, `update_fragment`, and `delete_fragments` to compose semantic fragments before finalizing.

`classify` suggests the standard categories `logic`, `component`, `config`, `implementation`, `test`, `docs`, and `unknown` from paths. Documentation extensions such as Markdown and reStructuredText, conventional documentation filenames such as `README` and `CHANGELOG`, and files under documentation directories such as `docs/` and `guides/` are classified as `docs`.

This workflow uses draft schema version 3 and final `groups.json` schema version 2. Re-run `grouping init --force` to replace an older draft.

`groups.json` is the source of truth. Every fragment contains its path, one or more old/new ranges, and its semantic description:

```json
{
  "version": 2,
  "base_sha": "<full base SHA>",
  "head_sha": "<full head SHA>",
  "groups": [
    {
      "id": "domain-change",
      "title": "Introduce domain change",
      "summary": "Introduces the shared domain boundary and its validation.",
      "importance": "core",
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
          "description": "Defines the domain contract and wires its validation.",
          "review_level": "careful"
        }
      ]
    }
  ]
}
```

Line numbers are one-based and `lines` must be positive. Omit `old` for a pure addition and omit `new` for a pure deletion. Multiple ranges let one semantic fragment select discontiguous edits. Set `file_metadata: true` on the fragment that owns a rename, mode, binary, file-creation, or file-deletion metadata change.

Every Group has an `importance` of `core`, `supporting`, or `side`, describing its place in the PR as a whole. `core` is why the PR exists, `supporting` completes the core change, and `side` is a separately meaningful change bundled into the same PR. Every authored Fragment has a `review_level` of `careful`, `normal`, or `skim`, telling the reviewer how closely to read that local change. Omitted Fragment review levels default to `normal` while drafting and are written explicitly to the final file.

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
        "description": "Defines the domain contract and connects its validation.",
        "review_level": "careful"
      }
    },
    {
      "op": "upsert_group",
      "group_id": "domain-change",
      "title": "Introduce domain change",
      "summary": "Introduces the shared domain boundary.",
      "importance": "core",
      "order": 1
    },
    {"op":"assign_fragments","group_id":"domain-change","members":["domain-contract"]},
    {"op":"set_file_categories","group_id":"domain-change","categories":{"src/domain.ts":"logic"}}
  ]
}
```

`merge_fragments` derives one multi-range fragment from one or more same-path suggestions or authored fragments. Available explicit definition operations are `add_fragment`, `update_fragment`, and `delete_fragments`. Group membership operations are `assign_fragments`, `move_fragments`, and `unassign_fragments`.

A fragment should be independently understandable, not merely contiguous. Do not create standalone fragments for closing delimiters, punctuation-only edits, dangling structural counterparts, or imports that only support a neighboring change. Attach those ranges to the fragment that owns the complete construct or behavior.
