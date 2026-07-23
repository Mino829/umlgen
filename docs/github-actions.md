# GitHub ActionsでPull Request差分図を生成する

umlgenの再利用可能ワークフローを呼び出すと、Pull Requestのbase／headからJavaの差分クラス図を生成し、PlantUMLとSVGをActions artifactとして保存できる。

## 最小構成

利用するリポジトリに`.github/workflows/umlgen-diff.yml`を作成する。

```yaml
name: UML diff

on:
  pull_request:

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

実行後、Actionsのrun summaryにあるArtifactsから`umlgen-pr-diff`をダウンロードする。内容は次の2ファイル。

```text
change-diagram.puml
change-diagram.svg
```

## 入力

| Input | Default | 説明 |
| --- | --- | --- |
| `source` | `.` | リポジトリ相対のJavaソースパス |
| `depth` | `1` | 変更型から含める関係の深さ |
| `direction` | `both` | 関係の探索方向：`in`、`out`、`both` |
| `umlgen-version` | `v0.3.0` | ダウンロードするumlgenのリリースタグ |
| `artifact-name` | `umlgen-pr-diff` | artifact名 |
| `retention-days` | `14` | artifact保持日数 |
| `base-sha` | Pull Requestのbase | base commitの上書き |
| `head-sha` | Pull Requestのhead | head commitの上書き |

`source`以下に変更されたJavaファイルがない場合も失敗にはしない。その場合は「Java変更なし」と記載したPlantUML／SVGをartifactへ保存する。

## 出力

再利用ワークフローは次のoutputを返す。

- `artifact-url`: アップロードされたartifactのURL
- `no-java-changes`: 選択したパスにJava変更がなければ`true`

呼び出し側で後続jobから参照する場合は、呼び出しjobへ`id`を付けるか、job outputとして引き継ぐ。

## セキュリティ

- `pull_request`イベントと`contents: read`だけを使用する
- Fork Pull Requestではread-onlyの`GITHUB_TOKEN`で動作し、secretを要求しない
- `pull_request_target`は使用しない
- checkout時に認証情報を保持しない
- umlgenのLinuxアーカイブは公開リリースの`SHA256SUMS.txt`で検証する
- PlantUML Serverは使わず、GitHub-hosted runner内のPlantUML CLIでSVGを生成する
- ソースコードを外部の図生成サービスへ送信しない

GitHubの設定によっては、初回Fork Pull Requestのworkflow実行にメンテナー承認が必要になる。

## トラブルシューティング

### base／headを取得できない

通常は`pull_request`から呼び出す。別イベントから呼ぶ場合は`base-sha`と`head-sha`を明示する。

### Java変更があるのに「変更なし」になる

`source`がリポジトリ相対パスになっているか、変更ファイルがその配下にあるか確認する。

### 再利用ワークフローを呼び出せない

呼び出し側リポジトリのActions設定で、公開リポジトリのaction／reusable workflow利用が許可されているか確認する。
