# MVP: Semantic Git Diff Viewer

GitのPR/ブランチ差分を、ファイル単位やコミット単位ではなく、**意味のある変更単位（Semantic Group）に再構成してレビューできるツール**を実装してください。

目的はGit履歴を書き換えることではありません。既存のGit commitやEntire.io等のprovenanceはそのまま維持し、その上に**レビュー専用のsemantic layer**を構築します。

## 背景

大きなPRでは `Files changed` に大量のファイルが並び、変更全体を理解するのが難しくなります。

しかし実際の変更は例えば、

* 新しいdomain conceptの追加
* DBへの永続化
* APIへの公開
* frontend対応
* test更新
* generated/mechanical change

のような意味単位に分けられます。

このツールではGit diffを細かな差分断片へ分解し、Agentがそれらを意味単位にグループ化し、その結果をWeb Viewerで表示します。

## 実装言語

**Goを使用してください。**

CLI、Git操作、データモデル、validation、ローカルWeb ServerはGoで実装してください。

Web ViewerはMVPでは可能な限り軽量にしてください。

* Go標準ライブラリの `net/http` を基本とする
* HTML/CSS/JavaScriptで十分ならフロントエンドフレームワークは導入しない
* 必要であればWeb assetsをGo binaryへembedしてよい
* dependenciesは最小限にする
* 最終的に単一バイナリとして配布しやすい構成を優先する

## コア概念

このMVPでは主に以下の2つのデータ構造を扱います。

### DiffFragment

`DiffFragment` は、`base..head` 間の変更の一部分を表すレビュー用の最小単位です。

1つのファイル内に複数存在でき、それぞれ別のSemantic Groupへ所属できます。

最低限、以下の情報を保持してください。

```text
DiffFragment
- id
- base commit SHA
- head commit SHA
- file path
- old range
- new range
- changed lines / patch
```

`id` は、特定の `base..head` に対して一意に識別できれば十分です。

MVPでは、PR更新をまたいで同一fragmentを永続的に追跡する必要はありません。

例えば以下を元にIDを生成して構いません。

```text
base SHA
+ head SHA
+ file path
+ old/new range
+ 必要ならpatch hash
```

DiffFragmentの境界は、まずは実装しやすい方法で構いません。

例えば、連続する変更行のまとまりを1つのDiffFragmentとして扱ってください。

ただし、将来的により細かく分割できるよう、データモデルと内部APIは特定の分割方式に強く依存しない形にしてください。

### SemanticGroup

`SemanticGroup` は、複数のDiffFragmentを意味的にまとめたレビュー単位です。

例えば、

```text
Introduce OrderStatus
Persist OrderStatus
Expose OrderStatus through API
Display OrderStatus in frontend
Update tests
Generated changes
```

のような単位です。

最低限、以下を保持してください。

```text
SemanticGroup
- id
- title
- summary
- fragment IDs
- optional review order
```

## 全体構成

以下の3レイヤーに分けて実装してください。

```text
Git repository
    ↓
Diff Fragment extraction / CLI
    ↓
Semantic Grouping Agent Skill
    ↓
groups.json
    ↓
Web Viewer
```

## 1. Diff Fragment Data Model / Git CLI

Git repositoryからcommit rangeとdiffを取得し、DiffFragmentへ変換するGo CLIを実装してください。

CLI名は `semdiff` とします。

最低限、以下の操作を提供してください。

```bash
semdiff commits <base>..<head>

semdiff fragments <base>..<head>

semdiff show <fragment-id>

semdiff validate <groups-file>

semdiff view <groups-file>
```

必要なら以下も追加して構いません。

```bash
semdiff context <fragment-id>

semdiff related <fragment-id>
```

Agent Skillから使うため、各コマンドはJSON出力をサポートしてください。

例えば、

```bash
semdiff fragments origin/main..HEAD --json
```

のような形です。

### commits

commit rangeに含まれるcommitの概要を返します。

例:

