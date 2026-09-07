---
name: semantic-grouping
description: semdiff CLI を使って Git のコミット範囲をレビュー向けの意味的な変更単位に分類します。このリポジトリの coverage-complete な groups.json を作成または更新するときに使います。Git の履歴は書き換えません。
---

# Semantic Grouping

`groups.json` を、レビューのために導出されるレイヤーとして作成します。コミットとリポジトリの履歴は保持します。事実の抽出・保存・検証は CLI に任せ、グループ化、タイトル、要約、説明、レビュー順だけに意味的な判断を使います。最終 JSON を直接書かず、再開可能な grouping draft を作って結果を構築します。

## 実行手順

以下の手順を実行し、構成の判断には「分割とレビュー順」、属性の設定には「属性の判断基準」、文章の仕上げには「説明の書き方」を参照します。コマンドの詳細と JSON 例は「CLI 操作と出力形式」、完了前の確認は「最終確認」にまとめています。

作業は「成果と根拠の整理 → Fragment・Group・Step の構成 → 説明の仕上げ → 検証」の順に進めます。構成中の説明は暫定で構いません。境界を見直した後は、関係する title・summary・description も更新し、古い説明を残さないでください。

1. draft を作成する前に `semdiff reviews resolve --json` を実行します。範囲を指定しない場合は `grouping init` と同じ pull request の範囲判定を使います。`found` が false なら `semdiff grouping init --json` で新しい draft を作成します。祖先レビューが見つかった場合（`exact: false`）は、`semdiff grouping init --from <groups_path> --force --json` でそのレビューを元にした現在の範囲の draft を新しく作成します。完全一致するレビューが見つかった場合は、ユーザーがグループ化の見直しを明示的に求めていない限り、その成果物を再利用します。
2. 既存レビューを引き継いだ draft は既存の Groups、summary、レビュー metadata、Fragment 定義を引き継ぎますが、Git の変更マップと suggestions は現在の範囲について必ず再計算されます。`review_head_sha` より後のすべてのコミットと、影響を受けたパスにある既存のすべての Fragment を再確認が必要な対象として扱います。古い行範囲が意味的に正しいとは仮定しないでください。CLI は、source の Base が異なる場合、または Head が現在の first-parent history 上にない場合、その source を拒否します。
3. `grouping init` が返した SHA を使って `semdiff commits <base-sha>..<head-sha> --json` を実行し、変更の経緯を把握します。祖先レビューの場合は、まず `semdiff commits <review_head_sha>..<head-sha> --json` も確認します。完全な diff を最初から読み込まないでください。
4. `semdiff grouping inspect --suggestions --json` を実行し、ファイルと周辺コードを確認しながら suggestions をレビューします。Groups を命名する前に、範囲全体にある独立してレビュー可能な成果を棚卸ししてください。最初に確認したファイル、コミット、suggestions だけで Group の構成を決めないでください。既存レビューを引き継いだ draft では、未グループ化の suggestions と、引き継ぎ元のレビュー後にファイルが変更された Groups を優先します。関連する候補には `semdiff show --draft .semdiff/grouping-draft.json <id> --json` を使います。
5. suggestions から `merge_fragments` を使って authored fragments を構成します。1 つの `member` は意味的に完結した suggestion を昇格し、同じパスの複数の `members` は 1 つの複数範囲 Fragment になります。意図した定義に明示的な範囲が必要な場合は `add_fragment`、`update_fragment`、`delete_fragments` を使います。status に表示され、割り当てが必要なのは authored fragments だけです。
6. 棚卸しの結果から、一貫性のある Groups を作り、`semdiff grouping apply <operations-file|-> --json` で判断をまとめて適用します。Groups の数に望ましい最小値、最大値、典型値はありません。数は意味的な境界の結果として決めます。関心事が変わっていない引き継いだ Group は維持し、新しい変更によって関心事が変わった場合は修正、分割、統合、移動します。複数の関心事に関係する Fragment でも、主たる所属は 1 つだけ割り当てます。Group の `importance` は PR 全体に対する相対的な位置づけとして、削除テストに従い `core`、`supporting`、`side` のいずれかを設定します。Fragment の `review_level` は、レビューでどの程度注意深く読むべきかを示す `careful`、`normal`、`skim` のいずれかにします。明確な理由がない限り `normal` を使います。各 Group の `order` は、前提となる Group が依存する Group より先に来るように設定します。根拠が不十分な場合は `mechanical-changes` や `unclassified` のような、意味の明確な fallback を使います。
7. 新規または修正した各 Fragment には、注目すべき 1 つの変更を表す短い意味的なラベルを書きます。目的、制約、関係が diff から明らかでなく、その点がレビュー判断を変える場合にだけ理由を加えます。Group の Fragment が参照する各ファイルには、`file_categories` の entry をちょうど 1 つ設定します。まず `classify` の出力を使い、コミットの意図、パスの意味、関連する Fragment の内容で確認または修正します。
8. 必要に応じて `status`、`inspect`、`apply` を繰り返します。draft は意図的に未完成でもよいものです。1 回の batch で未割り当ての Fragment が残っただけで停止しないでください。
9. 最終化の前に、各ファイルについて Fragment が不足していないか、細かく分けすぎていないかを確認します。大きな suggestion や新規ファイルでは、トップレベルの責務を確認し、関数、型、handler、test を独立して説明・検討できる場合は `add_fragment` と明示的な範囲を使います。ファイル境界、Git hunk、テストの境界、import、コメント、整形の変更だけを理由に分割しないでください。意味を独立して説明できない Fragment、特に区切り記号だけの Fragment や構文だけの Fragment、隣接する構造を完成させるだけの Fragment は統合します。
10. Fragment を割り当てた後、`set_review_steps` で各 Group の `review_steps` を作成します。Step は以下の実際の依存関係と推論の経路から導き、一般的な setup/implementation/integration/test の構成を使い回さないでください。title は「保存済みレコードを ID で取得できるようにする」のように、具体的な目的・効果を示す動作の表現にします。名詞句、抽象的なラベル、読解の指示は使わないでください。順に並べた title が Group summary を補う変更の概要になるようにします。summary は「Group と Step の summary」に従い、Why → What → So what を基本に書きます。各 Group Fragment は、その Group の Step の中にちょうど 1 回だけ現れる必要があります。
11. `semdiff grouping finalize --json` を実行します。明示的な出力パスがない場合、finalize は結果を Git の追跡対象外の `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` に書き込みます。ユーザーまたは周辺の作業手順から必要とされる場合だけ明示的な path を使います。すべての authored fragment が割り当てられ、説明され、完全な review Step に配置され、すべての Group に完全な summary とファイル分類があり、変更されたすべての行とメタデータ変更がちょうど 1 回選択されている場合にだけ finalize は成功します。

