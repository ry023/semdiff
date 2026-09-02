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
7. For every new or revised fragment, write a short semantic label for the one change worth noticing. Include why only when it changes the review judgment because the purpose, constraint, or relationship is not clear from the diff. For every file referenced by a Group's fragments, set exactly one `file_categories` entry. Start from `classify` output, then confirm or revise it using commit intent, path semantics, and relevant Fragment content.
8. Repeat `status`, `inspect`, and `apply` as needed. Drafts are intentionally allowed to be incomplete; do not stop merely because one batch has unassigned fragments.
9. Before finalizing, review each file for both under- and over-fragmentation. For a large suggestion or new file, inspect its top-level responsibilities and use `add_fragment` with explicit ranges when functions, types, handlers, or tests can be explained and questioned independently. Do not split merely at a file boundary, Git hunk, test boundary, import, comment, or formatting change. Merge fragments whose meaning cannot be explained independently, especially delimiter-only or syntax-only fragments and fragments that merely complete a neighboring construct.
10. After assigning fragments, create `review_steps` for every Group with `set_review_steps`. First list the questions a reviewer must answer in order, then map each Fragment to one Step. A Step exists only when it introduces a new prerequisite, question, or judgment; do not target a number of Steps. Write its title as a concise, concrete action that says what this part of the change does, such as `Add FindByID to Repository` or `Call CreateCommand.Execute from the Handler`. Do not use a noun phrase, a design label, or a reading instruction. In order, the Step titles should form an action-level outline that complements the Group summary. Write its summary as a compact Markdown outline: each bullet makes one claim about this stage, and a child bullet only supplies that parent claim's reason, concrete detail, consequence, or dependency. Write descriptively about the implementation; do not instruct the reviewer to “read,” “review,” or “look at” something. Adjacent Steps that answer the same question belong together. Each Group Fragment must occur exactly once across its Steps.
11. Run `semdiff grouping finalize --json`. With no explicit output path, finalize writes the result to the Git-ignored `.semdiff/reviews/<base-sha>...<head-sha>/groups.json`; use an explicit path only when the user or surrounding workflow requires one. Finalize succeeds only when every authored fragment is assigned, described, and placed in a complete review Step, every Group has a complete summary and file categories, and every changed line and metadata change is selected exactly once.

The required shape is:

```json
{"version":3,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"repository-lookup","title":"Add Repository lookup","summary":"- The Handler needs an existing record before it can apply the request.\n  - `FindByID` supplies that record.\n- Add `FindByID` to the Repository interface and implement the lookup.","importance":"core","order":1,"file_categories":[{"path":"src/repository.go","category":"logic"}],"review_steps":[{"id":"repository-method","title":"Add FindByID to Repository","summary":"- `FindByID` supplies the existing record before the Handler processes the request.","fragment_ids":["repository-find-by-id"]}],"fragments":[{"id":"repository-find-by-id","path":"src/repository.go","ranges":[{"old":{"start":10,"lines":4},"new":{"start":10,"lines":7}},{"old":{"start":80,"lines":2},"new":{"start":83,"lines":4}}],"description":"Adds `FindByID` to the Repository interface and implements the lookup.","review_level":"careful"}]}]}
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
    {"op":"upsert_group","group_id":"repository-lookup","title":"Add Repository lookup","summary":"- The Handler needs an existing record before it can apply the request.\n  - `FindByID` supplies that record.\n- Add `FindByID` to the Repository interface and implement the lookup.","importance":"core","order":1},
    {"op":"merge_fragments","members":["F-candidate-1","F-candidate-2"],"fragment":{"id":"repository-find-by-id","description":"Adds `FindByID` to the Repository interface and implements the lookup.","review_level":"careful"}},
    {"op":"assign_fragments","group_id":"repository-lookup","members":["repository-find-by-id"]},
    {"op":"set_review_steps","group_id":"repository-lookup","review_steps":[{"id":"repository-method","title":"Add FindByID to Repository","summary":"- `FindByID` supplies the existing record before the Handler processes the request.","fragment_ids":["repository-find-by-id"]}]},
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

Step titles follow the same rule: name the concrete action rather than a category or an abstract design concept. When writing Japanese, use a natural action phrase such as `Repository に FindByID を追加する` or `Handler から CreateCommand.Execute を呼び出す`. Avoid noun phrases such as `Repository メソッド` and abstract labels such as `検索の責務`.

### Japanese technical vocabulary

When writing Japanese, use vocabulary and notation natural to Japanese software development. Do not mechanically translate English technical terms into unfamiliar Japanese expressions. In particular, do not automatically translate `contract`, `wiring`, `ownership`, or `boundary` as `契約`, `接続`, `所有`, or `境界`.

Keep terms in English or katakana when that is natural in Japanese technical writing, including `interface`, `API`, `DI`, `Repository`, `Handler`, `Adapter`, `signature` / `シグネチャ`, and `middleware` / `ミドルウェア`. Prioritize wording that Japanese software engineers will find accurate and natural over fully translating every term.

## Fragment descriptions

Fragment descriptions are short labels, not miniature summaries. State the one concrete code change worth noticing, normally as one clause or sentence. Do not repeat the file name or path, and do not lead with bookkeeping such as “updated,” “added,” “deleted,” “new file,” or “first half”; the data structure and viewer already show that context.

Do not enumerate imports, comments, formatting, generated output, test setup, or other supporting mechanics when they have no independent review meaning. Include them in the Fragment that owns the behavior they support. If such a change must remain separate, give it the minimum direct label and use `skim` when appropriate.

Why is optional, not a default. Add it only when the reason is needed for review judgment and is not apparent from the diff: for example, a validation rule, a non-obvious caller adaptation, or a regression that the change prevents. Do not invent rationale that the available evidence does not support. For `skim` Fragments, omit why unless it is essential to avoid a misleading review.

Prefer descriptions such as:

- `Adds FindByID to the Repository interface.`
- `Calls CreateCommand.Execute from the create Handler.`
- `Rejects duplicate IDs so retries cannot create a second record.`
- `Adds table-driven tests for duplicate-ID errors.`

Avoid descriptions such as:

- `ファイルを更新。古いコードを削除。`
- `テストファイルを新規追加。関連するテストを追加。`
- `ファイル前半部分を更新。新しい依存関係を追加。`
- `FindByID のための import とコメントを追加。`
- `空IDを拒否する理由を説明するコメントを追加。`

## Group review order

Choose Group `order` as a reviewer-oriented dependency order, not alphabetical order, file order, or the order in which fragments were discovered. If Group B relies on a contract, type, schema, helper, migration, or behavior introduced by Group A, place A before B. Apply this transitively across the full set of Groups: establish prerequisites first, then dependent behavior and integrations, then follow-up validation or documentation when those are genuinely separate concerns.

Infer dependencies from imports and call sites, type and schema usage, configuration consumers, commit chronology, and the semantic descriptions—not merely from directory layout. Keep tests in the same Group as the behavior they verify unless the tests form an independently reviewable concern; when they do form a separate Group, place that Group after the behavior it verifies.

When Groups are independent, order them to minimize context switching and make the change read as a coherent narrative—for example, shared foundations before feature-specific uses, and core or supporting Groups before side Groups. Do not invent a dependency solely to force a tidy sequence. Before finalizing, scan the ordered summaries from first to last and revise `order` if a Group requires knowledge that is introduced only later.

## Group and Step summaries

A Group or Step `summary` is a compact Markdown outline, not a prose narrative or a restatement of every Fragment. Write one claim per bullet so the reviewer can scan the shape of the change without opening every file. Use a child bullet only for its parent's reason, concrete detail, consequence, or dependency; keep parallel claims at the same indentation level. Prefer one nesting level and use a second only when it makes a real dependency clearer. Do not add bullets merely to reach a target count: one important claim may need one bullet, while a larger Group may need more.

A Group summary generally covers the background or limitation when supported by evidence, the concrete implementation changes, and the resulting behavior, tests, or review implication when useful. A Step summary covers only that stage's role and its direct relationship to nearby Steps. Fragment descriptions remain short one-line labels, not outlines.

Use `semdiff commits` as the primary source for background: consider commit subjects, commit bodies, chronology, and which files each commit touched. Use fragment evidence to verify what was actually implemented and to connect the narrative to the grouped changes. Commit history is the current boundary of available context; do not invent product requirements, incidents, user reports, or design decisions that are not supported by commits or code. If the motivation is unclear, say what limitation the diff addresses only when that is directly observable, and keep uncertain interpretation out of the summary.

The summary should explain the relationship among the fragments rather than enumerate filenames or repeat each fragment description. Summary values support Markdown in the viewer. Use `- ` for top-level bullets and two spaces before `- ` for a child bullet; encode line breaks as `\n` in JSON. Use inline code and emphasis where useful, but do not depend on raw HTML. Keep explicit causal links such as “because,” “so that,” or “which allows.”

Prefer a summary shaped like:

```markdown
- State transitions were spread across direct mutations.
  - Related updates could diverge between call sites.
- Add `ApplyTransition(command)` and pass the inactive state through `SwitchMode`.
  - Both call sites now use the same state update.
- Test each supported mode through `ApplyTransition`.
```

Avoid a summary shaped like:

`- Replace the old implementation with a command layer, add caching, and add tests.`
