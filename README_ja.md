# semdiff

[English](README.md) | 日本語

> **Draft notice:** このリポジトリはまだドラフト段階です。ドキュメントも AI に簡単に生成させたものなので、読みにくかったり正確でない可能性があります。

`semdiff` は、pull request の diff を「何が、なぜ変わったのか」を中心にレビューできる形へ整理します。

## Why this?

コードは完成し、pull request も開きました。そこからレビューが始まります。

- 1つの機能が handler、共通 helper、テスト、設定ファイルに分散している。
- 1つの diff hunk に、振る舞いの変更、リファクタリング、フォーマット変更が混在している。
- commit の履歴は作業の順番を示していても、レビューすべき順番を示しているとは限らない。
- 変更が正しいか考える前に、どの変更が一緒に属するのかを読み手が整理しなければならない。

`semdiff` は、AI Agent に diff をレビューのストーリーへ変換させます。関連する変更をまとめ、各グループの目的を説明し、レビューしやすい順番で表示します。すべての変更行とファイル metadata がレビューに含まれていることも検証します。Git の履歴と実装コードは変更しません。

## Quick Start

Quick Start は人間のレビュー workflow を中心にしています。CLI とスキルをインストールしたら、後述する低レベルのコマンドはスキルに実行させます。

### 1. CLI とスキルをインストールする

リポジトリを clone し、CLI をインストールして、同梱されている2つのスキルを Agent の skills directory にコピーします。Codex の場合は次の通りです。

```sh
git clone https://github.com/ry023/semdiff.git
cd semdiff
go install .

mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/semantic-grouping skills/answer-semdiff \
  "${CODEX_HOME:-$HOME/.codex}/skills/"
```

インストール後に Agent を再起動すると、スキルが認識されます。`semdiff` 実行ファイルも Agent の `PATH` に含まれている必要があります。

### 2. Agent に変更を整理させる

レビュー対象の Git repository で AI Agent を開き、次を実行します。

```text
/semantic-grouping
```

Agent はレビュー範囲を決め、commit と変更コードを調べ、semantic Group と Fragment を作り、結果を検証します。確定したレビューは `.semdiff/reviews/` 以下にローカル保存されます。commit や Git の履歴は書き換えません。

特定の range をレビューする場合は、スキル呼び出しに追加します。

```text
/semantic-grouping main..HEAD
```

### 3. レビューを開く

Agent が完了したら、同じ repository で interactive viewer を起動します。

```sh
semdiff view
```

`http://127.0.0.1:7363` を開くと、commit 順やファイル順ではなく、semantic な順番で変更を読めます。

### 4. レビューについて質問する

Viewer を起動したまま、別の Agent session で次を実行します。

```text
/answer-semdiff
```

Agent が回答モードに入り、質問を待ちます。Viewer で Group や Fragment に質問したり、同じ thread で follow-up を続けたり、変更と回答を並べて確認できます。終わるときは Viewer の **End answer mode** を選択します。

## Concepts

### semdiff は何をするか

`semdiff` は、選択した Git の変更範囲（`base..head`）に semantic なレビュー層を追加します。Git が提供するのは commit、変更行、ファイル metadata といった事実です。レビュー層は、その事実をレビューで理解しやすい関心ごとに並べ替えます。

```text
Git range → change map → semantic Fragment → ordered Group → review viewer
```

AI Agent には、commit 履歴とコードを根拠に、次のことを求めます。

- 単独でレビューできる最小の変更を見つける。
- 関連する変更を、ファイルをまたいで semantic Group にまとめる。
- 各変更が何をし、なぜ必要なのかを、根拠がある範囲で説明する。
- レビュー順、ファイル category、Group の importance、Fragment の review level を設定する。
- 変更されたすべての行とファイル metadata が、ちょうど1つの Fragment で選択されたレビューを作る。

意味の判断は Agent が行います。CLI は決定的な事実を取得し、再開可能な draft を保存し、coverage を検証します。`semdiff` は commit を書き換えず、実装を編集せず、Git の hunk 境界を最終的なレビュー構造として扱いません。結果の `groups.json` は派生したレビュー成果物であり、Git の履歴と変更事実はそのまま残ります。