## 分割とレビュー順

### Group の境界

#### 位置付け

Group は、PR の一つの成果について、独立した採否判断の対象となる Fragment の集合です。

#### 原則

成果は、diff の量や配置ではなく、挙動、contract、データの流れ、設定の効果、migration、個別の動機がある cleanup から見つけます。1 つのファイルに独立した成果を実装する複数の Group があってもよく、数十ファイルが 1 つの成果を実現するなら 1 つの Group のままでも構いません。ファイル数、行数、コミット数、suggestion 数を Group 数の代わりに使わないでください。

まず、独立してレビュー可能な成果ごとに候補 Group を作ります。どちらか一方だけをレビューすると不完全または誤解を招き、別々の判断を支えない場合は候補を統合します。次の手掛かりで分割を検討し、下記の優先基準で確定します。

- その部分ごとに、独立した成果として採用・不採用を判断できる。
- 異なる外部観測可能な挙動、contract、データ移行、設定の効果を変更し、失敗の種類も異なる。
- 一方を理解するために必要な分野固有の知識が、他方には必要ない。
- title や summary を正確に保つには、「また」「無関係に」「さらに」のように別の成果を接続する表現が必要になる。

ファイル、package、アーキテクチャの層、commit、ファイル分類が異なることだけを理由に分割しないでください。同じ成果を成立させるために必要な implementation、test、documentation、生成物、呼び出し元の追随、configuration は、そのうちの 1 つに独自のレビュー判断がある場合を除き、同じ Group に保ちます。

