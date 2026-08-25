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
5. Write version 1 JSON with exact resolved `base_sha` and `head_sha`, unique group IDs, concise titles and summaries, optional numeric review order, and described `fragments` references.
6. Run `semdiff validate groups.json --json`. Do not report completion until it succeeds. Investigate every unknown, duplicate, or unassigned fragment; use a fallback group only after reasonable inspection.

The required shape is:

```json
{"version":1,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"domain-change","title":"Introduce domain change","summary":"Explains the review concern.","order":1,"fragments":[{"id":"F-...","description":"Adds the domain type used by the new workflow."}]}]}
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
