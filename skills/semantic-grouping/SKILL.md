---
name: semantic-grouping
description: Group a Git commit range into review-oriented semantic changes using the semdiff CLI. Use when creating or updating a coverage-complete groups.json for this repository; do not use it to rewrite Git history.
---

# Semantic Grouping

Create `groups.json` as a derived review layer. Preserve commits and repository history. Let the CLI extract, persist, and validate facts; use semantic judgment only for grouping, titles, summaries, descriptions, and review order. Build the result through a resumable grouping draft instead of writing the final JSON directly.

## Workflow

1. Run `semdiff grouping init <base>..<head> --json` to create a resumable draft containing the exact range, Git-derived suggestions, and mechanical category suggestions. Suggestions are navigation aids, not fragments that must each be assigned.
2. Run `semdiff commits <base>..<head> --json` to understand the change narrative. Do not load the complete diff up front.
3. Run `semdiff grouping inspect --suggestions --json` and review suggestions by file and neighboring code. Use `semdiff show --draft .semdiff/grouping-draft.json <id> --json` for the relevant candidates.
4. Compose authored fragments from the suggestions with `merge_fragments`: one `member` promotes a semantically complete suggestion, while several same-path `members` become one multi-range fragment. Use `add_fragment`, `update_fragment`, and `delete_fragments` only when the intended definition needs explicit ranges. Only authored fragments appear in status and require assignment.
5. Create a small number of cohesive groups and apply the decisions in batches with `semdiff grouping apply <operations-file|-> --json`. Assign each fragment one primary membership even when it relates to several concerns. Set each group's `order` so prerequisites appear before groups that depend on them. Use a clearly named fallback such as `mechanical-changes` or `unclassified` when evidence is insufficient.
6. For every fragment, write a concise semantic description of what changed and, when the evidence supports it, why the change was made. For every file referenced by a group's fragments, set exactly one `file_categories` entry. Start from `classify` output, then confirm or revise it using commit intent, path semantics, and relevant Fragment content.
7. Repeat `status`, `inspect`, and `apply` as needed. Drafts are intentionally allowed to be incomplete; do not stop merely because one batch has unassigned fragments.
8. Before finalizing, review each file for over-fragmentation. Merge fragments whose meaning cannot be explained independently, especially delimiter-only or syntax-only fragments and fragments that merely complete a neighboring construct.
9. Run `semdiff grouping finalize groups.json --json`. Finalize succeeds only when every authored fragment is assigned and described, every Group has a complete summary and file categories, and every changed line and metadata change is selected exactly once.

The required shape is:

```json
{"version":1,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"domain-change","title":"Introduce domain change","summary":"The previous implementation lacked a single place for this responsibility, which made related behavior harder to keep consistent. The change introduces the shared boundary and adds the checks needed to preserve its contract.","order":1,"file_categories":[{"path":"src/domain.ts","category":"logic"}],"fragments":[{"id":"domain-contract","path":"src/domain.ts","ranges":[{"old":{"start":10,"lines":4},"new":{"start":10,"lines":7}},{"old":{"start":80,"lines":2},"new":{"start":83,"lines":4}}],"description":"Adds the domain type and connects its validation at the call site."}]}]}
```

Every fragment must occur exactly once across all groups and must contain `id`, `path`, at least one `ranges` entry (or `file_metadata: true`), and a non-empty `description`. Every changed old/new line and file metadata change must be selected exactly once. Every file referenced by a Group must occur exactly once in that Group's `file_categories`.

## Grouping drafts

`semdiff grouping init` creates a draft schema version 2 file at `.semdiff/grouping-draft.json` by default. Recreate a version 1 draft with `grouping init --force`; do not continue it because version 1 reified Git spans as authored fragments. The draft is the working state; `groups.json` is only produced by `grouping finalize`. Apply operations can be repeated and can add, revise, move, or remove decisions. Use `--draft <path>` when maintaining more than one draft.

An apply request contains a batch of operations. For example:

```json
{
  "operations": [
    {"op":"upsert_group","group_id":"domain-change","title":"Introduce domain change","summary":"Explains the motivation and approach.","order":1},
    {"op":"merge_fragments","members":["F-candidate-1","F-candidate-2"],"fragment":{"id":"domain-contract","description":"Defines the shared domain contract and connects its validation."}},
    {"op":"assign_fragments","group_id":"domain-change","members":["domain-contract"]},
    {"op":"set_file_categories","group_id":"domain-change","categories":{"src/domain.ts":"logic"}}
  ]
}
```

`merge_fragments` accepts one or more same-path suggestion or authored-fragment IDs. It derives the path, concatenates their ranges, carries file metadata ownership, removes authored sources, and preserves a shared Group assignment. Provide explicit ranges in the resulting `fragment` only when the derived union is not the intended selection.

Use `move_fragments` when a fragment already belongs to another Group, and use `status` to identify only the remaining authored work. `add_fragment`, `update_fragment`, and `delete_fragments` edit definitions; fragment IDs are local handles while path/ranges are the source of truth. `apply` is atomic: an invalid operation leaves the previous draft unchanged. Do not edit the draft file by hand.

