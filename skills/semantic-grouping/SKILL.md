---
name: semantic-grouping
description: Group a Git commit range into review-oriented semantic changes using the semdiff CLI. Use when creating or updating a coverage-complete groups.json for this repository; do not use it to rewrite Git history.
---

# Semantic Grouping

Create `groups.json` as a derived review layer. Preserve commits and repository history. Let the CLI extract, persist, and validate facts; use semantic judgment only for grouping, titles, summaries, descriptions, and review order. Build the result through a resumable grouping draft instead of writing the final JSON directly.

## Workflow

1. Run `semdiff reviews resolve --json` before creating a draft. With no range, it uses the same pull-request range logic as `grouping init`. If `found` is false, create a fresh draft with `semdiff grouping init --json`. If it finds an ancestor review (`exact: false`), create a fresh current-range draft seeded from it with `semdiff grouping init --from <groups_path> --force --json`. If it finds an exact review, reuse that artifact unless the user explicitly asks to revise its grouping.
2. A seeded draft carries forward the existing Groups, summaries, review metadata, and Fragment definitions, but its Git change map and suggestions are always recomputed for the current range. Treat every commit after `review_head_sha` and every existing Fragment on an affected path as work to re-check; do not assume old line ranges remain semantically correct. The CLI refuses a source whose Base differs or whose Head is not on the current first-parent history.
3. Run `semdiff commits <base-sha>..<head-sha> --json` with the SHAs returned by `grouping init` to understand the change narrative. For an ancestor source, also inspect `semdiff commits <review_head_sha>..<head-sha> --json` first. Do not load the complete diff up front.
4. Run `semdiff grouping inspect --suggestions --json` and review suggestions by file and neighboring code. For a seeded draft, prioritize ungrouped suggestions and Groups whose files changed after the source review. Use `semdiff show --draft .semdiff/grouping-draft.json <id> --json` for the relevant candidates.
5. Compose authored fragments from the suggestions with `merge_fragments`: one `member` promotes a semantically complete suggestion, while several same-path `members` become one multi-range fragment. Use `add_fragment`, `update_fragment`, and `delete_fragments` when the intended definition needs explicit ranges or an inherited definition must change. Only authored fragments appear in status and require assignment.
6. Create a small number of cohesive groups and apply the decisions in batches with `semdiff grouping apply <operations-file|-> --json`. Preserve an inherited Group when its concern remains intact; revise, split, merge, or move it when the new change alters that concern. Assign each fragment one primary membership even when it relates to several concerns. Assign Group `importance` using `core`, `supporting`, or `side` relative to the PR as a whole, following the removal test below. Assign Fragment `review_level` using `careful`, `normal`, or `skim` to tell the reviewer how closely to read it; use `normal` unless the fragment clearly warrants more or less attention. Set each group's `order` so prerequisites appear before groups that depend on them. Use a clearly named fallback such as `mechanical-changes` or `unclassified` when evidence is insufficient.
7. For every new or revised fragment, write a concise semantic description of what changed and, when the evidence supports it, why the change was made. For every file referenced by a Group's fragments, set exactly one `file_categories` entry. Start from `classify` output, then confirm or revise it using commit intent, path semantics, and relevant Fragment content.
8. Repeat `status`, `inspect`, and `apply` as needed. Drafts are intentionally allowed to be incomplete; do not stop merely because one batch has unassigned fragments.
9. Before finalizing, review each file for both under- and over-fragmentation. For a large suggestion or new file, inspect its top-level responsibilities and use `add_fragment` with explicit ranges when functions, types, handlers, or tests can be explained and questioned independently. Do not split merely at a file boundary, Git hunk, or test boundary. Merge fragments whose meaning cannot be explained independently, especially delimiter-only or syntax-only fragments and fragments that merely complete a neighboring construct.
10. After assigning fragments, create `review_steps` for every Group with `set_review_steps`. First list the questions a reviewer must answer in order, then map each Fragment to one Step. A Step exists only when it introduces a new prerequisite, question, or judgment; do not target a number of Steps. Its summary describes this stage's role in the change and its relationship to adjacent Steps: state what it establishes and which later behavior depends on it, or how it completes what came before. Write descriptively about the implementation; do not instruct the reviewer to “read,” “review,” or “look at” something. Adjacent Steps that answer the same question belong together. Each Group Fragment must occur exactly once across its Steps.
11. Run `semdiff grouping finalize --json`. With no explicit output path, finalize writes the result to the Git-ignored `.semdiff/reviews/<base-sha>...<head-sha>/groups.json`; use an explicit path only when the user or surrounding workflow requires one. Finalize succeeds only when every authored fragment is assigned, described, and placed in a complete review Step, every Group has a complete summary and file categories, and every changed line and metadata change is selected exactly once.