判断が競合する場合は、独立した成果として採否を判断できるかを優先します。技術的に別々に revert できること、必要な知識や失敗の種類が違うこと、説明に「また」が入ることは分割を検討する手掛かりであり、それだけでは分割しません。

#### 例

- 同じ Group: 重複 ID の拒否処理と、その正常系・異常系テスト。テストだけ取り除けても、同じ成果を成立・検証する変更です。
- 別 Group: 重複 ID の拒否処理と、既存の日時表示の不具合修正。それぞれの成果を独立して採否判断できます。

### Fragment の境界

#### 位置付け

Fragment は、レビュアーが独立して理解し説明できる最小の変更単位です。一つのファイル内の、一つ以上の行範囲またはファイルのメタデータ変更を表します。

#### 原則

最小の連続した差分範囲ではありません。1 つの range に意味的な変更が完結しているならそれで十分です。離れた編集が 1 つの責務を実装している場合は、複数の range を使います。

ファイル境界と Fragment の境界は別々に扱います。特に、Git が新規ファイル全体を 1 つの addition として表示するからといって、そのファイルを 1 つの Fragment に保たないでください。そのファイルの異なる行範囲が、異なる Group に属する独立してレビュー可能な責務を実装しているなら、別々の Fragment を作成して、それぞれを担当する Group に割り当てます。共有する declaration、import、区切り記号、その他の構造上の行は、それを必要とする責務の Fragment に含めます。独立した意味のない補助構造のために別の Fragment を作らないでください。

次のような変更が、別の変更を完成させるだけなら、単独の Fragment として残さないでください。

- closing brace、bracket、parenthesis、comma、semicolon、その他の punctuation だけの編集。
- dangling な `else`、`catch`、JSX closing tag、その他の構造上の counterpart。
- 隣接する implementation Fragment だけが使う import。
- 独立したレビュー上の意味を持たない declaration/body、caller/callee adaptation、setup/assertion pair の片側。

その range を construct や behavior を所有する Fragment に付けます。description のテストとして、「閉じる」「構文を調整する」「import を追加する」としか書けない、または別の Fragment を曖昧に参照するだけなら統合します。range が近いというだけで統合しないでください。独立してレビュー可能な behavior、test、configuration、refactor は、Git が同じ hunk に置いていても分けます。

### Step の境界

#### 位置付け

Step は、一つの Group の内容を理解するための、順序付きの説明段階です。各 Step は、その Group に属する Fragment を参照します。

#### 原則

Group のすべての Fragment を 1 つの具体的な動作と 1 つのレビュー上の問いで説明できるなら、複数ファイルにまたがっていても 1 つの Step にします。単一ファイルで単一目的の変更なら、1 Group 1 Step になることがよくあります。

次の Step に進む前に役立つ、独立した前提、実装上の段階の切り替わり、正しさを判断する問いを導入する場合だけ、別の Step を作ります。棚卸しの結果を配るために Fragment ごとに Step を作らないでください。同じ問いに答える Step や、title が単なるファイル、層、テストの分類、一般的な工程になっている隣接 Step は統合します。一方、大きな Group では実際の依存関係の順序に必要な数だけ Step を作れます。したがって Step 数は Group 数や PR のサイズに直接比例せず、その Group 内部の推論に必要な量に応じて決まります。

Step を分ける理由は、次の段階を判断するために先に理解すべき前提が変わることです。実装とテストであることだけでは分けません。

#### 例

- 1 Step: 重複 ID を拒否する条件式と、その条件を検証するテスト。同じ「どの入力を拒否するか」という問いで説明できます。
- 複数 Step: 同じ拒否機能でも、まず Repository が返す重複エラーを定義し、次に HTTP handler がそのエラーを 409 応答へ変換する場合。後段の判断には前段のエラーの意味が必要です。各段階のテストは対応する Step に置きます。

### Group のレビュー順

#### 位置付け

Group の `order` は、成果物内で Group を提示する順序を表します。前提となる変更から、それを利用する変更へと読み進めるための属性です。