## Semantic fragment boundaries

A fragment should be the smallest change that a reviewer can understand and describe independently—not the smallest contiguous diff span. One range is sufficient when it contains a complete semantic change; use multiple ranges whenever separated edits implement one responsibility.

Treat file boundaries and fragment boundaries independently. In particular, do not keep a new file as one fragment merely because Git presents the entire file as one addition. When distinct line ranges in that file implement independently reviewable responsibilities that belong to different Groups, create separate fragments and assign each one to its owning Group. Include shared declarations, imports, delimiters, and other structural lines with the responsibility that makes them necessary; do not manufacture a separate fragment for scaffolding that has no independent meaning.

Never leave these as standalone fragments when they only complete another change:

- a closing brace, bracket, parenthesis, comma, semicolon, or other punctuation-only edit;
- a dangling `else`, `catch`, JSX closing tag, or similar structural counterpart;
- an import used only by a neighboring implementation fragment;
- one half of a declaration/body, caller/callee adaptation, or setup/assertion pair that has no independent review meaning.

Attach such ranges to the fragment that owns the construct or behavior. A useful test is the description: if it can only say “closes,” “adjusts syntax,” “adds an import,” or refer vaguely to another fragment, merge it. Do not merge merely because ranges are close; keep independently reviewable behavior, tests, configuration, and refactors separate even when Git placed them in one hunk.

The `classify` command only uses file paths, names, extensions, and directory structure. It intentionally has no confidence score or semantic rationale. Treat its output as a draft: the final category should describe the file's role in that Group, based on the commit narrative and relevant code when the mechanical guess is insufficient.

The default category vocabulary is `implementation` for general source code, `test` for tests, `component` for UI components, `logic` for UI-independent logic, `config` for configuration and dependency metadata, `docs` for documentation, and `unknown` when the path does not establish a useful role. These are conventions rather than an enum; use a more precise free-form category when needed.

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

## Group review order

Choose Group `order` as a reviewer-oriented dependency order, not alphabetical order, file order, or the order in which fragments were discovered. If Group B relies on a contract, type, schema, helper, migration, or behavior introduced by Group A, place A before B. Apply this transitively across the full set of Groups: establish prerequisites first, then dependent behavior and integrations, then follow-up validation or documentation when those are genuinely separate concerns.

Infer dependencies from imports and call sites, type and schema usage, configuration consumers, commit chronology, and the semantic descriptions—not merely from directory layout. Keep tests in the same Group as the behavior they verify unless the tests form an independently reviewable concern; when they do form a separate Group, place that Group after the behavior it verifies.

When Groups are independent, order them to minimize context switching and make the change read as a coherent narrative—for example, shared foundations before feature-specific uses and core behavior before peripheral adaptation. Do not invent a dependency solely to force a tidy sequence. Before finalizing, scan the ordered summaries from first to last and revise `order` if a Group requires knowledge that is introduced only later.

## Group summaries

A group `summary` is a review narrative, not a one-line restatement of its fragments. Write multiple sentences (normally 2–4) so a reviewer can understand the motivation and shape of the change without opening every file. A useful summary generally covers:

1. The background or existing situation, when it can be established from the commit history or surrounding code.
2. The problem, limitation, or risk that motivated the work.
3. The main approach taken and the important responsibilities or invariants it introduces.
4. The resulting behavior, tests, or review implication when the evidence supports it.

Use `semdiff commits` as the primary source for background: consider commit subjects, commit bodies, chronology, and which files each commit touched. Use fragment evidence to verify what was actually implemented and to connect the narrative to the grouped changes. Commit history is the current boundary of available context; do not invent product requirements, incidents, user reports, or design decisions that are not supported by commits or code. If the motivation is unclear, say what limitation the diff addresses only when that is directly observable, and keep uncertain interpretation out of the summary.

The summary should explain the relationship among the fragments rather than enumerate filenames or repeat each fragment description. Summary values support Markdown in the viewer. Put the main parts on separate lines in the JSON string: use one line for the background or problem, one for the approach, and optionally one for the result or review implication. Encode those line breaks as `\n` (use `\n\n` for separate Markdown paragraphs when useful). You may use ordinary Markdown such as paragraphs, lists, emphasis, and inline code; do not depend on raw HTML. Keep explicit causal links such as “because,” “so that,” or “which allows.” Do not compress the summary into one line merely to make the JSON shorter.

Prefer a summary shaped like:

```markdown
The previous implementation spread state transitions across direct mutations, which made related updates difficult to keep consistent.
The change introduces a centralized command boundary and retains inactive state needed when switching modes, so callers can apply the same transition rules.
The associated tests exercise the shared contract and give reviewers one place to inspect the invariants that protect it.
```

Avoid a summary shaped like:

`Replaces the old implementation with a command layer, adds caching, and adds tests.`
