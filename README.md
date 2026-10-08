# umlgen

`umlgen`は、Java／Goソースコードをローカルで解析し、編集可能なPlantUMLクラス図を生成するCLIです。

```bash
umlgen class ./src/main/java
umlgen class . --language go
```

ソースコードが外部へ送信されることはありません。

**[Pull Requestの設計変更がどう見えるか、30秒デモを見る](examples/order-service/README.md)**

## 主な機能

- Tree-sitter JavaによるAST解析
- Go標準ASTによるGoソース解析
- class、interface、annotation、enum、recordの抽出
- Goのstruct、interface、field、methodの抽出
- Goのembeddingと暗黙的なinterface実装の関係生成
- importと入れ子型を考慮した型解決
- フィールド、メソッド、コンストラクタの抽出
- 継承、実装、フィールド型、引数型、戻り値型による関係の生成
- コレクションとOptionalの多重度表示
- 特定の型と周辺だけを表示するフォーカス機能
- Git差分に含まれる型と周辺型の色分け
- パッケージやパスによる絞り込み
- PlantUML、SVG、PNG出力
- `.umlgen.yaml`によるプロジェクト設定
- macOS、Linux、WindowsでのCIとリリースビルド

## インストール

### macOS（Apple Silicon／Intel）

管理者権限なしのインストーラーがMacの種類を自動判定します。

```bash
installer="$TMPDIR/install-umlgen.sh"
curl -fL https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-macos.sh -o "$installer"
bash "$installer"
```

インストール後にターミナルを開き直し、`umlgen version`で確認します。画面の開き方から図の生成、更新、アンインストールまでの説明は[Macセットアップガイド](docs/macos-setup.md)を参照してください。

### Windows（おすすめ）

PowerShellインストーラーを使うと、管理者権限やJavaの事前準備なしでumlgenとSVG生成環境をセットアップできます。

```powershell
$installer = "$env:TEMP\install-umlgen.ps1"
Invoke-WebRequest https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-windows.ps1 -OutFile $installer
powershell -NoProfile -ExecutionPolicy Bypass -File $installer -InstallPlantUML
```

インストール後にPowerShellを開き直し、`umlgen version`で確認します。画面の開き方から図の生成、更新、アンインストールまでの説明は[Windowsセットアップガイド](docs/windows-setup.md)を参照してください。

### Goからインストール

Go 1.24以降とCコンパイラが必要です。

```bash
go install github.com/Mino829/umlgen/cmd/umlgen@latest
```

### GitHub Releases