#### 原則

Group の `order` は、辞書順、ファイル順、Fragment を発見した順序ではなく、レビュアー向けの依存順にします。

Group B が Group A が導入した contract、type、schema、helper、migration、behavior に依存する場合は、A を B より先に置きます。全 Group に対してこの関係を推移的に適用し、まず前提、次に依存する挙動と integration、最後に本当に独立した concern である後続の検証や documentation を置きます。

dependency は import と call site、type と schema の利用、設定を利用する処理、コミットの時系列、意味的な説明から推測します。ディレクトリ配置だけから推測しないでください。test は、それが検証する behavior と同じ Group に保ちます。ただし、test 自体が独立してレビュー可能な concern なら別 Group にし、その場合は behavior の後に置きます。

Group が独立している場合は、文脈の切り替えを最小化し、変更を一貫した変更の経緯として読める順序にします。たとえば共通の基盤を機能固有の利用箇所より先に置き、core または supporting Group を side Group より先に置きます。整然とした順序にするためだけに dependency を作らないでください。最終化の前に、最初から最後まで summary を順に読み、Group が後で初めて導入される知識を必要としていないか確認し、必要なら `order` を修正します。

## 属性の判断基準

### Group の importance

#### 位置付け

Group の `importance` は、PR の目的に対するその Group の位置づけを表します。

#### 原則

Importance は、Group が PR の目的にどう関係するかを示すものであり、リスク、規模、難しさ、機械的な複雑さを示すものではありません。

Group 全体を頭の中で取り除いて分類します。

- `core`: これを取り除くと PR の存在理由がなくなる。
- `supporting`: core の目的は認識できるが、不完全、壊れた状態、説明不足、または未検証になる。
- `side`: core の目的と完全性はおおむね保たれ、PR に同梱された独立した意味のある変更である。

ファイル種別や変更の機械的な性質だけで分類しないでください。core の変更が不完全になるなら、必要な生成物、configuration、test、documentation、rename、整形は `supporting` になりえます。core の変更を弱めない機会的な cleanup や整形は `side` になりえます。`side` は不正またはレビュー不能という意味ではなく、その Group が PR の中心的な目的を完了する一部ではないという意味です。

### Fragment の review_level

#### 位置付け

Fragment の `review_level` は、その変更を読む際に必要な注意の程度を表します。PR の目的に対する位置づけを表す `importance` とは別の属性です。

#### 原則

`importance` と独立に、変更内容を読む際に必要な注意の程度を判定します。

- `careful`: 権限判定、データ消失、永続データの移行、外部 API の互換性、並行処理などについて、誤りの影響が大きい条件や自明でない正しさの判断を含む変更。
- `skim`: 挙動を変えないことが確認できる整形や機械的な追随など、詳細な論理判断がほぼ不要な変更。
- `normal`: 上記に当てはまらない変更。判断材料が足りない場合も既定値とします。

ファイルの種類、行数、新規追加というだけで判定しないでください。テストや生成物でも重要な条件の判断を含めば `careful` になりえます。`careful` を選んだ場合は、その根拠となる変更内容や影響を description または所属 Step に記述します。

### ファイルの file_categories

#### 位置付け

`file_categories` は、Group が参照する各ファイルの、その Group における役割を表します。`classify` は、ファイルパス・名前・拡張子・ディレクトリ構造に基づく分類候補を返すコマンドです。

#### 原則

確信度スコアや意味に基づく判断理由は意図的に持ちません。その出力は draft として扱います。最終 category は、機械的な推測だけでは不十分な場合、コミットから読み取れる変更の経緯と関連コードに基づいて、その Group におけるファイルの役割を表すものにします。

デフォルトの category 用語は、一般的なソースコードに `implementation`、test に `test`、UI コンポーネントに `component`、UI に依存しない logic に `logic`、configuration と依存関係のメタデータに `config`、documentation に `docs`、パスから有用な役割を判断できない場合に `unknown` です。これらは慣例であり enum ではありません。既定の分類で役割が伝わるならその語彙を使います。伝わらない場合だけ自由記述の分類を使い、同じ役割には同じ名称を使います。

