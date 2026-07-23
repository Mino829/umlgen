# umlgen

`umlgen`は、Javaソースコードをローカルで解析し、編集可能なPlantUMLクラス図を生成するCLIです。

```bash
umlgen class ./src/main/java
```

ソースコードが外部へ送信されることはありません。

## 主な機能

- Tree-sitter JavaによるAST解析
- class、interface、enum、recordの抽出
- フィールド、メソッド、コンストラクタの抽出
- 継承、実装、フィールド型、引数型、戻り値型による関係の生成
- 特定の型と周辺だけを表示するフォーカス機能
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
```

関係は双方向に探索されるため、依存先だけでなく、その型に依存している型も含まれます。

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

Javaの宣言構文はTree-sitterの構文木から取得します。リフレクション、Lombokが生成するメンバー、メソッド内部の呼び出し、Spring固有の高度な依存注入推論は対象外です。