```json
[
  {
    "sha": "abc123",
    "subject": "Add status support",
    "author": "Alice",
    "timestamp": "2026-08-24T10:00:00Z",
    "files_changed": 8
  }
]
```

### fragments

range全体についてDiffFragment inventoryを返します。

例:

```json
[
  {
    "id": "F001",
    "base_sha": "...",
    "head_sha": "...",
    "path": "server/order.go",
    "old_start": 42,
    "old_lines": 12,
    "new_start": 42,
    "new_lines": 18
  }
]
```

inventoryでは巨大なpatch全文を必ずしも返す必要はありません。

Agentが全diffを一度にコンテキストへ読み込まなくて済むよう、metadata中心の軽い出力にしてください。

### show

指定したDiffFragmentの実際の変更内容を返します。

例:

```bash
semdiff show F001 --json
```

```json
{
  "id": "F001",
  "path": "server/order.go",
  "old_start": 42,
  "old_lines": 12,
  "new_start": 42,
  "new_lines": 18,
  "patch": "..."
}
```

### context

実装する場合、DiffFragment周辺のソースコードを取得します。

Agentが変更箇所だけでは意味を判断できない場合に利用することを想定しています。

### related

実装する場合、あるDiffFragmentに関連しそうな候補を機械的なheuristicで返します。

例えば、

* 同一ファイル
* 同一commitに含まれる変更
* 近接する変更
* testとimplementationらしいファイル名
* import/reference関係

などです。

ただし、Semantic GroupそのものをCLI側で決定する必要はありません。

## 2. Semantic Grouping Agent Skill

Agentに巨大な `git log -p` や全diffを最初から丸ごと渡し、Markdownプロンプトだけで処理させる構成にはしないでください。

**決定的・機械的な処理はCLI、意味判断はAgent**という責務分離にしてください。

Agent SkillをMarkdownで用意し、Agentが `semdiff` CLIを使って段階的に探索するよう指示してください。

### Agentの責務

Agentは以下を担当します。

* 変更全体の意図を理解する
* DiffFragment間の意味的な関連性を判断する
* Semantic Groupを作る
* Group titleを付ける
* Group summaryを書く
* 各DiffFragmentをGroupへ割り当てる
* 必要ならレビュー順序を決める
* 分類困難な変更を適切なfallback groupへ分類する

出力例:

```json
{
  "groups": [
    {
      "id": "introduce-order-status",
      "title": "Introduce OrderStatus",
      "summary": "Introduce the OrderStatus domain concept and expose its core representation.",
      "fragments": [
        {"id": "F001", "description": "Defines the OrderStatus domain type."},
        {"id": "F004", "description": "Adds status to the API representation."},
        {"id": "F017", "description": "Wires status into order construction."}
      ]
    },
    {
      "id": "persist-order-status",
      "title": "Persist OrderStatus",
      "summary": "Store and retrieve OrderStatus through the repository layer.",
      "fragments": [
        {"id": "F002", "description": "Stores status in the repository."},
        {"id": "F009", "description": "Loads status from persisted records."}
      ]
    }
  ]
}
```

### Agentの探索フロー

Skillでは、おおむね以下の手順を指示してください。

```text
1. commit一覧を確認する
2. DiffFragment inventoryを取得する
3. commit message / path / fragment metadataから変更全体の概要を把握する
4. 必要なfragmentだけshowする
5. 必要に応じてcontext / relatedを使う
6. 仮のSemantic Groupを作成する
7. 全fragmentをGroupへ割り当てる
8. groups.jsonをvalidateする
9. 未割当fragmentがあれば再調査する
10. それでも分類不能ならfallback groupへ分類する
11. validate成功後にgroups.jsonを確定する
```

Agentが最初から全patchをコンテキストへ読み込まないことを重視してください。

## Coverageの不変条件

これはMVPで重要な制約です。

**すべてのDiffFragmentが必ずちょうど1つのSemantic Groupへ所属する**ようにしてください。

```text
∀ fragment, exactly one semantic group
```

