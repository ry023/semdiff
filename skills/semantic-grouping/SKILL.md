---
name: semantic-grouping
description: semdiff CLI を使って Git のコミット範囲をレビュー向けの意味的な変更単位に分類します。このリポジトリの coverage-complete な groups.json を作成または更新するときに使います。Git の履歴は書き換えません。
---

# Semantic Grouping

`groups.json` を、レビューのために導出されるレイヤーとして作成します。コミットとリポジトリの履歴は保持します。事実の抽出・保存・検証は CLI に任せ、グループ化、タイトル、要約、説明、レビュー順だけに意味的な判断を使います。最終 JSON を直接書かず、再開可能な grouping draft を作って結果を構築します。

## 手順

1. draft を作成する前に `semdiff reviews resolve --json` を実行します。範囲を指定しない場合は `grouping init` と同じ pull request の範囲判定を使います。`found` が false なら `semdiff grouping init --json` で新しい draft を作成します。祖先レビューが見つかった場合（`exact: false`）は、`semdiff grouping init --from <groups_path> --force --json` でそのレビューを元にした現在の範囲の draft を新しく作成します。完全一致するレビューが見つかった場合は、ユーザーがグループ化の見直しを明示的に求めていない限り、その成果物を再利用します。
2. seed された draft は既存の Groups、summary、レビュー metadata、Fragment 定義を引き継ぎますが、Git の変更マップと suggestions は現在の範囲について必ず再計算されます。`review_head_sha` より後のすべてのコミットと、影響を受けたパスにある既存のすべての Fragment を再確認が必要な対象として扱います。古い行範囲が意味的に正しいとは仮定しないでください。CLI は、source の Base が異なる場合、または Head が現在の first-parent history 上にない場合、その source を拒否します。
3. `grouping init` が返した SHA を使って `semdiff commits <base-sha>..<head-sha> --json` を実行し、変更の物語を把握します。祖先 source の場合は、まず `semdiff commits <review_head_sha>..<head-sha> --json` も確認します。完全な diff を最初から読み込まないでください。
4. `semdiff grouping inspect --suggestions --json` を実行し、ファイルと周辺コードを確認しながら suggestions をレビューします。Groups を命名する前に、範囲全体にある独立してレビュー可能な成果を棚卸ししてください。最初に確認したファイル、コミット、suggestions だけで Group の構成を決めないでください。seed された draft では、未グループ化の suggestions と、source review 後にファイルが変更された Groups を優先します。関連する候補には `semdiff show --draft .semdiff/grouping-draft.json <id> --json` を使います。
5. suggestions から `merge_fragments` を使って authored fragments を構成します。1 つの `member` は意味的に完結した suggestion を昇格し、同じパスの複数の `members` は 1 つの複数範囲 Fragment になります。意図した定義に明示的な範囲が必要な場合は `add_fragment`、`update_fragment`、`delete_fragments` を使います。status に表示され、割り当てが必要なのは authored fragments だけです。
6. 棚卸しの結果から、一貫性のある Groups を作り、`semdiff grouping apply <operations-file|-> --json` で判断をまとめて適用します。Groups の数に望ましい最小値、最大値、典型値はありません。数は意味的な境界の結果として決めます。関心事が変わっていない inherited Group は維持し、新しい変更によって関心事が変わった場合は修正、分割、統合、移動します。複数の関心事に関係する Fragment でも、主たる membership は 1 つだけ割り当てます。Group の `importance` は PR 全体に対する相対的な位置づけとして、削除テストに従い `core`、`supporting`、`side` のいずれかを設定します。Fragment の `review_level` は、レビューでどの程度注意深く読むべきかを示す `careful`、`normal`、`skim` のいずれかにします。明確な理由がない限り `normal` を使います。各 Group の `order` は、前提となる Group が依存する Group より先に来るように設定します。根拠が不十分な場合は `mechanical-changes` や `unclassified` のような、意味の明確な fallback を使います。
7. 新規または修正した各 Fragment には、注目すべき 1 つの変更を表す短い意味的なラベルを書きます。目的、制約、関係が diff から明らかでなく、その点がレビュー判断を変える場合にだけ理由を加えます。Group の Fragment が参照する各ファイルには、`file_categories` の entry をちょうど 1 つ設定します。まず `classify` の出力を使い、commit intent、パスの意味、関連する Fragment の内容で確認または修正します。
8. 必要に応じて `status`、`inspect`、`apply` を繰り返します。draft は意図的に未完成でもよいものです。1 回の batch で未割り当ての Fragment が残っただけで停止しないでください。
9. 最終化の前に、各ファイルについて Fragment が不足していないか、細かく分けすぎていないかを確認します。大きな suggestion や新規ファイルでは、トップレベルの責務を確認し、関数、型、handler、test を独立して説明・検討できる場合は `add_fragment` と明示的な範囲を使います。ファイル境界、Git hunk、test 境界、import、コメント、formatting の変更だけを理由に分割しないでください。意味を独立して説明できない Fragment、特に delimiter だけの Fragment や syntax だけの Fragment、隣接する構造を完成させるだけの Fragment は統合します。
10. Fragment を割り当てた後、`set_review_steps` で各 Group の `review_steps` を作成します。Step は、以下で説明する実際の依存関係と推論の経路から導きます。一般的な setup/implementation/integration/test の構成を使い回さないでください。各 Step の title は、`Add FindByID to Repository` や `Call CreateCommand.Execute from the Handler` のように、その部分が何をするかを示す簡潔で具体的な action にします。名詞句、design label、読解の指示は使わないでください。順番に並べた Step の title は、Group summary を補う action レベルの outline になる必要があります。summary は簡潔な Markdown outline とし、各 bullet はこの段階について 1 つの主張を述べ、子 bullet はその親の理由、具体的な詳細、結果、依存関係だけを補足します。実装を説明する形で書き、「reviewer が読む」「review する」「見る」よう指示しないでください。日本語で書く場合は、`Repository に FindByID を追加する` や `Handler から CreateCommand.Execute を呼び出す` のような自然な action phrase にします。`Repository メソッド` のような名詞句や、`検索の責務` のような抽象的な label は避けてください。各 Group Fragment は、その Group の Step の中にちょうど 1 回だけ現れる必要があります。
11. `semdiff grouping finalize --json` を実行します。明示的な output path がない場合、finalize は結果を Git-ignored な `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` に書き込みます。ユーザーまたは周辺 workflow から必要とされる場合だけ明示的な path を使います。すべての authored fragment が割り当てられ、説明され、完全な review Step に配置され、すべての Group に完全な summary と file categories があり、変更されたすべての行と metadata change がちょうど 1 回選択されている場合にだけ finalize は成功します。

