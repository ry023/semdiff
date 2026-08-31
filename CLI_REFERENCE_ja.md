# semdiff CLI リファレンス

このファイルは、同梱スキルが利用する決定的な CLI layer のリファレンスです。integration の構築、中間状態の確認、Agent 実行の troubleshooting、Agent を使わない semdiff の操作に利用してください。

人間および AI Agent 向けの workflow、インストール方法、レビュー共有については [README](README_ja.md) を参照してください。

## Draft workflow の全体像

`semantic-grouping` skill が内部で行う基本的な workflow は次の通りです。

```sh
semdiff grouping init main..HEAD
semdiff grouping inspect --suggestions --json
semdiff grouping apply decisions.json --json
semdiff grouping status --json
semdiff grouping finalize --json
semdiff validate
```

`grouping init` は `.semdiff/grouping-draft.json` を作成します。operation file には `merge_fragments`、`upsert_group`、`assign_fragments` などの atomic な変更を記述します。形式は [Draft 操作](#draft-操作) を参照してください。`grouping finalize` はデフォルトで `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` に書き込みます。

range を省略すると、`gh` が現在の pull request を特定できる場合はその base branch を、それ以外では Git remote の default branch を使います。

多くの inspection および mutation command は `--json` を受け付けます。AI Agent や別のプログラムから利用するときに便利です。flag は positional argument の前後どちらにも置けます。

## 変更範囲を理解する

grouping の前や途中で range を調べるには、次の読み取り専用 command を使います。

```sh
semdiff commits main..HEAD
semdiff fragments main..HEAD
semdiff classify main..HEAD
```

- `commits` は range 内の commit、author、変更ファイル数を表示します。
- `fragments` は semantic Fragment の出発点にできる、Git 由来の context なし変更範囲を出力します。これらは最終的な Fragment 境界ではありません。
- `classify` は変更された各 path の category（`logic`、`test`、`docs`、`config` など）を提案します。

構造化された出力が必要な場合は、これらの command に `--json` を追加します。

## Semantic Group を作成する

Grouping は draft を段階的に更新する workflow です。

```sh
semdiff grouping init [main..HEAD]
semdiff grouping inspect --suggestions
semdiff grouping apply decisions.json
semdiff grouping status
semdiff grouping finalize
```

- `grouping init` は base／head commit、Git 由来の候補、path の category 提案を `.semdiff/grouping-draft.json` に保存します。既存 draft を置き換えるには `--force`、別の場所を使うには `--draft <path>` を指定します。
- `grouping inspect` は draft の一部を絞って表示します。`--suggestions`、`--unassigned`、`--group <id>`、`--fragment <id>` のいずれか1つを必ず指定します。
- `grouping apply` は JSON file にある operation を atomic に適用します。標準入力から読む場合は file 名の代わりに `-` を渡します。
- `grouping status` は assignment と description の進捗、未設定の review metadata、finalize の準備状況を表示します。
- `grouping finalize` は draft と現在の Git diff を検証し、canonical な `groups.json` を書き込みます。出力先を明示することもできます。

すべての grouping command は `--draft <path>` を受け付けます。operation の形式は [Draft 操作](#draft-操作)、artifact の既定パスは [レビューを共有する](README_ja.md#レビューを共有する) を参照してください。

## Fragment を確認・検証する

`show` は、Fragment に選択された行を表示します。離れた複数の範囲にも対応しています。active draft または finalized review のどちらからでも読み込めます。

```sh
semdiff show --draft .semdiff/grouping-draft.json F-0123456789ab
semdiff show F-0123456789ab
semdiff show path/to/groups.json F-0123456789ab
```

`validate` は finalized review と、その `base_sha..head_sha` diff を照合します。追加行、削除行、metadata の変更は、すべてちょうど1つの Fragment に属していなければなりません。

```sh
semdiff validate
semdiff validate path/to/groups.json --json
```

groups file を指定しない場合、両 command は現在の grouping draft から finalized artifact を特定します。

## レビューを表示・export する

デフォルトの address で interactive viewer を起動します。別の listen address も指定できます。

```sh
semdiff view
semdiff view --addr 127.0.0.1:8080
```

groups file と `--draft` の両方を省略した `view` は、`grouping init` と同じロジックで現在の pull request range を計算します。完全一致する確定済み review を優先し、なければ現在の head の first-parent history 上にある同一 base の最も近い review を開きます。その場合、semantic grouping に未反映の commit と path を明示します。完全一致を必須にするには `semdiff view --exact` を使います。

server を起動する代わりに自己完結型の読み取り専用 file を作るには `--html` を使います。質問への回答は、明示的に含めない限り出力されません。

```sh
semdiff view --html review.html
semdiff view --html review.html --include-answers
```

いずれの形式でも、明示的な `groups.json` path を指定できます。`--draft <path>` を指定すると、draft による artifact 選択を明示的に利用できます。

## レビュー質問に回答する

interactive viewer は、Group または Fragment に質問 thread を紐づけられます。回答 worker は session を使って質問を待ち、回答を送信します。

```sh
semdiff questions session start --json
semdiff questions wait --session S-... --json
printf '%s\n' '回答本文' | semdiff questions answer Q-... --stdin
```

- `questions session start` は回答モードを開始し、session ID を返します。
- `questions wait` は session に質問が届くか、Viewer で停止されるまで待ちます。`--session` を省略すると、次の pending question を直接待ちます。
- `questions answer` は標準入力から空でない回答を読み込み、指定された question thread に追加します。

これらの command はデフォルトで現在の finalized review を使います。groups file を明示でき、`--draft <path>` で既定 artifact の特定方法を変更できます。

## レビューを公開・一覧表示する

現在の finalized review を、設定された Git artifact branch に公開します。

```sh
semdiff publish
```

その branch に保存されたレビューを fetch して一覧表示します。

```sh
semdiff reviews view
semdiff reviews view --addr 127.0.0.1:8080
```

両 command は `--remote`、`--repository`、`--branch` による上書きを受け付けます。`publish` が upload するのは `groups.json` だけで、ローカルの質問 thread は含まれません。設定と保存方法は [レビューを共有する](README_ja.md#レビューを共有する) を参照してください。

## Draft 操作

Draft 操作は atomic に適用され、file または標準入力から読み込めます。Fragment ID は draft と最終 file 内で使うローカルな handle です。Fragment の実体と source of truth は range です。

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

`merge_fragments` は、同じ path の候補または authored Fragment から、複数 range を持つ1つの Fragment を生成します。明示的な定義操作には `add_fragment`、`update_fragment`、`delete_fragments` を使用できます。Group の所属操作には `assign_fragments`、`move_fragments`、`unassign_fragments` を使用できます。

Fragment は、単に連続した範囲ではなく、単独で理解できる単位にします。閉じ括弧、句読点だけの変更、対応先のない構造要素、隣接する変更のためだけの import を独立した Fragment にしないでください。その範囲を、完全な構造や振る舞いを所有する Fragment に含めます。
