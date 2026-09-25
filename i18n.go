package main

import (
	"os"
	"strings"
)

func japaneseLocale() bool {
	lang := strings.ToLower(os.Getenv("LANG"))
	if lang == "ja" {
		return true
	}
	if !strings.HasPrefix(lang, "ja") || len(lang) < 3 {
		return false
	}
	return strings.ContainsRune("_.-@", rune(lang[2]))
}

func localized(english, japanese string) string {
	if japaneseLocale() {
		return japanese
	}
	return english
}

var commandHelpJA = map[string]string{
	"Git range to inspect.": "調べる Git の範囲。",
	"Print JSON output.":    "JSON 形式で出力します。",
	"Read a grouping draft instead of a finalized review.":            "確定済みレビューの代わりに grouping draft を読みます。",
	"Draft used to locate the default groups file.":                   "既定の groups file を特定するための draft。",
	"Viewer listen address.":                                          "Viewer の待受アドレス。",
	"Write a self-contained HTML file instead of serving the viewer.": "サーバを起動せず、自己完結型の HTML ファイルを書き出します。",
	"Include answered questions in an HTML export.":                   "HTML 出力に回答済みの質問を含めます。",
	"Use a draft to locate the groups file.":                          "draft から groups file を特定します。",
	"Require a finalized review for the current range.":               "現在の範囲と完全一致する確定済みレビューを必須にします。",
	"Grouping draft path.":                                            "grouping draft のパス。",
	"Finalized groups file used to seed this draft.":                  "この draft の元にする確定済み groups file。",
	"Replace an existing draft.":                                      "既存の draft を置き換えます。",
	"Show unassigned fragments.":                                      "未割り当ての Fragment を表示します。",
	"Show Git-derived fragment suggestions.":                          "Git から導出した Fragment 候補を表示します。",
	"Show one group.":                                                 "指定した Group を表示します。",
	"Show one fragment.":                                              "指定した Fragment を表示します。",
	"Answer session ID.":                                              "回答セッション ID。",
	"Read answer from stdin.":                                         "標準入力から回答を読みます。",
	"Listen address.":                                                 "待受アドレス。",
	"Git remote name.":                                                "Git remote の名前。",
	"Artifact repository URL or path.":                                "成果物 repository の URL またはパス。",
	"Artifact branch.":                                                "成果物 branch。",
	"Overwrite an existing groups file without asking.":               "確認せずに既存の groups file を上書きします。",
	"Fail if the groups file exists.":                                 "groups file が既にあればエラーにします。",
	"Groups file to publish.":                                         "共有する groups file。",
	"Require a review for the exact current range.":                   "現在の範囲と完全一致するレビューを必須にします。",
}

const usageJA = `semdiff は固定した Git の範囲を意味に基づくレビュー Group に整理します。

使い方:
  semdiff <command> [arguments] [flags]
  semdiff --help

レビューの作成と閲覧:
  grouping init [<base>..<head>]       Git の変更から draft を作成します。
  grouping inspect --suggestions      draft の候補を調べます。--unassigned、
                                      --group、--fragment も指定できます。
  grouping apply <operations-file|->  draft の操作を適用します。- は標準入力です。
  grouping status                     draft の進捗と coverage を確認します。
  grouping finalize [<groups-file>]   検証して groups.json をローカルに保存します。
  view [<groups-file>]                 ローカルレビューの Viewer を起動します。
  view --html <path>                   自己完結型の HTML を出力します。
  resolve [<base>..<head>]            完全一致または最も近い互換レビューを探します。
                                      --exact で祖先レビューへの fallback を禁止します。

Git の変更とレビュー内容の確認:
  commits <base>..<head>              範囲内の commit を一覧表示します。
  fragments <base>..<head>            Git 由来の Fragment 候補を一覧表示します。
  classify <base>..<head>             path に基づく file category を提案します。
  show [<groups-file>] <fragment-id>  確定済み Fragment と patch を表示します。
                                      --draft <path> で draft を読みます。
  validate [<groups-file>]           確定済みレビューを Git と照合します。

Viewer の質問への回答:
  questions session start            回答セッションを開始します。
  questions wait                     未回答の質問を待ちます。
  questions answer <id> --stdin       標準入力から回答を登録します。

Git の成果物 branch でレビューを共有:
  remote view-index                   リモートレビューの HTML 一覧を表示します。
  remote view [<base>..<head>]        保存せずにリモートレビューを表示します。
  remote pull [<base>..<head>]        リモートレビューをローカルに保存します。
                                      上書き時は確認します（--force/--no-clobber）。
  remote push [<base>..<head>]        ローカルレビューを共有します。省略時は
                                      現在の draft、明示指定には --groups-file を使います。

その他:
  --version                           CLI のバージョンを 1 行で表示します。
  version [--json]                    CLI と groups schema のバージョンを表示します。

範囲を使う command では、省略時に現在の PR または remote の default branch
を使います。remote push は省略時に現在の draft を使います。
draft は .semdiff/、確定済みレビューは .semdiff/reviews/ に保存されます。
対応する command では --json を使えます。--draft <path> で draft を、
--remote/--repository/--branch でリモートの保存先を指定できます。`