必須の形は次のとおりです。

```json
{"version":3,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"repository-lookup","title":"Repository に lookup を追加","summary":"- Handler は request を処理する前に既存の record を必要とする。\n  - `FindByID` がその record を提供する。\n- Repository interface に `FindByID` を追加し、lookup を実装する。","importance":"core","order":1,"file_categories":[{"path":"src/repository.go","category":"logic"}],"review_steps":[{"id":"repository-method","title":"Repository に FindByID を追加する","summary":"- `FindByID` が Handler の request 処理前に既存の record を提供する。","fragment_ids":["repository-find-by-id"]}],"fragments":[{"id":"repository-find-by-id","path":"src/repository.go","ranges":[{"old":{"start":10,"lines":4},"new":{"start":10,"lines":7}},{"old":{"start":80,"lines":2},"new":{"start":83,"lines":4}}],"description":"Repository interface に `FindByID` を追加し、lookup を実装する。","review_level":"careful"}]}]}
```

すべての Group に `importance` として `core`、`supporting`、`side` のいずれかを設定します。すべての Fragment に `review_level` として `careful`、`normal`、`skim` のいずれかを設定します。draft で省略された値は `normal` が既定値になります。すべての Fragment には `id`、`path`、少なくとも 1 つの `ranges` entry（または `file_metadata: true`）、空でない `description` が必要です。すべての Group には 1 つ以上の `review_steps` が必要で、Group の各 Fragment は順序付き `fragment_ids` の中にちょうど 1 回だけ現れる必要があります。変更されたすべての old/new line と file metadata change は、ちょうど 1 回選択する必要があります。Group が参照するすべてのファイルは、その Group の `file_categories` にちょうど 1 回現れる必要があります。

