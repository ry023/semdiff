# semdiff

> **Draft notice:** This repository is still a draft. The documentation was generated with lightweight AI assistance and may be difficult to read or contain inaccuracies.

`semdiff` turns a pull request diff into a review organized around what changed and why.

## Why this?

The code is finished. The pull request is open. Then the review starts:

- One feature is spread across a handler, a shared helper, tests, and configuration.
- A single diff hunk contains a behavior change, a refactor, and some formatting.
- The commit history explains how the work happened, but not necessarily the order in which it should be reviewed.
- Before asking whether the change is correct, the reviewer has to figure out what belongs together.

`semdiff` asks an AI agent to turn that diff into a review story: group related edits, explain the purpose of each group, and present the result in a useful order. It checks that the story still accounts for every changed line and file-metadata change. Git history and the implementation remain unchanged.

## Quick Start

Quick Start is intentionally centered on the human workflow. Install the CLI and skills, then let the skills drive the lower-level commands described later in this README.

### 1. Install the CLI and skills

Clone the repository, install the CLI, and copy the two bundled skills into your agent's skills directory. For Codex:

```sh
git clone https://github.com/ry023/semdiff.git
cd semdiff
go install .

mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/semantic-grouping skills/answer-semdiff \
  "${CODEX_HOME:-$HOME/.codex}/skills/"
```

Restart the agent after installing the skills so it can discover them. The `semdiff` executable must also be on the agent's `PATH`.

### 2. Ask an agent to organize the change

Open your AI agent in the Git repository you want to review and ask:

```text
/semantic-grouping
```

The agent determines the review range, inspects the commits and changed code, creates semantic Groups and Fragments, and validates the result. The finalized review is stored locally under `.semdiff/reviews/`; it does not rewrite commits or modify Git history.

To review a specific range, include it with the skill invocation:

```text
/semantic-grouping main..HEAD
```

### 3. Open the review

When the agent finishes, start the interactive viewer from the same repository:

```sh
semdiff view
```

Open `http://127.0.0.1:7363` to read the change in semantic order instead of commit or file order.

### 4. Ask questions about the review

Keep the viewer running. In another agent session, ask:

```text
/answer-semdiff
```

The agent enters answer mode and waits. You can then ask questions on a Group or Fragment in the viewer, ask follow-up questions in the same thread, and inspect the answers alongside the change. Select **End answer mode** in the viewer when you are done.

## Concepts

### What semdiff does

`semdiff` adds a semantic review layer on top of a fixed Git `base..head` range. Git provides the facts—the commits, changed lines, and file metadata—while the review layer reorganizes those facts around the concerns a reviewer needs to understand:

```text
Git range → change map → semantic Fragments → ordered Groups → review viewer
```

The tool expects an AI agent to use evidence from the commit history and code to:

- identify the smallest independently reviewable changes;
- combine related changes, including changes in different files, into semantic Groups;
- describe what each change does and why when the evidence supports it;
- set review order, file categories, Group importance, and Fragment review level; and
- produce a coverage-complete review in which every changed line and file-metadata change is selected exactly once.

The agent supplies semantic judgment. The CLI supplies deterministic facts, persists resumable draft state, and validates coverage. `semdiff` does not rewrite commits, edit the implementation, or treat Git hunk boundaries as the final review structure. The result is a derived `groups.json` review artifact; Git history remains unchanged and is still the source for change facts.

### Fragment

A Fragment is the smallest change that a reviewer can understand and describe independently. It is defined by one file path and one or more old/new line ranges (or ownership of file metadata such as a rename or binary change).

Fragments are semantic units, not Git hunks:

- One Git hunk may contain several Fragments when it mixes independently reviewable responsibilities.
- Several separated ranges may form one Fragment when they implement one responsibility.
- A new file may contain multiple Fragments; a file boundary does not imply a Fragment boundary.
- Imports, delimiters, punctuation, and other structural lines belong to the Fragment that needs them. They should not become standalone Fragments when they have no independent review meaning.

Unchanged lines inside a range provide context, but only added lines, deleted lines, and file metadata changes count toward coverage. Every authored Fragment belongs to exactly one Group in the finalized review.

### Group

