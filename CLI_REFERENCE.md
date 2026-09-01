# semdiff CLI Reference

This is the deterministic CLI layer used by the bundled skills. Use it directly when building an integration, inspecting intermediate state, troubleshooting an agent run, or operating semdiff without an agent.

See the [README](README.md) for the human and AI-agent workflow, installation, and review sharing.

## End-to-end draft workflow

The primitive workflow behind `semantic-grouping` is:

```sh
semdiff grouping init main..HEAD
semdiff grouping inspect --suggestions --json
semdiff grouping apply decisions.json --json
semdiff grouping status --json
semdiff grouping finalize --json
semdiff validate
```

`grouping init` writes `.semdiff/grouping-draft.json`. An operations file contains atomic changes such as `merge_fragments`, `upsert_group`, and `assign_fragments`; see [Draft operations](#draft-operations) for the format. `grouping finalize` writes `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` by default.

If the range is omitted, semdiff uses the current pull request's base branch when `gh` can identify it, or the Git remote's default branch otherwise.

Most inspection and mutation commands accept `--json`, which is useful when semdiff is driven by an AI agent or another program. Flags may appear before or after positional arguments.

## Understand a change range

Use these read-only commands when exploring a range before or during grouping:

```sh
semdiff commits main..HEAD
semdiff fragments main..HEAD
semdiff classify main..HEAD
```

- `commits` lists the commits in the range, including author and changed-file count.
- `fragments` emits zero-context, Git-derived ranges that can be used as starting points for semantic fragments. These suggestions are not final fragment boundaries.
- `classify` suggests a category for each changed path, such as `logic`, `test`, `docs`, or `config`.

Add `--json` to any of these commands for structured output.

## Build semantic groups

Grouping is an iterative draft workflow:

```sh
semdiff grouping init [main..HEAD] [--from path/to/groups.json]
semdiff grouping inspect --suggestions
semdiff grouping apply decisions.json
semdiff grouping status
semdiff grouping finalize
```

- `grouping init` records the base and head commits, Git-derived suggestions, and path classifications in `.semdiff/grouping-draft.json`. `--from <groups-file>` seeds the new draft with a validated, same-base review whose head is on the target's first-parent history; the target change map and suggestions are still recomputed. Use `--force` to replace an existing draft or `--draft <path>` to use another location.
- `grouping inspect` reads a focused part of the draft. Choose exactly one of `--suggestions`, `--unassigned`, `--group <id>`, or `--fragment <id>`.
- `grouping apply` atomically applies the operations in a JSON file. Pass `-` instead of a filename to read the request from standard input.
- `grouping status` reports assignment and description progress, missing review metadata, and whether the draft is ready to finalize.
- `grouping finalize` validates the draft against the current Git diff and writes the canonical `groups.json`. You may pass an explicit output path.

All grouping commands accept `--draft <path>`. See [Draft operations](#draft-operations) for the operation format and [Sharing reviews](README.md#sharing-reviews) for default artifact paths.

## Inspect and validate fragments

`show` renders the selected lines for one fragment, including discontiguous ranges. It can read either the active draft or a finalized review:

```sh
semdiff show --draft .semdiff/grouping-draft.json F-0123456789ab
semdiff show F-0123456789ab
semdiff show path/to/groups.json F-0123456789ab
```

`validate` checks a finalized review against its `base_sha..head_sha` diff. Every added line, deleted line, and metadata change must belong to exactly one fragment:

```sh
semdiff validate
semdiff validate path/to/groups.json --json
```

When no groups file is provided, both commands locate the finalized artifact from the current grouping draft.

## View or export a review

Start the interactive viewer on the default address, or choose another listen address:

```sh
semdiff view
semdiff view --addr 127.0.0.1:8080
```

Without a groups file or `--draft`, `view` infers the current pull-request range with the same logic as `grouping init`. It prefers an exact finalized review. If none exists, it opens the nearest same-base review on the current head's first-parent history and clearly lists the commits and paths that have not been semantically grouped. Use `semdiff view --exact` to require an exact review.

Use `reviews resolve` when a script or skill needs the same selection without starting the viewer. It returns `found`, `groups_path`, the current and selected SHAs, whether the match is exact, and the first-parent commit distance:

```sh
semdiff reviews resolve --json
semdiff reviews resolve --exact --json
```

To create a self-contained, read-only file instead of starting a server, use `--html`. Question answers are omitted unless explicitly included:

```sh
semdiff view --html review.html
semdiff view --html review.html --include-answers
```

An explicit `groups.json` path may be supplied to any of these forms. `--draft <path>` explicitly restores draft-based artifact selection.

## Answer reviewer questions

The interactive viewer can attach question threads to a Group or Fragment. An answer worker uses a session to wait for questions and submit answers:

```sh
semdiff questions session start --json
semdiff questions wait --session S-... --json
printf '%s\n' 'The answer text' | semdiff questions answer Q-... --stdin
```

- `questions session start` begins answer mode and returns a session ID.
- `questions wait` blocks until the session receives a question or is stopped in the viewer. Without `--session`, it waits for the next pending question directly.
- `questions answer` reads a non-empty answer from standard input and attaches it to the specified question thread.

These commands use the current finalized review by default. A groups file can be passed explicitly, and `--draft <path>` changes how the default artifact is located.

## Publish and browse reviews

Publish the current finalized review to the configured Git artifact branch:

```sh
semdiff publish
```

Then fetch and browse all reviews stored on that branch:

```sh
semdiff reviews view
semdiff reviews view --addr 127.0.0.1:8080
```

Both commands accept `--remote`, `--repository`, and `--branch` overrides. `publish` uploads only `groups.json`; local question threads are not included. See [Sharing reviews](README.md#sharing-reviews) for configuration and storage details.

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
    {"op":"set_review_steps","group_id":"domain-change","review_steps":[{"id":"contract","title":"Establish the contract","summary":"Read the contract before the dependent behavior.","fragment_ids":["domain-contract"]}]},
    {"op":"set_file_categories","group_id":"domain-change","categories":{"src/domain.ts":"logic"}}
  ]
}
```

`merge_fragments` derives one multi-range fragment from one or more same-path suggestions or authored fragments. Available explicit definition operations are `add_fragment`, `update_fragment`, and `delete_fragments`. Group membership operations are `assign_fragments`, `move_fragments`, and `unassign_fragments`.

A fragment should be independently understandable, not merely contiguous. Do not create standalone fragments for closing delimiters, punctuation-only edits, dangling structural counterparts, or imports that only support a neighboring change. Attach those ranges to the fragment that owns the complete construct or behavior.