## 説明の書き方

### 説明の抽象度

#### 位置付け

説明の抽象度は、具体的なコード操作から設計上の意味まで、変更をどの水準で述べるかを指します。

#### 原則

説明するコードの抽象度に合わせます。

追加の設計上の意味が必要でない限り、具体的な変更をより高いレベルの設計用語に一般化しないでください。

コードに直接対応する用語を優先します。interface、type、method、function、signature、parameter、return value、construction、call、registration、DI、implementation、conversion などです。たとえば interface に method を追加する変更では、単に contract を拡張した、boundary を変更したと書くのではなく、まず「既存の `Repository` interface に、新規メソッド `FindByID` を追加する」と書きます。

設計上の意味が有用なら、具体的なコード変更の後に説明します。`contract`、`boundary`、`ownership`、`wiring`、`responsibility` のような抽象語は、それ自体が議論の対象である場合に使い、直接的な implementation の説明の代わりにはしません。同じ意味なら、形式的・抽象的な表現より正確で直接的な表現を優先します。

Step title は「保存済みレコードを ID で取得できるようにする」のように、具体的な目的・効果を動作の表現で示します。識別子は追跡に役立つ場合に含め、種別や新規／既存の詳細は summary に書きます。「Repository メソッド」「検索の責務」のような名詞句や抽象的なラベルは避けます。

### 識別子の種別と新規／既存

#### 位置付け

識別子の種別は関数・メソッド・型などの区別を、新規／既存はレビュー範囲の Base に存在していたかどうかを表します。

#### 原則

レビュアーは一般的なプログラミング用語を理解している一方、このプロジェクト固有のコードや業務知識には詳しくないと想定します。

Group・Step の summary と Fragment description で、理解に必要な識別子を初めて出すときは、関数、メソッド、型、interface、変数、引数などの種別と、新規／既存の区別を簡潔に明示します。同じ説明の中では繰り返さず、標準ライブラリや変更の理解に不要なローカル変数の解説は増やしません。

種別は名前から推測せず、宣言や呼び出し元を確認します。新規／既存はレビュー範囲の Base と Head を比較して判断します。既存メソッドへの呼び出し追加や、移動・改名を新規メソッドの導入と混同しないでください。既存の型に新規メソッドを追加する場合は、それぞれを区別します。確認できない場合は断定しません。

役割の補足は、名前だけでは読み取れず、変更を理解するために必要な内容に限ります。補足は括弧書きにせず、変更を述べた後の文として続けます。名前の言い換えや interface などの一般的な概念の説明は不要です。

#### 例

- 名前で伝わる例: 既存の `Repository` interface に、新規メソッド `FindByID` を追加する。
- 補足が必要な例: 既存の `ReviewStore` 型に、新規メソッド `Resolve` を追加する。このメソッドは、現在のコミット範囲に一致するレビューを探し、見つからなければ祖先コミットのレビューを再利用候補として返す。
- 既存処理を変更する例: 既存メソッド `ApplyTransition` に、新規引数 `inactive` を追加する。この引数で、状態変更後も処理を停止したままにするかを指定する。

### 日本語の技術用語

#### 位置付け

この節は、日本語の説明文で使う技術用語と表記を扱います。

#### 原則

日本語で書く場合は、日本語のソフトウェア開発で自然な用語と表記を使います。

英語の技術用語を、馴染みのない日本語表現に機械的に翻訳しないでください。特に `contract`、`wiring`、`ownership`、`boundary` を、それぞれ自動的に「契約」「接続」「所有」「境界」と翻訳しないでください。

日本語の技術文書で自然な場合は、`interface`、`API`、`DI`、`Repository`、`Handler`、`Adapter`、`signature` / `シグネチャ`、`middleware` / `ミドルウェア` などの用語を英語またはカタカナのまま使います。すべての用語を翻訳することより、日本のソフトウェアエンジニアにとって正確で自然な表現を優先します。

### Group と Step の summary

#### 位置付け

`summary` は、変更の理由・内容・効果を Markdown の箇条書きで伝える説明です。Group では成果全体を、Step ではその段階を扱います。

