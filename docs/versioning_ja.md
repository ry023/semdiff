# バージョン管理

semdiffは製品とartifactを別々にバージョン管理します。CLIと同梱pluginは一つのrelease単位です。`groups.json` は生成したbinaryより長く残る可能性があるため、独立したschema versionを持ちます。

## 製品release

rootの `VERSION` がCLIとplugin versionのsource of truthです。Codex manifest、Claude manifest、Claude marketplace entryはこの値と一致させます。Git release tagには `v` prefixを付けるため、製品version `0.3.0` のtagは `v0.3.0` です。

製品versionが `1.0.0` 未満の間は、patch releaseを互換な修正、minor releaseを機能追加または破壊的変更に使用します。`1.0.0` 以降は通常のSemVerのmajor/minor/patch互換性規則に従います。現在はprereleaseやbuild metadataを含まないrelease-onlyの `MAJOR.MINOR.PATCH` だけを公開します。

pluginとCLIはrelease時に同じversionを使用します。pluginは同じminor系列の任意のCLI patch releaseを利用できます。そのためplugin `0.3.x` はCLI `>=0.3.0, <0.4.0` を受け入れます。

## 製品releaseの自動化

`main` のrelease workflowは [tagpr](https://github.com/Songmu/tagpr)を使い、未releaseの変更に対するrelease pull requestを作成・更新します。`.tagpr` には `VERSION` と三つのplugin manifestをversion fileとして指定しているため、release pull requestの中でCLIと同梱pluginのversionが同期されます。pull requestのlabelでminorまたはmajor bumpを選べます。指定がなければtagprはpatch releaseを提案します。

release pull requestをmergeすると `v<VERSION>` tagと、tagprが生成したRelease Noteを含むDraftのGitHub Releaseが作られます。同じworkflow内でtagprのtag outputをGoReleaserに渡し、GoReleaserはそのDraftを引き継いで複数platform向けarchiveと `checksums.txt` を添付し、公開します。GoReleaserの前にDraft ReleaseとそのRelease Noteが存在する必要があるため、release tagはtagprのrelease pull request経由で作成します。

Homebrewの公開設定はまだ含めず、別の変更で追加する予定です。

## `groups.json` schema

finalized reviewはformatとschema versionを宣言します。

```json
{
  "format": "semdiff.groups",
  "format_version": "1.0.0"
}
```

schema versionは製品versionから独立しています。schema major releaseは非互換、minor releaseは後方互換なformat追加、patch releaseはwire contractを変更しない訂正です。readerは明示的に検証したminor系列だけを受け入れ、未知の将来minorを安全だと仮定しません。CLIはartifact全体のdecodeやGit状態の読み取りより先にenvelopeを検査します。

| CLI/plugin | 読み取れる `groups.json` | 書き出す `groups.json` |
|---|---|---|
| `0.3.x` | `semdiff.groups >=1.0.0, <1.1.0` | `semdiff.groups 1.0.0` |

数値の `version` fieldは従来formatです。CLI `0.3.x` はこのartifactをmigrationも読み取りもしません。再生成するか、artifactを作成した旧unversioned CLIを使用してください。

draft schema version 4、question file version 2、answer session version 1はローカル状態のformatです。製品versionおよびfinalized artifact schemaとは独立したまま管理します。

## Release checklist

1. schema対応を変える場合は、互換表とCLIのschema read/write定数を更新します。
2. 対応するCLI minor系列が変わる場合は、同梱する両skillを更新します。
3. `gofmt`、`go test ./...`、plugin validation、`git diff --check` を実行します。
4. tagprのrelease pull requestをmergeします。`VERSION` とplugin manifestが更新され、`v<VERSION>` tagとRelease Note付きのDraft Releaseが作られます。その後GoReleaserがartifactを添付して公開します。
