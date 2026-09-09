# AGENTS.md

このファイルは、このリポジトリで作業する AI Agent 向けのガイドです。

## プロジェクト概要

`semdiff` は、固定された Git の `base..head` を意味的な Fragment、Step、Group に整理する Go 製 CLI です。CLI は Git から変更事実を取得し、draft の保存、coverage の検証、viewer の生成を担当します。変更の意味やレビュー順は Agent が判断します。

Git の commit や履歴を書き換えるツールではありません。実装時も、この性質を維持してください。

## リポジトリ構成

- `main.go`, `*_commands.go`: CLI の引数解析と各 command の orchestration
- `internal/gitdiff`: Git range、diff、commit の取得と materialize
- `internal/groupingdraft`: 再開可能な grouping draft と operation の適用
- `internal/groups`: 最終的な review model と coverage validation
- `internal/reviews`: local review の探索と artifact branch への保存
- `internal/questions`: viewer の質問 thread と answer session
- `internal/viewer`: interactive viewer と静的 HTML の生成
- `internal/config`: project/local 設定の読み込みと優先順位
- `internal/categories`: path に基づく file category の提案
- `skills/`: `semdiff` CLI を利用する Agent skill
- `docs/design.md`: data model と validation の設計
- `README.md`, `README_ja.md`: user workflow
- `CLI_REFERENCE.md`, `CLI_REFERENCE_ja.md`: CLI reference

## 設計上の不変条件

- Git hunk と semantic Fragment を同一視しない。hunk は候補であり、Fragment の境界は意味に基づいて決める。
- 最終 review では、変更行と file metadata の各 coverage atom をちょうど一つの Fragment が所有する。
- Fragment は一つの Group に所属し、その Group の review Step からちょうど一度参照される。
- patch は派生データである。`groups.json` に保存せず、SHA、path、range から再構築する。
- draft は途中状態を許容する。finalize 時に現在の Git change map で再検証する。
- `.semdiff/` はローカル状態・生成物であり、明示的に求められない限り commit しない。

schema version や既定値を変更するときは、model、load/save、validation、既存 version の拒否または migration、documentation を一体として確認してください。

## 実装方針

- まず変更対象に最も近い package と test を読む。公開されていない `internal` package の責務を保ち、command layer に domain logic を増やさない。
- error は呼び出し元が状況を判断できる形で返し、CLI の終了は `main` に集約する。
- map iteration や Git 出力に依存する結果は、必要に応じて明示的に sort し、再現可能にする。
- filesystem と Git を扱う test は `t.TempDir()` と一時 repository を使い、利用者の working tree や global Git config に依存させない。
- 既存の user-facing behavior を変える場合は、成功経路だけでなく、不正入力、古い schema、coverage gap/overlap、Git failure など関連する失敗経路も test する。
- unrelated な refactor や formatting を混ぜない。既存の未コミット変更は利用者のものとして保持する。

## 検証

Go ファイルを変更したら、対象ファイルへ `gofmt` を適用します。

```sh
gofmt -w <changed-go-files>
go test ./...
git diff --check
```

開発中は対象 package の test を先に実行して構いませんが、完了前には原則として `go test ./...` を実行してください。実行できない場合は、未実行の command と理由を報告します。

CLI の表示、viewer、静的 HTML を変更した場合は、該当 test に加えて実際の出力も確認します。生成した一時ファイルは repository に残しません。

## Documentation と skill

- user-facing な command、flag、既定値、保存先、workflow を変えたら README と CLI reference を更新する。
- 英語版と日本語版は同じ仕様を説明するよう、原則として同じ変更で同期する。
- data model、coverage、draft/finalize の意味を変えたら `docs/design.md` も更新する。
- skill が利用する command や JSON field を変えたら `skills/` 内の該当 `SKILL.md` を確認する。
- documentation は実装と test を根拠に書く。確認できない動機や挙動を推測で追加しない。

## semdiff 自身を使う場合

このリポジトリの変更を semantic grouping するときは `skills/semantic-grouping/SKILL.md` に従います。`groups.json` を直接編集せず、CLI の grouping draft workflow を使って coverage を検証してください。
