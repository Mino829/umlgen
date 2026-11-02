# umlgen

**ソースコードからクラス図を自動生成するCLIツール**

umlgenは、JavaとGoのソースコードを解析して、編集可能なPlantUMLクラス図を生成します。ローカルで完結するので、ソースコードが外部に送信されることはありません。

```bash
# Javaプロジェクトのクラス図を生成
umlgen class ./src/main/java

# Goプロジェクトのクラス図を生成
umlgen class . --language go
```

## 🎯 こんな方におすすめ

- **既存コードの理解**: 大規模プロジェクトの構造を視覚的に把握したい
- **設計レビュー**: チームでアーキテクチャを共有・議論したい
- **ドキュメント自動生成**: コードの変更に合わせて図も最新に保ちたい
- **PRレビュー**: 変更が全体に与える影響を素早く確認したい

## 🚀 クイックスタート

### 1. インストール

**macOS（推奨）:**
```bash
curl -fL https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-macos.sh | bash
```

**Windows（推奨）:**
```powershell
iwr https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-windows.ps1 | iex
```

**その他:** [Releases](https://github.com/Mino829/umlgen/releases) からダウンロード

### 2. クラス図の生成

```bash
# Javaプロジェクト
umlgen class ./src/main/java -o diagram.puml

# SVG画像も生成（PlantUML必要）
umlgen class ./src/main/java --format svg -o diagram.svg
```

### 3. 大きなプロジェクトの場合

```bash
# 特定のクラスとその関連だけを表示
umlgen class ./src --focus UserService --depth 2

# パッケージ単位で絞り込み
umlgen class ./src --include com.example.service
```

## 📚 ドキュメント

- **[Mac セットアップガイド](docs/macos-setup.md)** - インストールから使い方まで
- **[Windows セットアップガイド](docs/windows-setup.md)** - インストールから使い方まで
- **[PR差分図の使い方](docs/pr-diff.md)** - Pull Requestでの活用法
- **[GitHub Actions 連携](docs/github-actions.md)** - CI/CDでの自動生成
- **[パフォーマンス](docs/performance.md)** - ベンチマーク結果
- **[開発方針と説明機能の完了条件](docs/development-direction.md)** - 学習者の理解を最優先にした拡張順序

## 💡 主な機能

### 📊 多言語対応
- **Java**: Tree-sitterによる高速AST解析
- **Go**: 標準ライブラリによる確実な解析

### 🎨 柔軟な出力
- PlantUML（編集可能）
- SVG（ベクター画像）
- PNG（ラスター画像）

### 🔍 強力なフィルタリング
- `--focus`: 特定のクラスとその関連だけを表示
- `--include`/`--exclude`: パッケージやパスで絞り込み
- `--hide-private`: プライベートメンバーを非表示

### 🔗 関係性の可視化
- 継承（extends）
- 実装（implements）
- フィールド依存
- メソッド引数・戻り値
- 多重度（1対多、0..1など）

### 📈 Git連携
- 変更差分のハイライト（追加:緑、変更:黄、削除:赤）
- PR専用の差分図生成
- GitHub Actionsでの自動生成

### ⚡ 高速キャッシュ
- 解析結果をキャッシュして2回目以降を高速化
- ファイル内容と設定に基づいた安全なキャッシュ



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

## パッケージ表示とレイアウト

既定の`flat`は、完全なパッケージ名ごとの囲みと従来の出力を維持します。`hierarchical`はJavaの`.`、Goの`/`ごとに囲みを階層化し、Goのドメイン名（`example.com`など）は1つの階層として扱います。`none`はパッケージの囲みを省略します。

```bash
# パッケージ階層と横方向の配置
umlgen class ./src --package-grouping hierarchical --layout-direction left-to-right

# 囲みなしの図
umlgen class ./src --package-grouping none

# 依存先の探索方向と、図の配置方向は別の設定
umlgen class ./src --focus UserService --direction out \
  --layout-direction left-to-right --skinparam linetype=ortho
```

`--layout-direction`は`top-to-bottom`（既定）または`left-to-right`です。`.umlgen.yaml`にも設定できます。

```yaml
layout:
  package_grouping: hierarchical
  direction: left-to-right
  skinparams:
    linetype: ortho
    shadowing: false
    classAttributeIconSize: 0
```

`--skinparam key=value`は繰り返し指定でき、設定ファイルの同じキーだけを上書きします。同じキーを複数回指定した場合は最後の値を使います。安全に読み込める設定は次の3つに限定しています。

- `linetype`: `ortho`または`polyline`
- `shadowing`: `true`または`false`
- `classAttributeIconSize`: 非負の32bit整数（`0`でメンバーのアイコンを非表示）

未対応のキー・値や改行を含む設定は、図を書き出す前に引数エラーとして拒否します。任意のPlantUML本文や`!include`は読み込みません。タイトル、差分の追加・変更・削除色、型のaliasと関係線は維持します。

これらの設定は`--focus`と`diff`でも共通です。差分プレースホルダにも配置方向とskinparamが適用されます。レイアウト設定だけの変更では、解析キャッシュを無効化しません。

## 解析キャッシュ

Java／Goファイルの解析結果はOSのユーザーキャッシュ領域へ保存され、変更のない2回目以降の実行で再利用されます。キャッシュキーにはファイル内容、umlgenバージョン、言語別解析スキーマ、設定が含まれます。出力先だけを変えた場合は安全に再利用します。

Goではpackage節などの構文情報だけを再利用し、packageパスは各実行で現在の`go.mod`とソースの配置から決定します。同じソースを別moduleへ置いた場合や、`go.mod`のmodule名・配置ディレクトリを変更した場合も、以前のpackageパスを引き継ぎません。祖先に`go.mod`がなければpackage節の名前を使います。旧Go解析スキーマのエントリーは自動的にmissになります。

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

### 関係線の根拠を確認する

`--explain`は、生成した関係線の種類・意味、解決先、根拠となった型・メンバー、ソースファイルと1始まりの宣言行番号を標準出力へ表示します。開発版では宣言のソース抜粋と解析状況も表示します。Java／Goに対応し、通常の`.puml`や画像の内容は変更しません。

```bash
# 図を生成しながら、関係線の理由も読む
umlgen class ./src --explain

# UserServiceが保持している型を、フィールド名と宣言位置から調べる
umlgen class ./src --focus UserService --direction out \
  --relations field --explain --quiet

# Goの埋め込みや、メソッド集合から推定したインターフェース実装を確認
umlgen class . --language go --explain

# 差分図に表示された関係線の理由を読む
umlgen diff main...HEAD --explain
```

たとえば`UserService`に`UserRepository`型の`repository`フィールドがある場合、`field`関係の説明にフィールド名・宣言された型・そのファイルと行番号が表示されます。オーバーロードなどで1本の関係線に複数の根拠がある場合は、すべての宣言位置を列挙します。

説明対象は`--focus`、`--direction`、`--relations`などで絞った後の図に合わせます。`--no-relations`や差分プレースホルダでは関係線と参照の診断を表示せず、解析範囲と解析失敗だけを表示します。`--hide-fields`や`--hide-methods`はメンバー表示のみの設定なので、図に残る関係線の根拠は説明されます。`--quiet`でも、明示的に要求した説明は出力します。

行番号は宣言の開始位置です。引数は各引数宣言、戻り値はメソッド宣言を指します。抜粋はその位置から最大3行で、宣言全体とは限りません。解析に渡した同じソースを使い、作業ツリーは`working tree`、削除ファイルのGit比較元は`Git base`と表示します。表示時にファイルを読み直さず、本文を解析キャッシュへ保存しません。

Goの埋め込みはJavaのクラス継承と区別します。暗黙のインターフェース実装では、候補の型宣言に加え、一致した各メソッドのシグネチャと宣言位置・抜粋を示します。別ファイルや埋め込み由来のメソッドも対象です。ただし判定は構文ベースで、Goコンパイラーによる検証済みとは表示しません。

```bash
# 学習用fixtureで宣言と図の根拠を読み比べる
go run ./cmd/umlgen class testdata/java/compatibility/project --explain --quiet

# Goの暗黙実装を支えるメソッドを読む
go run ./cmd/umlgen class testdata/go/compatibility \
  --relations implementation --explain --quiet

# 型だけに絞ったとき、図の外の既知の型と未解決参照を区別する
go run ./cmd/umlgen class ./src --focus UserService --depth 0 --explain --quiet
```

解析状況は関係線とは別に読みます。

- `Analysis scope`: 対象パス、選択ファイル数、言語、include／exclude条件。対象外・外部依存のソースを網羅的に読んだとは主張しません。
- `Analysis failures`: 実際に読み込み・解析に失敗したファイルとエラー。一部失敗時も表示し、全件解析失敗時は従来どおり終了コード3のまま状況を読めます。
- `Unresolved references`: 解析した宣言から解決できなかった参照。型が存在しない、依存がない、外部ライブラリだから、といった原因を推測して断定しません。
- `Known references outside diagram`: 解析済みですが、パッケージ条件やfocusで図の外になった既知の型。未解決参照とは別です。

出力形式は人が読むためのテキストです。公開JSON APIや機械向けの安定した出力スキーマはまだ提供していません。

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

`package-info.java`のみの変更など、解析できた変更ファイルに表示可能な型がない場合は、プレースホルダ図を生成して終了コード0で成功します。変更ファイルを解析できず、表示可能な変更型がない場合は終了コード3です。

差分図の詳しい使い方、精度、検出ルールについては[`docs/pr-diff.md`](docs/pr-diff.md)を参照してください。

### Pull Requestで自動生成

公開している再利用可能GitHub Actionsワークフローを利用すると、Pull Requestごとに差分図の`change-diagram.puml`とSVG、およびPRのhead commitから生成した全体図の`full-diagram.puml`とSVGを同じartifactへ保存できます。

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

layout:
  package_grouping: flat
  direction: top-to-bottom
  skinparams: {}

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

タイムアウト時や空（0バイト）の画像が返された場合は、レンダリング失敗（終了コード5）として扱い、元の`.puml`を保持します。ローカル／JARレンダラーでは、コマンド終了後も子プロセスが標準出力・標準エラーを開いたままの場合、パイプの待機を最大100msで打ち切ります。子孫プロセス全体の終了は保証しません。

## 開発

```bash
make test
make bench
make vet
make build
```

Tree-sitterを使用するため、ビルドにはCGoとCコンパイラが必要です。リリース用バイナリは各OSのGitHub Actionsランナー上でネイティブビルドされます。

CI・CodeQL・リリースビルドではGo 1.26.10を使用します。`crypto/x509`の脆弱性[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809)の修正版で、CIの`govulncheck`による検査を継続します。

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

## 🔧 トラブルシューティング

### `umlgen`コマンドが見つからない

インストール後にターミナルを再起動してください。それでも解決しない場合は、PATHが通っているか確認します。

```bash
# macOS/Linux
echo $PATH | grep -o "umlgen"
which umlgen

# Windows
echo $env:PATH
Get-Command umlgen
```

### SVG/PNGが生成されない

PlantUMLがインストールされているか確認します。

```bash
# macOS
brew install plantuml

# Windows（Chocolatey）
choco install plantuml

# 手動インストール
# PlantUMLのJARファイルをダウンロードして、PLANTUML_JAR環境変数を設定
export PLANTUML_JAR=/path/to/plantuml.jar
```

### 図が空になる

- `--include`や`--exclude`ですべてのファイルが除外されていないか確認
- 対象ディレクトリにJava/Goファイルがあるか確認
- `--verbose`オプションで詳細ログを確認

```bash
umlgen class ./src --verbose
```

### Goのinterface実装が検出されない

- ポインタレシーバーのメソッドは、ポインタ型のみが実装と判定されます
- 値型とポインタ型の両方で実装と判定したい場合は、値レシーバーを使用してください

### キャッシュをクリアしたい

```bash
umlgen cache clean
```

### その他の問題

1. `--verbose`オプションで実行して詳細ログを確認
2. [GitHub Issues](https://github.com/Mino829/umlgen/issues)で報告



## ライセンス

umlgenは[MIT License](LICENSE)で公開しています。個人・法人を問わず、利用、改変、再配布、商用利用が可能です。再配布する場合は、MIT Licenseの著作権表示と許諾表示を含めてください。