#### 原則

Group または Step の `summary` は、長い散文やすべての Fragment の言い換えではなく、簡潔な Markdown の箇条書きです。

各箇条書き項目で 1 つの主張を述べ、レビュアーがすべてのファイルを開かなくても変更の形を把握できるようにします。子箇条書き項目は、親の理由、具体的な詳細、結果、依存関係だけを補足します。通常は入れ子を一段までとし、二段目は依存関係を明確にする場合だけ使います。目標の項目数に達するためだけに箇条書き項目を増やさないでください。重要な主張が 1 つなら 1 箇条書き項目でよく、大きな Group ならそれ以上必要になることがあります。

Group と Step の summary は、原則として次の順番で説明します。

- Why: 対処する問題・制約、またはこの変更が必要になる処理上の理由。
- What: 何をどう変えるか。識別子の種別と新規／既存を示す。
- So what: その結果、何が可能になるか、どの挙動が変わるか、何を防げるか。

Group は成果全体、Step はその段階が必要な理由と後続の処理への効果を扱います。Step ごとに PR 全体の動機を繰り返さないでください。Why・What・So what のラベルを基本にしますが、同じ意味になる項目はまとめて構いません。自明な理由や名前の言い換えで項目を埋めず、説明全体から理由・変更・効果を理解できるようにします。Fragment description は短い説明に保ち、同じ構成を強制しません。

Why は、変更前のどの不足・制約が今回の変更を必要にするかを説明します。「取得する必要がある → 取得処理を追加 → 取得できる」のように同じ内容を三項目に引き延ばさないでください。So what はその不足がどう解消されるかを述べ、What の言い換えだけになるなら What に統合します。

変更の動機と、コード上で果たす役割を区別します。動機が不明な場合は、コードから確認できる必要条件や前後の処理との関係を Why として説明します。それも確認できなければ、Why を捏造して埋めないでください。

単一 Step の Group では内容が重なることを許容しますが、summary 全文を複製しません。Group で問題と成果を説明し、Step では具体的な変更とその効果を短く示します。この場合、Step で既出の Why を繰り返す必要はありません。

背景の主な source として `semdiff commits` を使います。コミットの件名、コミットの本文、時系列、各 commit が変更したファイルを確認します。Fragment の根拠で、実際に実装された内容を検証し、変更の経緯とグループ化した変更を結び付けます。コミット履歴は利用できる context の境界です。commit や code に裏付けのない製品要件、障害、ユーザー報告、設計判断を作らないでください。動機が不明な場合、diff から直接観察できるときに限って、どの制約に対処する変更かを述べます。不確かな解釈は summary に入れないでください。

summary は、ファイル名を列挙したり各 Fragment の description を繰り返したりするのではなく、Fragment 同士の関係を説明します。summary の値は viewer で Markdown を使えます。最上位箇条書き項目には `- `、子箇条書き項目には 2 つのスペースの後に `- ` を使います。JSON では改行を `\n` としてエンコードします。必要ならインラインコードと強調を使いますが、生の HTML には依存しないでください。「なぜなら」「そのため」「これにより」のような明示的な因果関係を保ちます。

#### 例

以下は、既存コードで状態更新が二箇所に分散していることを確認できた場合の例です。

```markdown
- Why: 状態の更新処理が二箇所に分散し、呼び出し元によって更新内容が食い違う可能性がある。
- What: 新規関数 `ApplyTransition` に状態更新をまとめ、既存関数 `SwitchMode` から呼び出す。この処理は、動作モードと停止状態を一緒に更新する。
- So what: 両方の呼び出し元が同じ処理を使い、状態の組み合わせを統一できる。
```

次のような summary は避けます。

`- 古い implementation を command layer に置き換え、cache と test を追加する。`

### Fragment の description

#### 位置付け

Fragment の `description` は、その Fragment が表す一つの具体的な変更についての短い説明です。

#### 原則

Fragment description は、注目すべき 1 つの具体的な変更を短く説明します。

通常は 1 文とし、名前からわからない役割や理由の補足が必要なら、続く文に書きます。ファイル名や path を繰り返したり、「ファイルを更新」「前半を修正」のような作業記録だけで済ませたりしないでください。識別子の種別と新規／既存の区別は明示します。

