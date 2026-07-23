# umlgen

`umlgen`は、Javaソースコードをローカルで解析し、編集可能なPlantUMLクラス図を生成するCLIです。

```bash
umlgen class ./src/main/java
```

ソースコードが外部へ送信されることはありません。

## 主な機能

- Tree-sitter JavaによるAST解析
- class、interface、enum、recordの抽出
- importと入れ子型を考慮した型解決
- フィールド、メソッド、コンストラクタの抽出
- 継承、実装、フィールド型、引数型、戻り値型による関係の生成
- コレクションとOptionalの多重度表示
- 特定の型と周辺だけを表示するフォーカス機能
- Git差分に含まれる型と周辺型の色分け
- パッケージやパスによる絞り込み
- PlantUMLおよびSVG出力
- `.umlgen.yaml`によるプロジェクト設定
- macOS、Linux、WindowsでのCIとリリースビルド

## インストール

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

# 出力先を指定
umlgen class ./src/main/java -o docs/domain.puml

# SVGも生成（ローカルにPlantUMLが必要）
umlgen class ./src --format svg

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

# 依存先だけを表示
umlgen class ./src --focus UserService --direction out

# この型へ依存している型だけを表示
umlgen class ./src --focus UserService --direction in
```

関係は双方向に探索されるため、依存先だけでなく、その型に依存している型も含まれます。
`--direction`を指定すると、`in`、`out`、`both`から探索方向を選べます。

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

`List<User>`や`User[]`は`*`、`Optional<User>`は`0..1`として関係線へ出力されます。

## Git差分からクラス図を生成

変更されたJava型と、その周辺の型だけを生成します。

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

差分図では追加を緑、変更を黄色、削除を赤で表示します。削除されたJavaファイルもGit履歴から読み込んで図に含めます。デフォルト出力先は`change-diagram.puml`です。

## 設定ファイル

設定のひな型を生成します。

```bash
umlgen init
```

生成される`.umlgen.yaml`：

```yaml
language: java

source:
  - src/main/java

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
```

優先順位は、コマンドライン、`--config`で指定した設定、`.umlgen.yaml`、デフォルト値の順です。

## SVG出力

SVG出力にはPlantUMLが必要です。

macOS：

```bash
brew install plantuml
umlgen class ./src --format svg
```

または`PLANTUML_JAR`へ`plantuml.jar`のパスを設定できます。SVG生成に失敗した場合も、元の`.puml`は残ります。

## 開発

```bash
make test
make vet
make build
```

Tree-sitterを使用するため、ビルドにはCGoとCコンパイラが必要です。リリース用バイナリは各OSのGitHub Actionsランナー上でネイティブビルドされます。

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

Javaの宣言構文はTree-sitterの構文木から取得します。明示的import、ワイルドカードimport、同一パッケージ、入れ子型を使ってプロジェクト内の型を解決します。

sealed class／interfaceは通常のclass／interfaceとして、annotation宣言はinterfaceとして図へ出力します。record、generic型、wildcard型に含まれるプロジェクト内の型も、解決できる範囲で関係へ反映します。

構文エラーのあるファイルは警告してスキップし、解析できるファイルから図を生成します。すべてのJavaファイルを解析できなかった場合は終了コード3で終了します。

現在、次の要素は意味解析の対象外です。

- sealed型の`permits`関係
- annotationの用途や意味
- generic型パラメーターと境界の完全な型解析
- プロジェクト外の未解決型
- リフレクション、Lombokが生成するメンバー
- メソッド内部の呼び出し
- Spring固有の高度な依存注入推論

互換性は、Maven／Gradleで一般的な`src/main/java`構成を模した複数モジュールfixtureと、生成PlantUMLのgolden testで継続的に確認します。