## Semantic Group の境界

Group は、PR の 1 つの意味ある成果について、レビュー上の 1 つの判断をともに支える Fragment の集合です。成果は、diff の量や配置ではなく、挙動、contract、data flow、configuration effect、migration、個別の動機がある cleanup から見つけます。1 つのファイルに独立した成果を実装する複数の Group があってもよく、数十ファイルが 1 つの成果を実現するなら 1 つの Group のままでも構いません。ファイル数、行数、コミット数、suggestion 数を Group 数の代わりに使わないでください。

まず、独立してレビュー可能な成果ごとに候補 Group を作ります。どちらか一方だけをレビューすると不完全または誤解を招き、別々の判断を支えない場合は候補を統合します。次のいずれかに当てはまる場合は候補を分割します。

- その部分ごとに異なるレビュー判断を下せる、または独立して revert できる。
- 異なる外部 observable behavior、contract、data migration、configuration effect を変更し、failure mode も異なる。
- 一方を理解するために必要な domain context が、他方には必要ない。
- title や summary を正確に保つには、「また」「無関係に」「さらに」のように別の成果を接続する表現が必要になる。

ファイル、package、architectural layer、commit、file category が異なることだけを理由に分割しないでください。同じ成果を成立させるために必要な implementation、test、documentation、generated output、caller adaptation、configuration は、そのうちの 1 つに独自のレビュー判断がある場合を除き、同じ Group に保ちます。

最終化の前に、数を目標にせず、実際の境界を監査します。

- 最大の Group について、すべての Fragment が同じ判断を支えているか確認します。そうでなければ分割します。
- 小さい Group や隣接する Group の各ペアについて、本当に異なる判断を支えているか確認します。そうでなければ統合します。
- title が「implementation」「tests」「backend」「cleanup」のような広い分類ではなく、具体的な成果を示しているか確認します。
- 作成した Group を成果の全体 inventory と比較し、最初に確認した順序が後の関心事を隠していないか確認します。

## Review Step の境界

Step は、Group を理解し判断するための順序付きの段階であり、quota でも小さな Group でもありません。Group のすべての Fragment を 1 つの具体的な action と 1 つのレビュー上の問いで説明できるなら、複数ファイルにまたがっていても 1 つの Step にします。単一ファイルで単一目的の変更なら、1 Group 1 Step になることがよくあります。

次の Step に進む前に役立つ、独立した prerequisite、implementation transition、correctness question を導入する場合だけ、別の Step を作ります。inventory を配るために Fragment ごとに Step を作らないでください。同じ問いに答える Step や、title が単なるファイル、layer、test category、generic phase になっている隣接 Step は統合します。一方、大きな Group では実際の dependency path に必要な数だけ Step を作れます。したがって Step 数は Group 数や PR のサイズに直接比例せず、その Group 内部の推論に必要な量に応じて決まります。

## Group の importance

Importance は、Group が PR の目的にどう関係するかを示すものであり、risk、規模、難しさ、mechanical complexity を示すものではありません。Group 全体を頭の中で取り除いて分類します。

- `core`: これを取り除くと PR の存在理由がなくなる。
- `supporting`: core の目的は認識できるが、不完全、壊れた状態、説明不足、または未検証になる。
- `side`: core の目的と完全性はおおむね保たれ、PR に同梱された独立した意味のある変更である。