独立したレビュー上の意味を持たない import、comment、整形、生成物、テストの準備、その他の補助的な編集を列挙しないでください。それらは、それを支える behavior を所有する Fragment に含めます。そのような変更を分ける必要がある場合は、最小限の直接的なラベルを付け、必要に応じて `skim` を使います。

Fragment では What と、その変更の役割・効果が伝わるように書きます。名前で役割が伝わる場合は補足を省きます。Group・Step と違い、Why を独立した項目にする必要はありません。自明でない制約や防ぐ不具合など、理解に必要な理由だけを加え、Group の背景を繰り返さないでください。`skim` でも、理解に必要な補足は残します。根拠のない理由は作りません。

#### 例

次のような description を優先します。

- 既存の `Repository` interface に、新規メソッド `FindByID` を追加する。
- 既存メソッド `CreateCommand.Execute` の呼び出しを追加し、受信した作成リクエストの保存処理を開始する。
- 重複 ID を拒否し、再試行でレコードが二重作成されないようにする。
- 重複 ID のエラーを検証するテーブル駆動テストを追加する。

次のような description は避けます。

- `ファイルを更新。古いコードを削除。`
- `テストファイルを新規追加。関連するテストを追加。`
- `ファイル前半部分を更新。新しい依存関係を追加。`
- `FindByID のための import とコメントを追加。`
- `空IDを拒否する理由を説明するコメントを追加。`

## CLI 操作と出力形式

### Grouping draft の操作

#### 位置付け

Grouping draft は、最終化前のグループ化作業を保持する中間成果物です。CLI を通じて判断を追加・修正し、作業を再開できます。

#### 操作ルール

`semdiff grouping init` は、デフォルトで `.semdiff/grouping-draft.json` にスキーマバージョン 4 の draft を作成します。

`grouping init --from <groups-file>` は、互換性のある過去の最終化済みレビューを引き継ぎながら、現在の範囲の新しい draft を作成します。古い draft を作り直す場合は `grouping init --force` を使います。schema と authored Fragment のフィールドが変わっているため、古い draft をそのまま続けないでください。draft が作業状態であり、finalized v3 の `groups.json` は誤って commit されないよう、デフォルトでは `.semdiff/reviews/` の下に書き込まれます。適用操作は繰り返し実行でき、判断の追加、修正、移動、削除が可能です。複数の draft を管理する場合は `--draft <path>` を使います。

`merge_fragments` は、同じパスの suggestion または authored-fragment ID を 1 つ以上受け取ります。path を導出し、range を連結し、file metadata の ownership を引き継ぎ、authored source を削除し、共有されている Group assignment を保持します。導出された union が意図した選択範囲ではない場合だけ、結果の `fragment` に明示的な range を指定します。

すでに別の Group に属する Fragment には `move_fragments` を使い、残っている authored work のみを確認するには `status` を使います。`add_fragment`、`update_fragment`、`delete_fragments` は定義を編集します。Fragment ID はローカルな handle であり、判断の基準は path/ranges です。`apply` は atomic です。無効な operation があっても、前の draft は変更されません。draft ファイルを手で編集しないでください。

#### 例

apply request は operation の batch を含みます。例:

```json
{
  "operations": [
    {
      "op": "upsert_group",
      "group_id": "repository-lookup",
      "title": "保存済みレコードを ID で取得できるようにする",
      "summary": "- Why: 更新前に保存済みの値と照合する必要があるが、既存の `Repository` interface には取得メソッドがない。\n- What: 新規メソッド `FindByID` を追加し、ID による取得処理を実装する。\n- So what: 呼び出し元で、保存済みの値に基づく更新可否の判断を組み立てられる。",
      "importance": "core",
      "order": 1
    },
    {
      "op": "merge_fragments",
      "members": [
        "F-candidate-1",
        "F-candidate-2"
      ],
      "fragment": {
        "id": "repository-find-by-id",
        "description": "既存の `Repository` interface に、新規メソッド `FindByID` を追加し、取得処理を実装する。",
        "review_level": "normal"
      }
    },
    {
      "op": "assign_fragments",
      "group_id": "repository-lookup",
      "members": [
        "repository-find-by-id"
      ]
    },
    {
      "op": "set_review_steps",
      "group_id": "repository-lookup",
      "review_steps": [
        {
          "id": "repository-method",
          "title": "保存済みレコードを ID で取得できるようにする",
          "summary": "- What: 既存の `Repository` interface とその実装に、新規メソッド `FindByID` を追加する。",
          "fragment_ids": [
            "repository-find-by-id"
          ]
        }
      ]
    },
    {
      "op": "set_file_categories",
      "group_id": "repository-lookup",
      "categories": {
        "src/repository.go": "logic"
      }
    }
  ]
}
```