[Releases](https://github.com/Mino829/umlgen/releases)からOSに合ったファイルをダウンロードし、展開した`umlgen`をPATHが通った場所へ配置します。

macOS／Linuxの例：

```bash
chmod +x umlgen
mv umlgen ~/.local/bin/
```

## 基本的な使い方

```bash
# クラス図を生成
umlgen class ./src/main/java

# go.modからGoプロジェクトを自動判定
umlgen class .

# 言語を明示
umlgen class . --language go

# 出力先を指定
umlgen class ./src/main/java -o docs/domain.puml

# SVGも生成（ローカルにPlantUMLが必要）
umlgen class ./src --format svg

# PNGも生成（ローカルにPlantUMLが必要）
umlgen class ./src --format png

# privateメンバーとメソッドを非表示
umlgen class ./src --hide-private --hide-methods

# 対象パッケージを限定
umlgen class ./src --include com.example.user

# テストや生成コードを除外
umlgen class ./src --exclude test --exclude generated
```

オプションは対象パスの前後どちらにも指定できます。

```bash
umlgen class --help
```

## 大きなプロジェクトを読みやすくする

`--focus`は、指定した型と直接関係する型だけを出力します。

```bash
umlgen class ./src --focus UserService
```

`--depth`で何段先の関係まで含めるか指定できます。デフォルトは`1`です。

```bash
# UserServiceだけ
umlgen class ./src --focus UserService --depth 0

# 2段先まで
umlgen class ./src --focus UserService --depth 2

# 同名クラスがある場合は完全修飾名を使用
umlgen class ./src --focus com.example.user.UserService --depth 2

# Goの完全修飾名（module path + package + type）
umlgen class . --focus github.com/example/project/internal/user.Service --depth 2

# 依存先だけを表示
umlgen class ./src --focus UserService --direction out

# この型へ依存している型だけを表示
umlgen class ./src --focus UserService --direction in
```

関係は双方向に探索されるため、依存先だけでなく、その型に依存している型も含まれます。
`--direction`を指定すると、`in`、`out`、`both`から探索方向を選べます。

## 解析キャッシュ

Java／Goファイルの解析結果はOSのユーザーキャッシュ領域へ保存され、変更のない2回目以降の実行で再利用されます。キャッシュキーにはファイル内容、umlgenバージョン、言語別解析スキーマ、設定が含まれます。出力先だけを変えた場合は安全に再利用します。

詳細ログではヒット数を確認できます。

```bash
umlgen class ./src --verbose
```

一時的にキャッシュを使わない場合：

```bash
umlgen class ./src --no-cache
```

すべてのumlgenキャッシュを削除する場合：

```bash
umlgen cache clean
```

キャッシュにはソース本文やソースファイルのパスを保存せず、型・メンバー・関係解析に必要な宣言情報だけを保存します。ネットワーク通信は行いません。破損したエントリーは破棄して自動的に再解析します。

## 関係の種類と多重度

関係線を継承、実装、フィールド、引数、戻り値から選択できます。

```bash
umlgen class ./src \
  --relations inheritance,implementation,field \
  --show-relation-labels
```

利用できる値：

- `inheritance`
- `implementation`
- `field`
- `parameter`
- `return`
- `all`

`List<User>`、`User[]`、Goの`[]User`や`map[string]User`は`*`、`Optional<User>`は`0..1`として関係線へ出力されます。

## Git差分からクラス図を生成

変更されたJava／Go型と、その周辺の型だけを生成します。

```bash
# 直前の状態との差分
umlgen diff HEAD~1

# mainブランチとのPR差分
umlgen diff main...HEAD --depth 2

# 関係ラベル付きのSVG
umlgen diff main...HEAD \
  --show-relation-labels \
  --format svg \
  --output docs/change-diagram.puml
```

差分図では追加を緑、変更を黄色、削除を赤で表示します。削除されたJava／GoファイルもGit履歴から読み込んで図に含めます。デフォルト出力先は`change-diagram.puml`です。

### Pull Requestで自動生成

公開している再利用可能GitHub Actionsワークフローを利用すると、Pull Requestごとに差分図の`.puml`とSVGをartifactへ保存できます。

```yaml
jobs:
  umlgen-diff:
    permissions:
      contents: read
    uses: Mino829/umlgen/.github/workflows/umlgen-pr-diff.yml@main
    with:
      source: src/main/java
```

導入方法、入力、Fork Pull Requestのセキュリティ上の注意は[`docs/github-actions.md`](docs/github-actions.md)を参照してください。

## 設定ファイル

設定のひな型を生成します。

```bash
umlgen init
```

生成される`.umlgen.yaml`：

```yaml
language: auto

source:
  - .

exclude:
  - src/test
  - target
  - build
  - generated

output:
  file: docs/class-diagram.puml
  format: plantuml

visibility:
  public: true
  protected: true
  private: true
  package_private: true

members:
  fields: true
  methods: true

relations:
  inheritance: true
  implementation: true
  field_dependency: true
  parameter_dependency: true
  return_dependency: true

renderer:
  type: auto
  server_url: ""
  timeout: 30s
```

`language`は`auto`、`java`、`go`から選択できます。`auto`は対象ファイルと`go.mod`、`pom.xml`、Gradle設定から判定します。JavaとGoが混在して判定できない場合は、`--language`または設定ファイルで明示してください。

優先順位は、コマンドライン、`--config`で指定した設定、`.umlgen.yaml`、デフォルト値の順です。

## SVG／PNG出力

SVG・PNG出力にはPlantUMLレンダラーが必要です。デフォルトでは`plantuml`コマンドを探し、見つからなければ`PLANTUML_JAR`を使います。`renderer`を`server`に設定した場合のみ、PlantUML Serverを利用します（ソースコードがサーバーに送信されます）。

macOS：

```bash
brew install plantuml
umlgen class ./src --format svg
umlgen class ./src --format png
```

または`PLANTUML_JAR`へ`plantuml.jar`のパスを設定できます。画像生成に失敗した場合も、元の`.puml`は残ります。

### レンダラー設定

`.umlgen.yaml`でレンダラーとタイムアウトを指定できます。

```yaml
renderer:
  type: auto       # auto, local, jar, server
  server_url: ""   # server使用時のみ（ソースが外部に送信されます）
  timeout: 30s
```

`--renderer`、`--server-url`、`--render-timeout`フラグでも上書きできます。

## 開発

```bash
make test
make bench
make vet
make build
```

Tree-sitterを使用するため、ビルドにはCGoとCコンパイラが必要です。リリース用バイナリは各OSのGitHub Actionsランナー上でネイティブビルドされます。

ベンチマークの構成と参考結果は[`docs/performance.md`](docs/performance.md)に記録しています。

タグをpushすると、macOS（Apple Silicon／Intel）、Linux amd64、Windows amd64向けのアーカイブとSHA-256チェックサムがGitHub Releasesへ自動公開されます。

```bash
git tag v0.2.0
git push origin v0.2.0
```

## 終了コード

| コード | 意味 |
| ---: | --- |
| 0 | 正常終了 |
| 1 | 一般エラーまたは対象エラー |
| 2 | CLI引数または設定エラー |
| 3 | すべてのソースファイルの解析に失敗 |
| 4 | 出力エラー |
| 5 | SVGレンダリングエラー（`.puml`は保持） |

## 現在の解析範囲

### Java

Javaの宣言構文はTree-sitterの構文木から取得します。特定のフレームワークやビルドツールに依存せず、一般的なJavaソースからクラス図を生成することを基本方針としています。明示的import、ワイルドカードimport、同一パッケージ、入れ子型を使ってプロジェクト内の型を解決します。

annotation宣言とその要素、enum定数、record、generic型パラメーター、abstract／final／sealed／non-sealed型、static／abstractメンバーを図へ反映します。generic型やwildcard型に含まれるプロジェクト内の型も、解決できる範囲で関係へ反映します。

構文エラーのあるファイルは警告してスキップし、解析できるファイルから図を生成します。すべてのJavaファイルを解析できなかった場合は終了コード3で終了します。

現在、次の要素は意味解析の対象外です。

- sealed型の`permits`一覧からの関係推論（型自身の`extends`／`implements`は反映します）
- annotationの用途や意味
- generic型パラメーターと境界の完全な型解析
- プロジェクト外の未解決型
- ローカルクラス、匿名クラス、enum定数固有のclass body
- リフレクション、Lombokが生成するメンバー
- メソッド内部の呼び出し
- Spring固有の高度な依存注入推論

互換性は、Maven／Gradleで一般的な`src/main/java`構成を模した複数モジュールfixtureと、生成PlantUMLのgolden testで継続的に確認します。

### Go

Goの宣言構文は標準ライブラリの`go/parser`から取得します。`go.mod`のmodule path、package、import aliasを使ってプロジェクト内の型を解決します。

- structは`<<struct>>`付きのclassとして表示する
- interface embeddingとstruct embeddingを継承関係として表示する
- プロジェクト内interfaceのmethod setを満たすstructを実装関係として表示する
- slice、array、mapを多重度`*`として表示する
- `_test.go`と`vendor`はデフォルトの解析対象から除外する
- receiver methodが型宣言とは別ファイルにあっても統合する

現在、次の要素はGoの意味解析対象外です。

- 実行時解析とcall graph
- CGo内部の解析
- build tagによるファイル選択
- プロジェクト外interfaceへの暗黙実装の推論
- 関数本体からの依存関係推論

互換性は、複数package、import、embedding、暗黙interface実装を含むGo fixtureで継続的に確認します。

## ライセンス

umlgenは[MIT License](LICENSE)で公開しています。個人・法人を問わず、利用、改変、再配布、商用利用が可能です。再配布する場合は、MIT Licenseの著作権表示と許諾表示を含めてください。