### Fragment

Fragment は、レビュアーが単独で理解し説明できる最小の変更単位です。1つのファイルパスと、1つ以上の変更前／変更後の行範囲（または rename や binary change などの file metadata の所有権）で定義します。

Fragment は Git hunk とは異なる semantic な単位です。

- 1つの Git hunk に、独立してレビューできる複数の責務が混在していれば、複数の Fragment に分けます。
- 離れた複数の範囲が1つの責務を実装しているなら、1つの Fragment にまとめます。
- 新しいファイルでも複数の Fragment を持てます。ファイル境界は Fragment 境界ではありません。
- import、delimiter、句読点などの構造要素は、それを必要とする Fragment に含めます。単独でレビューする意味がないものを独立した Fragment にしません。

範囲内の未変更行は context として表示されますが、coverage の対象になるのは追加行、削除行、ファイル metadata の変更だけです。確定したレビューでは、すべての authored Fragment がちょうど1つの Group に属します。

### Group

Group は、複数ファイルにまたがっていても、1つのレビュー上の関心ごとを説明する Fragment をまとめたものです。Group には title、背景と変更を説明する summary、レビュー順、pull request 全体における `importance` があります。

- `core` は pull request が存在する理由そのものです。
- `supporting` は core の変更を完成させ、適応させ、説明し、または検証します。
- `side` は同じ pull request に含まれる、独立した意味を持つ別の変更です。

## semdiff を AI Agent と使う

同梱されている2つのスキルは、レビューの異なる段階を担当します。

- `semantic-grouping` は、coverage を満たす semantic review を作成または更新します。実装が終わった後や、現在の branch をレビュー向けに整理し直したいときに使います。
- `answer-semdiff` は、interactive viewer の Group や Fragment に付けられた質問に回答します。Viewer で回答モードを終了するまで待ち続け、コードやレビュー成果物は編集しません。

Agent には、通常の言葉でレビューの観点を追加できます。

> `main..HEAD` を、公開 API と migration path を特に重視して Group 化してください。

> 最新の変更を反映して semantic grouping を見直し、レビューを更新してください。

> 実装と commit 履歴を根拠に、保留中の semdiff の質問へ回答してください。

Agent が Fragment の範囲を選び、operation JSON を書き、質問 session を調整する必要はありません。そうした CLI interface は、自動化、デバッグ、integration のためにも引き続き利用できます。

## CLI Reference

CLI はスキルが利用する決定的な layer です。完全なコマンドリファレンスは別ファイルにあります。

[CLI リファレンスを読む →](CLI_REFERENCE_ja.md)

## レビューを共有する

range を省略した `grouping init` は、`gh` で現在の PR を特定できる場合はその base branch を、できない場合は Git remote の default branch を使います。merge-base と `HEAD` を比較し、`<base>..<head>` を渡せば明示的に上書きできます。`grouping finalize` は出力先を省略すると、Git 管理外の `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` に保存します。明示的な groups file も利用できます。`show`、`validate`、`questions`、`publish` は、groups file の省略時に現在の grouping draft から確定済みファイルを特定します。

groups file と `--draft` の両方を省略した `semdiff view` は、`grouping init` と同じロジックで現在の range を独立に計算します。完全一致する確定済み review があればそれを開き、なければ現在の head の first-parent history をたどって、同じ merge-base を持つ最も近い review を開きます。古い snapshot を表示する場合は HEAD より遅れていることを明示し、未レビューの commit と変更 path を別枠に表示します。古い Fragment range を現在の diff に適用することはありません。fallback を許可しない場合は `--exact`、特定の snapshot を選ぶ場合は groups file または `--draft <path>` を指定します。

`publish` はレビュー成果物である `groups.json` だけを Git の artifact branch に保存します。質問 thread はローカルのままで、共有・upload されません。設定なしでは現在の repository の `origin` と `semdiff/reviews` branch を使います。branch がまだなければ最初の publish 時に orphan branch として作成されます。