The required shape is:

```json
{"version":3,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"repository-lookup","title":"Add Repository lookup","summary":"The Handler needs an existing record before it can apply the request. The change adds `FindByID` to the Repository interface and implements that lookup.","importance":"core","order":1,"file_categories":[{"path":"src/repository.go","category":"logic"}],"review_steps":[{"id":"repository-method","title":"Add the Repository method","summary":"`FindByID` provides the existing record required before the request can be processed.","fragment_ids":["repository-find-by-id"]}],"fragments":[{"id":"repository-find-by-id","path":"src/repository.go","ranges":[{"old":{"start":10,"lines":4},"new":{"start":10,"lines":7}},{"old":{"start":80,"lines":2},"new":{"start":83,"lines":4}}],"description":"Adds `FindByID` to the Repository interface and implements the lookup.","review_level":"careful"}]}]}
```

Every Group must have `importance` set to `core`, `supporting`, or `side`. Every Fragment must have `review_level` set to `careful`, `normal`, or `skim`; omitted draft values default to `normal`. Every fragment must occur exactly once across all groups and must contain `id`, `path`, at least one `ranges` entry (or `file_metadata: true`), and a non-empty `description`. Every Group must have one or more `review_steps`, and every Group Fragment must occur exactly once in their ordered `fragment_ids`. Every changed old/new line and file metadata change must be selected exactly once. Every file referenced by a Group must occur exactly once in that Group's `file_categories`.

## Group importance

Importance tells the reviewer how a Group relates to the purpose of the PR, not how risky, large, difficult, or mechanically complex it is. Classify by mentally removing the whole Group:

- `core`: removing it removes the reason the PR exists.
- `supporting`: the core purpose remains recognizable, but becomes incomplete, broken, unexplained, or unverified.
- `side`: the core purpose and its completeness remain substantially intact; this is a separately meaningful change bundled into the same PR.

Do not classify from file type or change mechanics alone. Required generated output, configuration, tests, documentation, renames, or formatting can be `supporting` when the core change would be incomplete without them. Opportunistic cleanup or formatting can be `side` when removing it would not weaken the core change. `side` does not mean invalid or unreviewable; it means the Group is not part of completing the PR's central purpose.

## Grouping drafts

`semdiff grouping init` creates a draft schema version 4 file at `.semdiff/grouping-draft.json` by default. `grouping init --from <groups-file>` creates a new current-range draft while carrying forward a compatible earlier finalized review. Recreate an older draft with `grouping init --force`; do not continue it because the schema and authored Fragment fields have changed. The draft is the working state; the finalized v3 `groups.json` is written beneath `.semdiff/reviews/` by default so it is not accidentally committed. Apply operations can be repeated and can add, revise, move, or remove decisions. Use `--draft <path>` when maintaining more than one draft.

An apply request contains a batch of operations. For example:

```json
{
  "operations": [
    {"op":"upsert_group","group_id":"repository-lookup","title":"Add Repository lookup","summary":"Adds the lookup used by the Handler.","importance":"core","order":1},
    {"op":"merge_fragments","members":["F-candidate-1","F-candidate-2"],"fragment":{"id":"repository-find-by-id","description":"Adds `FindByID` to the Repository interface and implements the lookup.","review_level":"careful"}},
    {"op":"assign_fragments","group_id":"repository-lookup","members":["repository-find-by-id"]},
    {"op":"set_review_steps","group_id":"repository-lookup","review_steps":[{"id":"repository-method","title":"Add the Repository method","summary":"`FindByID` gives the Handler the record it needs before it processes the request.","fragment_ids":["repository-find-by-id"]}]},
    {"op":"set_file_categories","group_id":"repository-lookup","categories":{"src/repository.go":"logic"}}
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

## Technical explanation style

### Explanation level

Match the explanation to the abstraction level of the code being described. Do not generalize a concrete change into higher-level design language unless that additional design meaning is needed.

Prefer vocabulary that maps directly to the code: interface, type, method, function, signature, parameter, return value, construction, call, registration, DI, implementation, and conversion. For example, when a change adds a method to an interface, first say `Adds FindByID to the Repository interface`, rather than only saying that it expands a contract or changes a boundary.

When design meaning is useful, explain it after the concrete code change. Use abstract terms such as `contract`, `boundary`, `ownership`, `wiring`, and `responsibility` when those concepts are themselves under discussion, not as replacements for a direct description of the implementation. Prefer precise and direct wording over formal or abstract wording with the same meaning.

### Japanese technical vocabulary

When writing Japanese, use vocabulary and notation natural to Japanese software development. Do not mechanically translate English technical terms into unfamiliar Japanese expressions. In particular, do not automatically translate `contract`, `wiring`, `ownership`, or `boundary` as `契約`, `接続`, `所有`, or `境界`.

Keep terms in English or katakana when that is natural in Japanese technical writing, including `interface`, `API`, `DI`, `Repository`, `Handler`, `Adapter`, `signature` / `シグネチャ`, and `middleware` / `ミドルウェア`. Prioritize wording that Japanese software engineers will find accurate and natural over fully translating every term.

## Fragment descriptions

Describe the concrete code change and its behavior or test coverage—not the file operation. Do not repeat the file name or path, and do not lead with bookkeeping such as “updated,” “added,” “deleted,” “new file,” or “first half”; the data structure and viewer already show that context.

When the diff, surrounding code, commit narrative, or group intent provides evidence, include the reason or purpose after the concrete change. Useful reasons include enabling a workflow, removing duplicated code, preserving a validation rule, adapting callers to a new method, or preventing a regression. Do not invent rationale that the available evidence does not support; when the reason is unclear, state only the concrete semantic change.

Prefer descriptions such as:

- `Removes the duplicate render call and invokes SharedButton.Render instead.`
- `Adds table-driven tests for the supported operations and their error results.`
- `Calls CreateCommand.Execute from the create Handler so creation uses the same validation as the CLI command.`

Avoid descriptions such as:

- `ファイルを更新。古いコードを削除。`
- `テストファイルを新規追加。関連するテストを追加。`
- `ファイル前半部分を更新。新しい依存関係を追加。`

## Group review order

Choose Group `order` as a reviewer-oriented dependency order, not alphabetical order, file order, or the order in which fragments were discovered. If Group B relies on a contract, type, schema, helper, migration, or behavior introduced by Group A, place A before B. Apply this transitively across the full set of Groups: establish prerequisites first, then dependent behavior and integrations, then follow-up validation or documentation when those are genuinely separate concerns.

Infer dependencies from imports and call sites, type and schema usage, configuration consumers, commit chronology, and the semantic descriptions—not merely from directory layout. Keep tests in the same Group as the behavior they verify unless the tests form an independently reviewable concern; when they do form a separate Group, place that Group after the behavior it verifies.

When Groups are independent, order them to minimize context switching and make the change read as a coherent narrative—for example, shared foundations before feature-specific uses, and core or supporting Groups before side Groups. Do not invent a dependency solely to force a tidy sequence. Before finalizing, scan the ordered summaries from first to last and revise `order` if a Group requires knowledge that is introduced only later.

## Group summaries

A group `summary` is a review narrative, not a one-line restatement of its fragments. Write multiple sentences (normally 2–4) so a reviewer can understand the motivation and shape of the change without opening every file. A useful summary generally covers:

1. The background or existing situation, when it can be established from the commit history or surrounding code.
2. The problem, limitation, or risk that motivated the work.
3. The concrete implementation changes and, when needed, the rules they preserve.
4. The resulting behavior, tests, or review implication when the evidence supports it.

Use `semdiff commits` as the primary source for background: consider commit subjects, commit bodies, chronology, and which files each commit touched. Use fragment evidence to verify what was actually implemented and to connect the narrative to the grouped changes. Commit history is the current boundary of available context; do not invent product requirements, incidents, user reports, or design decisions that are not supported by commits or code. If the motivation is unclear, say what limitation the diff addresses only when that is directly observable, and keep uncertain interpretation out of the summary.

The summary should explain the relationship among the fragments rather than enumerate filenames or repeat each fragment description. Summary values support Markdown in the viewer. Put the main parts on separate lines in the JSON string: use one line for the background or problem, one for the approach, and optionally one for the result or review implication. Encode those line breaks as `\n` (use `\n\n` for separate Markdown paragraphs when useful). You may use ordinary Markdown such as paragraphs, lists, emphasis, and inline code; do not depend on raw HTML. Keep explicit causal links such as “because,” “so that,” or “which allows.” Do not compress the summary into one line merely to make the JSON shorter.

Prefer a summary shaped like:

```markdown
The previous implementation spread state transitions across direct mutations, which made related updates difficult to keep consistent.
The change adds `ApplyTransition(command)` and passes the inactive state through `SwitchMode`, so both call sites use the same state update.
The associated tests call `ApplyTransition` for each supported mode and preserve the expected inactive state.
```

Avoid a summary shaped like:

`Replaces the old implementation with a command layer, adds caching, and adds tests.`
