# semdiff

[English](README.md) | 日本語

`semdiff` は、Git の履歴を変更せずに、指定した Git range をレビュー向けの意味的なグループへ再構成します。fragment は Git の hunk 境界ではなく、ファイルの行範囲によって定義されます。

## インストール

```sh
go install .
```

## 使い方

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

`fragments` は、Git から取得したコンテキストなしの変更範囲を初期候補として出力します。この範囲は fragment の確定境界ではありません。`grouping init` は `.semdiff/grouping-draft.json` 内で候補と authored fragment を分離して保存するため、Git の各変更範囲がそのまま割り当て義務にはなりません。`grouping inspect --suggestions` で候補を確認し、finalize の前に `merge_fragments`、`add_fragment`、`update_fragment`、`delete_fragments` を使って意味的な fragment を構成します。

`classify` はパスから標準カテゴリ `logic`、`component`、`config`、`implementation`、`test`、`docs`、`unknown` を提案します。Markdown や reStructuredText などのドキュメント拡張子、`README` や `CHANGELOG` などの定番ファイル名、`docs/` や `guides/` などのドキュメント用ディレクトリ配下を `docs` に分類します。

この workflow では draft schema version 2 を使用します。version 1 の draft は `grouping init --force` で作り直してください。最終的な `groups.json` の schema は引き続き version 1 です。

`groups.json` が source of truth です。各 fragment はファイルパス、1つ以上の変更前・変更後の行範囲、意味的な説明を保持します。

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

行番号は1始まりで、`lines` には正の値を指定します。純粋な追加では `old` を、純粋な削除では `new` を省略します。複数の range を指定すると、離れた変更箇所を1つの意味的な fragment として選択できます。

rename、mode、binary、ファイル作成、ファイル削除など、行以外の変更を所有する fragment には `file_metadata: true` を指定します。

validation は、指定された range と現在の `base_sha..head_sha` の diff を比較します。追加行、削除行、ファイル metadata の各変更は、必ずちょうど1つの fragment に選択されなければなりません。range に未変更行が含まれていても構いません。未変更行は coverage に影響しません。

## Draft 操作

Draft 操作は atomic に適用され、ファイルまたは標準入力から読み込めます。fragment ID は draft および最終ファイル内で使用するローカルな操作用ハンドルです。fragment の実体と source of truth は range です。

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

`merge_fragments` は、同じファイルに属する1つ以上の候補または authored fragment から、複数 range を持つ1つの fragment を生成します。明示的な定義操作には `add_fragment`、`update_fragment`、`delete_fragments` を使用できます。Group への所属操作には `assign_fragments`、`move_fragments`、`unassign_fragments` を使用できます。

fragment の境界は「連続しているか」ではなく「単独で理解できるか」で決めます。閉じ括弧、句読点だけの変更、対応先なしでは意味を持たない構文要素、隣接する変更のためだけの import を単独 fragment にしないでください。完全な構文要素または振る舞いを所有する fragment に、その range を含めます。