file type や変更の機械的な性質だけで分類しないでください。core の変更が不完全になるなら、必要な generated output、configuration、test、documentation、rename、formatting は `supporting` になりえます。core の変更を弱めない機会的な cleanup や formatting は `side` になりえます。`side` は不正またはレビュー不能という意味ではなく、その Group が PR の中心的な目的を完了する一部ではないという意味です。

## Grouping draft

`semdiff grouping init` は、デフォルトで `.semdiff/grouping-draft.json` に schema version 4 の draft を作成します。`grouping init --from <groups-file>` は、互換性のある過去の finalized review を引き継ぎながら、現在の範囲の新しい draft を作成します。古い draft を作り直す場合は `grouping init --force` を使います。schema と authored Fragment の field が変わっているため、古い draft をそのまま続けないでください。draft が作業状態であり、finalized v3 の `groups.json` は誤って commit されないよう、デフォルトでは `.semdiff/reviews/` の下に書き込まれます。Apply operations は繰り返し実行でき、判断の追加、修正、移動、削除が可能です。複数の draft を管理する場合は `--draft <path>` を使います。

apply request は operation の batch を含みます。例:

```json
{
  "operations": [
    {"op":"upsert_group","group_id":"repository-lookup","title":"Repository に lookup を追加","summary":"- Handler は request を処理する前に既存の record を必要とする。\n  - `FindByID` がその record を提供する。\n- Repository interface に `FindByID` を追加し、lookup を実装する。","importance":"core","order":1},
    {"op":"merge_fragments","members":["F-candidate-1","F-candidate-2"],"fragment":{"id":"repository-find-by-id","description":"Repository interface に `FindByID` を追加し、lookup を実装する。","review_level":"careful"}},
    {"op":"assign_fragments","group_id":"repository-lookup","members":["repository-find-by-id"]},
    {"op":"set_review_steps","group_id":"repository-lookup","review_steps":[{"id":"repository-method","title":"Repository に FindByID を追加する","summary":"- `FindByID` が Handler の request 処理前に既存の record を提供する。","fragment_ids":["repository-find-by-id"]}]},
    {"op":"set_file_categories","group_id":"repository-lookup","categories":{"src/repository.go":"logic"}}
  ]
}
```

`merge_fragments` は、同じパスの suggestion または authored-fragment ID を 1 つ以上受け取ります。path を導出し、range を連結し、file metadata の ownership を引き継ぎ、authored source を削除し、共有されている Group assignment を保持します。導出された union が意図した選択範囲ではない場合だけ、結果の `fragment` に明示的な range を指定します。

すでに別の Group に属する Fragment には `move_fragments` を使い、残っている authored work のみを確認するには `status` を使います。`add_fragment`、`update_fragment`、`delete_fragments` は定義を編集します。Fragment ID はローカルな handle であり、source of truth は path/ranges です。`apply` は atomic です。無効な operation があっても、前の draft は変更されません。draft file を手で編集しないでください。

## Semantic Fragment の境界

Fragment は、reviewer が独立して理解し説明できる最小の変更です。最小の連続した diff span ではありません。1 つの range に意味的な変更が完結しているならそれで十分です。離れた編集が 1 つの責務を実装している場合は、複数の range を使います。

file boundary と Fragment boundary は別々に扱います。特に、Git が新規ファイル全体を 1 つの addition として表示するからといって、そのファイルを 1 つの Fragment に保たないでください。そのファイルの異なる行範囲が、異なる Group に属する独立してレビュー可能な責務を実装しているなら、別々の Fragment を作成して、それぞれを担当する Group に割り当てます。共有する declaration、import、delimiter、その他の structural line は、それを必要とする責務の Fragment に含めます。独立した意味のない scaffolding のために別の Fragment を作らないでください。

次のような変更が、別の変更を完成させるだけなら、単独の Fragment として残さないでください。

- closing brace、bracket、parenthesis、comma、semicolon、その他の punctuation だけの編集。
- dangling な `else`、`catch`、JSX closing tag、その他の構造上の counterpart。
- 隣接する implementation Fragment だけが使う import。
- 独立したレビュー上の意味を持たない declaration/body、caller/callee adaptation、setup/assertion pair の片側。

