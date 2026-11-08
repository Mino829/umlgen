# GitHub ActionsでPull Request差分図と変更後の全体図を生成する

umlgenの再利用可能ワークフローを呼び出すと、Pull Requestのbase／headからJavaの差分クラス図を、PRのhead commitから変更後の全体クラス図を生成し、それぞれPlantUMLとSVGをActions artifactとして保存できる。

差分図の読み方、精度、ローカルCLIの使い方については[`docs/pr-diff.md`](./pr-diff.md)を参照してください。

## 最小構成

利用するリポジトリに`.github/workflows/umlgen-diff.yml`を作成する。

```yaml
name: UML diff

on:
  pull_request:
    types: [opened, reopened, synchronize, edited]

permissions:
  contents: read

jobs:
  umlgen-diff:
    permissions:
      contents: read
    uses: Mino829/umlgen/.github/workflows/umlgen-pr-diff.yml@main
    with:
      source: src/main/java
```

導入直後の試用では`@main`を利用できる。本運用では、内容が変化しないumlgenのリリースタグまたは完全なcommit SHAへ固定する。

実行後、Actionsのrun summaryに適用条件・比較commit・本文hashを表示し、Artifactsから`umlgen-pr-diff`をダウンロードする。内容は次の5ファイル。

```text
change-diagram.puml
change-diagram.svg
full-diagram.puml
full-diagram.svg
review-scope.json
```

`change-diagram`は変更型とその周辺の差分図、`full-diagram`はPRのhead commitにある対象パスのJava型の全体図です（`.umlgen.yaml`の除外設定は適用されます）。全体図に変更色や削除された型は含まれません。全体図はmerge commitではなくPRのhead commitから生成します。

## 入力

| Input | Default | 説明 |
| --- | --- | --- |
| `source` | `.` | リポジトリ相対のJavaソースパス（このワークフローは`.java`ファイルの変更を検知します） |
| `depth` | `1` | 変更型から含める関係の深さ |
| `direction` | `both` | 関係の探索方向：`in`、`out`、`both` |
| `umlgen-version` | `v0.5.0` | ダウンロードするumlgenのリリースタグ |
| `artifact-name` | `umlgen-pr-diff` | artifact名 |
| `retention-days` | `14` | artifact保持日数 |
| `base-sha` | Pull Requestのbase | base commitの上書き |
| `head-sha` | Pull Requestのhead | head commitの上書き |

`source`以下に変更されたJavaファイルがない場合も失敗にはしない。その場合は差分図に「Java変更なし」と記載したPlantUML／SVGを保存し、全体図は通常どおりheadのソースから生成する。headで対象パスのJavaソースがすべて削除された場合は、全体図に「Javaソースなし」と記載したPlantUML／SVGを保存する。

なお、このワークフローは`.java`ファイルの変更のみを検知する。Goプロジェクトで利用する場合はローカルCLIまたは独自ワークフローで`umlgen diff`を呼び出してください。

## PR本文でレビュー範囲を指定する（開発版）

対応するworkflowのcommit／リリースを使うと、PR本文の専用blockで**差分図だけ**の範囲を指定できる。呼び出し側の`pull_request.types`には上の例のように`edited`を含める。旧workflowの参照先を更新しない場合、この設定は適用されない。

````markdown
### 今回レビューしたい範囲

```umlgen
--include "com.example.order"
--relations inheritance,implementation,field
--depth 2
--direction out
```
````

| 引数 | 受理する値 |
| --- | --- |
| `--include` | Javaのpackage prefixを**1つ**。Unicode識別子も可。UTF-8で1024 bytes以下。複数prefix・CSV・glob・ファイルパスは不可 |
| `--relations` | `inheritance,implementation,field,parameter,return`の一意な部分集合、または`all`単独 |
| `--depth` | `0`〜`10`の整数。符号・小数・指数・先頭0は禁止。`0`は変更型のみ |
| `--direction` | `in`／`out`／`both` |

- `--flag value`と`--flag=value`を使える。single／double quote、backslash escape、行継続を扱うが、変数・command substitution・シェル構文は実行しない。
- 対象はinfo stringが正確に`umlgen`のfenced blockが1つ。backtick／tildeは3個以上、indentは3 spacesまで。長い外側fenceに入った記述例は設定にしない。別のinfo stringやinline codeも対象外。
- blockなしは従来どおり、`with.depth`／`with.direction`を使い、include／relationsはCLIとrepository設定の既定を維持する。block内で指定した値だけが優先する。信頼済みの`with.depth`には本文用の最大10を適用しない。
- 空block・閉じ忘れ・複数block・同じ引数の重複・未対応の引数／値は、checkoutやdownloadの前に明示エラーにする。無指定の図へfallbackしない。
- 本文は256 KiB、blockは8 KiB、tokenは32個まで。検証jobは5分、図生成は15分の期限を設ける。
- `--config`／`--output`／`--source`／`--language`／`--renderer`／`--server-url`／`--focus`等は受け付けない。source、比較commit、CLI版、出力先、rendererはworkflow管理のまま。

`--relations`は線の種類を選ぶ。配布版CLIの近傍選択により、選択しなかった関係の周辺型が文脈として残る場合がある。package prefixやdepthによる型の範囲とは区別する。

全体図の`full-diagram`へ本文フィルタは適用しない。こちらは従来どおりheadの全体文脈で、repository設定は有効。差分図と全体図を同じレビュー条件の図として解釈しない。

`review-scope.json`には、渡した引数、本文／workflow既定の別、base・比較merge-base・headの完全SHA、source path、CLI版、本文のSHA-256、run ID／attempt、生成status、Java変更なしの判定を記録する。本文そのものは保存しない。指定していないinclude／relationsのCLI・repository既定まで解決した公開APIではない。