### 最終 groups.json の形式と必須条件

#### 位置付け

最終成果物は、レビュー対象のコミット範囲と Group・Step・Fragment を記録した version 3 の `groups.json` です。

#### 必須条件

最終成果物は version 3 の `groups.json` です。draft を CLI で最終化して作成します。

すべての Group に `importance` として `core`、`supporting`、`side` のいずれかを設定します。すべての Fragment に `review_level` として `careful`、`normal`、`skim` のいずれかを設定します。draft で省略された値は `normal` が既定値になります。すべての Fragment には `id`、`path`、少なくとも 1 つの `ranges` entry（または `file_metadata: true`）、空でない `description` が必要です。すべての Group には 1 つ以上の `review_steps` が必要で、Group の各 Fragment は順序付き `fragment_ids` の中にちょうど 1 回だけ現れる必要があります。変更されたすべての old/new line とファイルのメタデータ変更は、ちょうど 1 回選択する必要があります。Group が参照するすべてのファイルは、その Group の `file_categories` にちょうど 1 回現れる必要があります。

#### 例

必須の形は次のとおりです。以下は、更新前の照合に取得メソッドが不足していることをコードで確認できた場合の例です。呼び出し元の更新処理の実装まで完了したとは主張しません。

```json
{"version":3,"base_sha":"<full SHA>","head_sha":"<full SHA>","groups":[{"id":"repository-lookup","title":"保存済みレコードを ID で取得できるようにする","summary":"- Why: 更新前に保存済みの値と照合する必要があるが、既存の `Repository` interface には取得メソッドがない。\n- What: 新規メソッド `FindByID` を追加し、ID による取得処理を実装する。\n- So what: 呼び出し元で、保存済みの値に基づく更新可否の判断を組み立てられる。","importance":"core","order":1,"file_categories":[{"path":"src/repository.go","category":"logic"}],"review_steps":[{"id":"repository-method","title":"保存済みレコードを ID で取得できるようにする","summary":"- What: 既存の `Repository` interface とその実装に、新規メソッド `FindByID` を追加する。","fragment_ids":["repository-find-by-id"]}],"fragments":[{"id":"repository-find-by-id","path":"src/repository.go","ranges":[{"old":{"start":10,"lines":4},"new":{"start":10,"lines":7}},{"old":{"start":80,"lines":2},"new":{"start":83,"lines":4}}],"description":"既存の `Repository` interface に、新規メソッド `FindByID` を追加し、取得処理を実装する。","review_level":"normal"}]}]}
```

## 最終確認

最終化の前に、数を目標にせず、実際の境界を監査します。

- 最大の Group について、すべての Fragment が同じ判断を支えているか確認します。そうでなければ分割します。
- 小さい Group や隣接する Group の各ペアについて、本当に異なる判断を支えているか確認します。そうでなければ統合します。
- title が「implementation」「tests」「backend」「cleanup」のような広い分類ではなく、具体的な成果を示しているか確認します。
- 作成した Group を成果の全体棚卸しの結果と比較し、最初に確認した順序が後の関心事を隠していないか確認します。

- コードをまだ読んでいない人の視点で title・summary・description を通読し、理由・変更・効果がつながるか確認します。
- 識別子の種別と新規／既存がわかるか確認します。
- 名前の言い換えや括弧の補足で文章が膨らんでいないか確認します。