A Group collects Fragments that together explain one review concern, even when they span multiple files. A Group has a title, a narrative summary, an order in which it should be reviewed, and an `importance` relative to the pull request:

- `core` is why the pull request exists;
- `supporting` completes, adapts, explains, or verifies the core change; and
- `side` is a separately meaningful change bundled into the same pull request.

Each Group also has ordered review steps. A step gives a reader the next question or prerequisite to understand, a short connection to the next stage, and the Fragments to inspect. The guided viewer follows these steps; the Files view keeps file and source order for verification.

## Using semdiff with an AI agent

The two bundled skills serve different stages of a review:

- `semantic-grouping` creates or updates a coverage-complete semantic review. Use it after implementing a change, or whenever the current branch needs to be reorganized for review.
- `answer-semdiff` answers questions attached to Groups and Fragments in the interactive viewer. It keeps waiting until you end answer mode; it does not edit the code or review artifact.

You can give the agent additional review intent in ordinary language. For example:

> Group `main..HEAD` with special attention to the public API and migration path.

> Revisit the semantic grouping after my latest changes and update the review.

> Answer the pending semdiff questions using the implementation and commit history as evidence.

The agent should invoke the skills rather than requiring you to choose fragment ranges, write operation JSON, or coordinate question sessions yourself. Those CLI interfaces remain available for automation, debugging, and integrations.

## CLI Reference

The CLI is the deterministic layer used by the skills. The complete command reference is maintained separately:

[Read the CLI Reference →](CLI_REFERENCE.md)

## Sharing reviews

With no range, `grouping init` uses the current pull request's base branch when `gh` can identify one, otherwise the Git remote's default branch. It compares the merge base with `HEAD`; pass `<base>..<head>` to override this. With no output argument, `grouping finalize` writes to the Git-ignored `.semdiff/reviews/<base-sha>...<head-sha>/groups.json`. An explicit groups file remains supported. `show`, `validate`, `questions`, and `publish` locate the finalized file from the current grouping draft when it is omitted.

With no groups file or `--draft`, `semdiff view` independently infers the current range using the same logic as `grouping init`. It opens an exact finalized review when one exists. Otherwise it walks the current head's first-parent history and opens the nearest review with the same merge-base. The viewer marks that snapshot as behind HEAD and lists the unreviewed commits and changed paths separately; it never applies old Fragment ranges to the current diff. Use `--exact` to reject this fallback, or pass a groups file or `--draft <path>` to select a specific snapshot.

`semdiff reviews resolve --json` exposes the same current-range selection for scripts and skills. Use its `groups_path` with `semdiff grouping init --from <groups-path> --force` to seed a new current-range draft from the nearest compatible review. The source must have the same base SHA and a head on the current first-parent history; the new draft always recomputes Git facts and must be validated again before finalization.

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

Use `semdiff view --html review.html` to export a self-contained, read-only viewer. Add `--include-answers` to include a snapshot of answered question turns. Static exports do not provide Ask, follow-up, or answer-session controls.

`fragments` emits Git-derived zero-context ranges as initial suggestions. They are not canonical fragment boundaries. `grouping init` stores suggestions separately from authored fragments in `.semdiff/grouping-draft.json`, so each Git span does not become an assignment obligation. Inspect them with `grouping inspect --suggestions`, then use `merge_fragments`, `add_fragment`, `update_fragment`, and `delete_fragments` to compose semantic fragments before finalizing.

`classify` suggests the standard categories `logic`, `component`, `config`, `implementation`, `test`, `docs`, and `unknown` from paths. Documentation extensions such as Markdown and reStructuredText, conventional documentation filenames such as `README` and `CHANGELOG`, and files under documentation directories such as `docs/` and `guides/` are classified as `docs`.

This workflow uses draft schema version 4 and final `groups.json` schema version 3. Re-run `grouping init --force` to replace an older draft.

`groups.json` is the source of truth. Every fragment contains its path, one or more old/new ranges, and its semantic description:

```json
{
  "version": 3,
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
      "review_steps": [
        {"id": "contract", "title": "Establish the contract", "summary": "Read the contract before its dependent behavior.", "fragment_ids": ["domain-contract"]}
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