その range を construct や behavior を所有する Fragment に付けます。description のテストとして、「閉じる」「syntax を調整する」「import を追加する」としか書けない、または別の Fragment を曖昧に参照するだけなら統合します。range が近いというだけで統合しないでください。独立してレビュー可能な behavior、test、configuration、refactor は、Git が同じ hunk に置いていても分けます。

`classify` command は、file path、name、extension、directory structure だけを使います。confidence score や semantic rationale は意図的に持ちません。その出力は draft として扱います。最終 category は、機械的な推測だけでは不十分な場合、commit narrative と関連コードに基づいて、その Group におけるファイルの役割を表すものにします。

デフォルトの category vocabulary は、一般的な source code に `implementation`、test に `test`、UI component に `component`、UI に依存しない logic に `logic`、configuration と dependency metadata に `config`、documentation に `docs`、パスから有用な役割を判断できない場合に `unknown` です。これらは慣例であり enum ではありません。必要なら、より正確な free-form category を使います。

## 技術的な説明の書き方

### 説明の抽象度

説明するコードの abstraction level に合わせます。追加の design 上の意味が必要でない限り、具体的な変更をより高いレベルの design language に一般化しないでください。

コードに直接対応する vocabulary を優先します。interface、type、method、function、signature、parameter、return value、construction、call、registration、DI、implementation、conversion などです。たとえば interface に method を追加する変更では、単に contract を拡張した、boundary を変更したと書くのではなく、まず `Repository interface に FindByID を追加する` と書きます。

design の意味が有用なら、具体的なコード変更の後に説明します。`contract`、`boundary`、`ownership`、`wiring`、`responsibility` のような抽象語は、それ自体が議論の対象である場合に使い、直接的な implementation の説明の代わりにはしません。同じ意味なら、形式的・抽象的な表現より正確で直接的な表現を優先します。

Step title も同じルールに従います。category や抽象的な design concept ではなく、具体的な action を命名します。日本語では、`Repository に FindByID を追加する` や `Handler から CreateCommand.Execute を呼び出す` のような自然な action phrase にします。`Repository メソッド` のような名詞句や、`検索の責務` のような抽象 label は避けてください。

### 日本語の技術用語

日本語で書く場合は、日本語の software development で自然な vocabulary と notation を使います。英語の technical term を、馴染みのない日本語表現に機械的に翻訳しないでください。特に `contract`、`wiring`、`ownership`、`boundary` を、それぞれ自動的に「契約」「接続」「所有」「境界」と翻訳しないでください。

日本語の technical writing で自然な場合は、`interface`、`API`、`DI`、`Repository`、`Handler`、`Adapter`、`signature` / `シグネチャ`、`middleware` / `ミドルウェア` などの term を英語または katakana のまま使います。すべての term を翻訳することより、日本の software engineer にとって正確で自然な wording を優先します。

## Fragment の description

Fragment description は短い label であり、小さな summary ではありません。通常は、注目すべき 1 つの具体的な code change を 1 つの clause または sentence で示します。file name や path を繰り返さず、「更新した」「追加した」「削除した」「新規ファイル」「前半」のような bookkeeping から始めないでください。その context は data structure と viewer がすでに示しています。

独立したレビュー上の意味を持たない import、comment、formatting、generated output、test setup、その他の supporting mechanics を列挙しないでください。それらは、それを支える behavior を所有する Fragment に含めます。そのような変更を分ける必要がある場合は、最小限の直接的な label を付け、必要に応じて `skim` を使います。

Why は任意であり、常に書くものではありません。validation rule、わかりにくい caller adaptation、変更が防ぐ regression など、レビュー判断に理由が必要で、その理由が diff から明らかでない場合だけ追加します。利用できる evidence が裏付けていない rationale を作らないでください。`skim` Fragment では、誤解を避けるために不可欠な場合を除いて why を省略します。

次のような description を優先します。