未所属も重複所属もvalidation errorです。

Agentの判断だけに任せず、`semdiff validate` で機械的に検証してください。

例えば、

```text
87 fragments total
84 assigned
3 unassigned

F031
F052
F086
```

あるいは、

```text
F017 is assigned to multiple groups:
- introduce-order-status
- persist-order-status
```

のように問題を返してください。

Agent Skillには、

```text
validateが成功するまでgroupingを完了扱いにしない
```

というルールを書いてください。

意味的な分類が困難なfragmentを無理に既存Groupへ押し込む必要はありません。

必要に応じて、

* Mechanical changes
* Generated changes
* Incidental cleanup
* Unclassified

などのfallback groupを作って構いません。

## 複数Groupへの意味的な関連

MVPでは、1つのDiffFragmentを複数Groupへ重複所属させないでください。

意味的に複数のconcernへ関係する場合でも、primary membershipは1つにします。

将来的には、

```text
related_group_ids
```

のような補助relationを追加しても構いませんが、MVPでは不要です。

## 3. Web Viewer

Semantic GroupデータとGit repositoryの実際のdiffを読み込み、意味単位で差分をレビューできるローカルWeb Viewerを実装してください。

GitHubの `Files changed` が、

```text
file
  -> diff fragments
```

であるのに対して、このViewerでは、

```text
semantic group
  -> file
      -> diff fragments
```

という階層で表示します。

表示イメージ:

```text
Semantic Changes

▼ 1. Introduce OrderStatus
    Introduces the new domain concept and API representation.

    domain/order.go
    --------------------------------
    - old code
    + new code
    --------------------------------

    api/schema.ts
    --------------------------------
    - old
    + new
    --------------------------------


▼ 2. Persist OrderStatus
    Stores the newly introduced status.

    repository/order.go
    --------------------------------
    ...
```

### Viewerで最低限欲しい機能

MVPでは以下を実装してください。

* Semantic Group一覧
* Group title
* Group summary
* Group単位の展開/折りたたみ
* file path表示
* unified diff表示
* fragment数
* file数
* range全体のbase/head SHA表示
* syntax highlightingが容易なら追加

GitHub API連携やレビューコメント投稿は不要です。

ローカルViewerとして成立すれば十分です。

例えば、

```bash
semdiff view groups.json
```

でHTTP serverを起動し、ブラウザで閲覧できるようにしてください。

必要であればWeb assetsは `embed` packageでバイナリに埋め込んでください。

## groups.json

Agentが生成するSemantic Groupデータのschemaを明確に定義してください。

例:

```json
{
  "version": 1,
  "base_sha": "abc...",
  "head_sha": "def...",
  "groups": [
    {
      "id": "introduce-order-status",
      "title": "Introduce OrderStatus",
      "summary": "Introduce the new domain concept.",
      "order": 1,
      "fragments": [
        {"id": "F001", "description": "Defines the new domain concept."},
        {"id": "F004", "description": "Exposes the concept through the API."}
      ]
    }
  ]
}
```

以下をvalidationしてください。

* `base_sha` / `head_sha` が現在のfragment inventoryと一致する
* Group IDが一意
* fragment IDが存在する
* 新しい `fragments` 形式では各fragmentの `description` が空でない
* すべてのfragmentがちょうど1回だけ出現する
* unknown fragmentがない
* 重複所属がない
* 未所属fragmentがない

既存の `fragment_ids` 形式は後方互換のため読み込み可能ですが、新しく生成する場合は説明付きの `fragments` 形式を使用してください。

## CLIとAgentの責務分離

原則として以下を守ってください。

### CLI / deterministic logic

担当するもの:

* Git command実行
* commit range解析
* diff取得
* DiffFragment抽出
* DiffFragment ID生成
* JSON serialization
* schema validation
* coverage validation
* Viewerへのデータ提供
* 必要なら周辺コードや関連候補の取得

### Agent / semantic reasoning

担当するもの:

* 変更意図の理解
* fragment間の意味的関連性判断
* grouping
* group title
* group summary
* review order
* fallback分類判断

CLI側に高度なLLM的semantic clusteringを実装しないでください。

MVPではAgentを意味判断に集中させてください。

## Git履歴との関係

このMVPはGit履歴を書き換えません。

以下はすべてそのまま維持します。

```text
Git commits
Git commit hashes
branch history
Entire.io checkpoints / provenance
その他既存Git tooling
```

Semantic Groupは完全なderived dataです。

```text
Git repository
       |
       +--> existing Git / Entire workflow
       |
       +--> semdiff
                |
                +--> DiffFragments
                +--> Semantic Groups
                +--> Web Viewer
```

Entire.ioとの直接連携はMVPでは不要です。

## Source commitについて

最終的な `base..head` のDiffFragmentが、必ずしも単一のcommitだけに由来するとは限りません。

そのため、`source_commit_sha` をDiffFragmentの必須フィールドにはしないでください。

必要であれば、Git履歴から推定・参照できるprovenance情報として別途追加できる設計にしてください。

MVPの本質は、最終差分を意味単位へ整理してレビューすることです。

## プロジェクト構成

Go projectとして自然な構成にしてください。

例えば以下は一案です。

```text
cmd/
  semdiff/

internal/
  git/
  diff/
  fragment/
  group/
  validate/
  viewer/

web/
  templates/
  static/

skills/
  semantic-grouping/
    SKILL.md
```

ただし、この構成を機械的に採用する必要はありません。

過剰なpackage分割は避け、MVPとして読みやすい構成を選んでください。

## テスト

最低限、以下をテストしてください。

### DiffFragment extraction

* 1ファイルの単純な変更
* 1ファイル内の複数変更
* 複数ファイル
* file追加
* file削除
* renameを含むケース
* 複数commitを跨ぐrange

### Validation

* 正常なgroups.json
* 未所属fragment
* 重複所属
* unknown fragment ID
* duplicate Group ID
* base/head mismatch

### Viewer

過剰なE2Eテストは不要ですが、少なくとも主要なdata transformationがテスト可能な構造にしてください。

## MVPの実装順序

いきなり完成形を目指さず、まず以下をend-to-endで動かしてください。

```text
Git repository
  ↓
base..head指定
  ↓
DiffFragmentsをJSON生成
  ↓
手書きのgroups.json
  ↓
validate
  ↓
Web ViewerでSemantic Group単位にdiff表示
```

このvertical sliceが動いてからAgent Skillを追加してください。

優先順位は以下です。

1. Go CLI skeleton
2. データモデル
3. Git diff → DiffFragment
4. Group schema
5. validation
6. Web Viewer
7. Agent Skill
8. UX改善

## 完了条件

最低限、以下が動作する状態にしてください。

```bash
semdiff fragments origin/main..HEAD --json
```

でDiffFragment一覧を取得できる。

手書きまたはAgent生成の `groups.json` に対して、

```bash
semdiff validate groups.json
```

がcoverageとschemaを検証できる。

さらに、

```bash
semdiff view groups.json
```

でローカルWeb Viewerが起動し、

```text
Semantic Group
  -> files
      -> DiffFragments
```

の順番で実際の差分を閲覧できる。

また、`skills/` 以下にAgent Skillを用意し、AgentがCLIを使って段階的に変更を探索し、coverageを満たす `groups.json` を生成できるようにしてください。

## 実装方針

まずrepositoryの現状を調査してください。

その上で、MVPを最短で成立させる設計を選び、実装してください。

過剰な抽象化、過剰なフレームワーク導入、将来機能の先行実装は避けてください。

設計判断に迷った場合は、以下を最優先してください。

> Gitの物理履歴を変更せず、最終差分をレビューしやすい意味単位へ再構成する。

そして、

> 機械的に判断できることはGo CLIに任せ、意味的な判断はAgent Skillに任せる。

という責務分離を維持してください。