保存先は full SHA による `<base-sha>...<head-sha>/groups.json` です。`semdiff reviews view` は branch の一覧を表示し、Refresh で fetch します。

任意の repository 共通 `semdiff.yaml` で保存先を指定できます。

```yaml
review_store:
  remote: origin
  branch: semdiff/reviews
```

別 repository を使う場合は、Git 管理外の個人用 `.semdiff/config.local.yaml` に `repository` を設定できます。

```yaml
review_store:
  repository: git@github.com:org/semdiff-reviews.git
  branch: semdiff/reviews
```

CLI flag、ローカル設定、`semdiff.yaml`、既定値の順に優先されます。`remote` と `repository` は同時に指定できません。

Viewer では semantic Group または Fragment に質問 thread を紐づけられます。同じ thread への follow-up は回答済み turn を文脈として継続し、新しい Ask は独立した context を開始します。`semdiff view` を起動したまま、AI Agent に `answer-semdiff` skill を開始させてください。skill は回答 session を開始し、pending の turn を1件ずつ claim します。Agent は回答を登録したあと次の質問を待ち、Viewer の「End answer mode」で session を終了すると skill も完了します。回答モード外では Ask button を隠し、開始方法の案内を Viewer 上部に表示します。thread の状態は `.semdiff/questions/`、現在の回答 session は `.semdiff/sessions/` に保存され、どちらも `groups.json` から分離されています。

`semdiff view --html review.html` は Viewer を自己完結型の読み取り専用 HTML として出力します。`--include-answers` を追加すると、回答済み turn の snapshot も含めます。静的 HTML では Ask、follow-up、回答 session の操作は利用できません。

`fragments` は Git から取得した context なしの変更範囲を初期候補として出力します。この範囲は Fragment の確定境界ではありません。`grouping init` は `.semdiff/grouping-draft.json` 内で候補と authored Fragment を分けて保存するため、Git の各変更範囲がそのまま割り当て義務にはなりません。`grouping inspect --suggestions` で候補を確認し、finalize の前に `merge_fragments`、`add_fragment`、`update_fragment`、`delete_fragments` を使って semantic Fragment を構成します。

`classify` はパスから標準 category `logic`、`component`、`config`、`implementation`、`test`、`docs`、`unknown` を提案します。Markdown や reStructuredText などのドキュメント拡張子、`README` や `CHANGELOG` などの定番ファイル名、`docs/` や `guides/` などのドキュメント用 directory 配下を `docs` に分類します。

この workflow では draft schema version 3、最終的な `groups.json` schema version 2 を使用します。古い draft は `grouping init --force` で作り直してください。

`groups.json` が source of truth です。各 Fragment はファイルパス、1つ以上の変更前・変更後の行範囲、semantic な説明を保持します。

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

行番号は1始まりで、`lines` には正の値を指定します。純粋な追加では `old` を、純粋な削除では `new` を省略します。複数の range を指定すると、離れた変更箇所を1つの semantic Fragment として選択できます。rename、mode、binary、ファイル作成、ファイル削除など、行以外の変更を所有する Fragment には `file_metadata: true` を指定します。

Group の `importance` は PR 全体における位置づけを `core`、`supporting`、`side` で表します。`core` は PR が存在する理由、`supporting` は core を完成させる変更、`side` は同じ PR に含まれた別の意味ある変更です。authored Fragment の `review_level` は、その変更をどの程度丁寧に読むべきかを `careful`、`normal`、`skim` で表します。draft で省略した場合は `normal` が設定され、最終ファイルには明示的に保存されます。

validation は、指定された range と現在の `base_sha..head_sha` の diff を比較します。追加行、削除行、ファイル metadata の各変更は、必ずちょうど1つの Fragment に選択されなければなりません。range に未変更行が含まれていても構いません。未変更行は coverage に影響しません。

Draft の operation 形式と各 CLI command の詳細は [CLI リファレンス](CLI_REFERENCE_ja.md) を参照してください。