同じrepository・caller workflow・PR・artifact名の新しいrunが古いrunを取り消す。異なる図を同時生成する場合は`artifact-name`を分ける。caller側で同名のconcurrency groupを重ねて設定しない。artifactとSummaryはrun・commit・本文hashに結び付き、以前のrunのartifactは履歴であって「常に最新」のURLではない。

失敗時にはSummaryとログへ診断を残し、成功した図のartifactとしてuploadしない。本文検証に失敗した場合は生成job自体を開始しない。本文への自動書き戻し・PRコメント投稿は行わない。

## 出力

再利用ワークフローは次のoutputを返す。

- `artifact-url`: 差分図と変更後の全体図を収めたartifactのURL
- `no-java-changes`: 選択したパスにJava変更がなければ`true`

呼び出し側で後続jobから参照する場合は、呼び出しjobへ`id`を付けるか、job outputとして引き継ぐ。

## セキュリティ

- `pull_request`イベントと`contents: read`だけを使用する
- Fork Pull Requestではread-onlyの`GITHUB_TOKEN`で動作し、secretを要求しない
- `pull_request_target`は使用しない
- checkout時に認証情報を保持しない
- umlgenのLinuxアーカイブは公開リリースの`SHA256SUMS.txt`で検証する
- PlantUML 1.2026.6のJARをMaven Centralから取得し、固定したSHA-256で検証してからrunner内でSVGを生成する。Ubuntuの`plantuml`パッケージ（1.2020.2）はJava annotationの宣言を描画できないため使用しない
- ソースコードを外部の図生成サービスへ送信しない
- 本文の解析はcheckoutしない検証job内の固定・SHA pinされたgithub-scriptで行い、event JSONをデータとして読む。PR内のスクリプトを解析helperとして実行しない
- 許可した値だけをJSONとしてjob間へ渡し、NUL区切りの引数配列からCLIを呼ぶ。本文や引数文字列をshellへ再評価しない
- このワークフローではSVG出力に固定している。PNG出力やPlantUML Serverレンダラーを利用する場合はローカルCLIで`umlgen diff`を実行する

GitHubの設定によっては、初回Fork Pull Requestのworkflow実行にメンテナー承認が必要になる。

## トラブルシューティング

### base／headを取得できない

通常は`pull_request`から呼び出す。別イベントから呼ぶ場合は`base-sha`と`head-sha`を明示する。

### Java変更があるのに「変更なし」になる

`source`がリポジトリ相対パスになっているか、変更ファイルがその配下にあるか確認する。

### 変更ファイルに型が含まれていない場合

`package-info.java`など型を含まないファイルだけが変更された場合、既定の`v0.5.0` CLIはプレースホルダ図を生成する。`v0.4.0`のように「変更ファイルに型がない」というエラーだけを出した場合も、ワークフロー側でプレースホルダ図を生成するフォールバックが動作する。この場合もJavaファイルの変更はあるため、`no-java-changes`は`false`になる。解析警告を伴う場合や、その他の終了コード1のエラーは失敗として扱う。

### 再利用ワークフローを呼び出せない

呼び出し側リポジトリのActions設定で、公開リポジトリのaction／reusable workflow利用が許可されているか確認する。

## メンテナー向け: 公開前の配布リハーサル

これは上記の利用者向けPR差分workflowとは別です。umlgen自身のCIは`.github/workflows/release-build.yml`を版`0.6.0-rc.1`で呼び、Linux amd64／macOS arm64・amd64／Windows amd64をネイティブビルドします。Go 1.26.10と意味解析の最小JDK 17を使用し、Linux配布はUbuntu 22.04（glibc 2.35）を基準にします。

- チェックアウトの開発用CLIではなく、**アーカイブから取り出し／候補installerで配置したCLI**を`scripts/smoke-release.py`で検証します。版、全図種、cache一致、Git snapshot差分、秘密値非表示、watchの更新・削除・OS固有の中断が対象です。
- macOS／Windowsではローカルarchive modeと`scripts/smoke-installer.py`も使用し、破損・版違い・重複checksum・不正／重複memberの拒否、既存CLI／PATH保持、アンインストールを検証します。従来の公開版ダウンロードとWindows PlantUMLの画像生成も通常CIに残します。
- artifact `release-assets`には4つの公開形式アーカイブと`SHA256SUMS.txt`だけを保存します。候補artifactはrun／commitに紐付き、GitHub Releaseやlatest版の代用ではありません。成功したrunからダウンロードしてchecksumを検証してください。
- buildは`contents: read`のみ。`.github/workflows/release.yml`の検証済み`v*`タグpushだけが同じビルドを呼び、成功後のpublish jobに限って`contents: write`を使います。prereleaseタグはlatestにしません。通常PRや候補ビルドからの公開経路はありません。

ローカルでもネイティブ環境で候補を生成できます（Python 3、Go／CGO／Cコンパイラーが必要）。macOSの例:

```bash
GOTOOLCHAIN=go1.26.10 python3 scripts/package-release.py \
  --version 0.6.0-rc.1 --output-dir /tmp/umlgen-rc-dist
bash scripts/install-macos.sh --archive-dir /tmp/umlgen-rc-dist \
  --version 0.6.0-rc.1 --install-dir /tmp/umlgen-rc-install --skip-path-update
python3 scripts/smoke-release.py --binary /tmp/umlgen-rc-install/umlgen \
  --version 0.6.0-rc.1 --work-dir /tmp/umlgen-rc-smoke
```

smokeのwork directoryは空の新しい場所を指定します。意味解析にはフルJDK 17以上、Git差分にはGitが必要です。公開の判断・タグ作成は[リリース候補の受入条件](next-steps.md#公開前の受入条件)に従って別途行います。
