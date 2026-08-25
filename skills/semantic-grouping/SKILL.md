---
name: semantic-grouping
description: Group a Git commit range into review-oriented semantic changes using the semdiff CLI. Use when creating or updating a coverage-complete groups.json for this repository; do not use it to rewrite Git history.
---

# Semantic Grouping

Create `groups.json` as a derived review layer. Preserve commits and repository history. Let the CLI extract and validate facts; use semantic judgment only for grouping, titles, summaries, and review order.

## Workflow

1. Run `semdiff commits <base>..<head> --json` to understand the change narrative.
2. Run `semdiff fragments <base>..<head> --json` to get the lightweight inventory. Do not load the complete diff up front.
3. Infer tentative concerns from commit subjects, paths, and ranges. Inspect only relevant fragments with `semdiff show <id> --json`. The `fragments` command must run first because `show` reads its latest inventory.
4. Create a small number of cohesive groups. Assign each fragment one primary membership even when it relates to several concerns. For every fragment, write a concise semantic description of what changed and, when the evidence supports it, why the change was made. Use a clearly named fallback such as `mechanical-changes` or `unclassified` when evidence is insufficient.
5. Write version 1 JSON with exact resolved `base_sha` and `head_sha`, unique group IDs, concise titles, multi-sentence review summaries, optional numeric review order, and described `fragments` references.
6. Run `semdiff validate groups.json --json`. Do not report completion until it succeeds. Investigate every unknown, duplicate, or unassigned fragment; use a fallback group only after reasonable inspection.

The required shape is:

```json
{"version":1,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"domain-change","title":"Introduce domain change","summary":"The previous implementation lacked a single place for this responsibility, which made related behavior harder to keep consistent. The change introduces the shared boundary and adds the checks needed to preserve its contract.","order":1,"fragments":[{"id":"F-...","description":"Adds the domain type used by the new workflow."}]}]}
```

Every extracted fragment must occur exactly once across all groups and every `fragments` entry must have a non-empty `description`. Legacy files using `fragment_ids` remain readable, but new output must use `fragments`.

## Fragment descriptions

Describe the changed behavior, responsibility, contract, or test coverage—not the file operation. Do not repeat the file name or path, and do not lead with bookkeeping such as “updated,” “added,” “deleted,” “new file,” or “first half”; the data structure and viewer already show that context.

When the diff, surrounding code, commit narrative, or group intent provides evidence, include the reason or purpose of the change. Useful reasons include enabling a workflow, removing duplicated responsibility, preserving an invariant, adapting callers to a new contract, or preventing a regression. Do not invent rationale that the available evidence does not support; when the reason is unclear, state only the concrete semantic change.

Prefer descriptions such as:

- `Removes the duplicated rendering path so presentation is handled by the shared component.`
- `Adds coverage for the supported operations to guard their expected behavior against regressions.`
- `Routes creation through the centralized command interface so it follows the same execution path as related actions.`

Avoid descriptions such as:

- `ファイルを更新。古いコードを削除。`
- `テストファイルを新規追加。関連するテストを追加。`
- `ファイル前半部分を更新。新しい依存関係を追加。`

## Group summaries

A group `summary` is a review narrative, not a one-line restatement of its fragments. Write multiple sentences (normally 2–4) so a reviewer can understand the motivation and shape of the change without opening every file. A useful summary generally covers:

1. The background or existing situation, when it can be established from the commit history or surrounding code.
2. The problem, limitation, or risk that motivated the work.
3. The main approach taken and the important responsibilities or invariants it introduces.
4. The resulting behavior, tests, or review implication when the evidence supports it.

Use `semdiff commits` as the primary source for background: consider commit subjects, commit bodies, chronology, and which files each commit touched. Use fragment evidence to verify what was actually implemented and to connect the narrative to the grouped changes. Commit history is the current boundary of available context; do not invent product requirements, incidents, user reports, or design decisions that are not supported by commits or code. If the motivation is unclear, say what limitation the diff addresses only when that is directly observable, and keep uncertain interpretation out of the summary.

The summary should explain the relationship among the fragments rather than enumerate filenames or repeat each fragment description. Put the main parts on separate lines in the JSON string: use one line for the background or problem, one for the approach, and optionally one for the result or review implication. Encode those line breaks as `\n` (use `\n\n` only when a blank line is useful). Keep explicit causal links such as “because,” “so that,” or “which allows.” Do not compress the summary into one line merely to make the JSON shorter.

Prefer a summary shaped like:

`The previous implementation spread state transitions across direct mutations, which made related updates difficult to keep consistent.\nThe change introduces a centralized command boundary and retains inactive state needed when switching modes, so callers can apply the same transition rules.\nThe associated tests exercise the shared contract and give reviewers one place to inspect the invariants that protect it.`

Avoid a summary shaped like:

`Replaces the old implementation with a command layer, adds caching, and adds tests.`