- `Repository interface に FindByID を追加する。`
- `create Handler から CreateCommand.Execute を呼び出す。`
- `duplicate ID を拒否し、retry で 2 つ目の record が作成されないようにする。`
- `duplicate-ID error の table-driven test を追加する。`

次のような description は避けます。

- `ファイルを更新。古いコードを削除。`
- `テストファイルを新規追加。関連するテストを追加。`
- `ファイル前半部分を更新。新しい依存関係を追加。`
- `FindByID のための import とコメントを追加。`
- `空IDを拒否する理由を説明するコメントを追加。`

## Group のレビュー順

Group の `order` は、alphabetical order、file order、Fragment を発見した順序ではなく、reviewer 向けの dependency order にします。Group B が Group A が導入した contract、type、schema、helper、migration、behavior に依存する場合は、A を B より先に置きます。全 Group に対してこの関係を推移的に適用し、まず prerequisite、次に dependent behavior と integration、最後に本当に独立した concern である follow-up validation や documentation を置きます。

dependency は import と call site、type と schema の利用、configuration consumer、commit chronology、semantic description から推測します。directory layout だけから推測しないでください。test は、それが検証する behavior と同じ Group に保ちます。ただし、test 自体が独立してレビュー可能な concern なら別 Group にし、その場合は behavior の後に置きます。

Group が独立している場合は、context switching を最小化し、変更を一貫した narrative として読める順序にします。たとえば shared foundation を feature-specific use より先に置き、core または supporting Group を side Group より先に置きます。整然とした順序にするためだけに dependency を作らないでください。最終化の前に、最初から最後まで summary を順に読み、Group が後で初めて導入される知識を必要としていないか確認し、必要なら `order` を修正します。

## Group と Step の summary

Group または Step の `summary` は、prose narrative やすべての Fragment の言い換えではなく、簡潔な Markdown outline です。各 bullet で 1 つの主張を述べ、reviewer がすべてのファイルを開かなくても変更の形を把握できるようにします。子 bullet は、親の理由、具体的な詳細、結果、依存関係だけを補足します。通常は 1 level の nesting を優先し、2 level 目は本当の dependency を明確にする場合だけ使います。target count に達するためだけに bullet を増やさないでください。重要な主張が 1 つなら 1 bullet でよく、大きな Group ならそれ以上必要になることがあります。

Group summary は通常、evidence がある場合の background や limitation、具体的な implementation change、結果としての behavior、test、review 上の意味を必要に応じて扱います。Step summary は、その stage の役割と隣接する Step との直接的な関係だけを扱います。Fragment description は短い 1 行の label のままで、outline にはしません。

background の主な source として `semdiff commits` を使います。commit subject、commit body、chronology、各 commit が変更したファイルを確認します。fragment evidence で、実際に実装された内容を検証し、narrative と grouped change を結び付けます。commit history は利用できる context の境界です。commit や code に裏付けのない product requirement、incident、user report、design decision を作らないでください。motivation が不明な場合、diff から直接観察できるときに限って、どの limitation に対処する変更かを述べます。不確かな解釈は summary に入れないでください。

summary は、ファイル名を列挙したり各 Fragment の description を繰り返したりするのではなく、Fragment 同士の関係を説明します。summary の値は viewer で Markdown を使えます。top-level bullet には `- `、child bullet には 2 つの space の後に `- ` を使います。JSON では改行を `\\n` として encode します。必要なら inline code と emphasis を使いますが、raw HTML には依存しないでください。「なぜなら」「そのため」「これにより」のような明示的な因果関係を保ちます。

次のような summary の形を優先します。

```markdown
- State transition が direct mutation に分散していた。
  - call site 間で関連する update が分岐する可能性があった。
- `ApplyTransition(command)` を追加し、`SwitchMode` に inactive state を渡す。
  - 両方の call site が同じ state update を使うようになる。
- `ApplyTransition` を通じて各 supported mode を test する。
```

次のような summary は避けます。

`- 古い implementation を command layer に置き換え、cache と test を追加する。`
