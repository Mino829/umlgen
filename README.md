# umlgen

**Java／Goのソースコードから各種UML図、Compose／Kubernetesの宣言から配置図を生成するCLIツール**

umlgenは、JavaとGoのソースコードや対応する配置manifestを解析して、編集可能なPlantUMLを生成します。解析はローカルで行います。明示的にserverレンダラーを選んだ場合のみ、生成済みPlantUMLを外部送信します。配置図は環境変数・Secret／ConfigMapの値を取り込みません。

`package`・package差分・`sequence`・`activity`・`state`・`component`・`deployment`・`watch`は**v0.6.0のリリース候補**です。現在の公開安定版はv0.5.0で、通常のインストーラーは引き続き公開版を取得します。候補版の範囲・配布検証・公開条件は[次にやること](docs/next-steps.md#v060のリリース候補)を参照してください。

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
- **Java**: class／packageはTree-sitter、sequence／activity／stateはJDKコンパイラーによる意味解析
- **Go**: class／packageは標準AST、sequence／activity／stateは選択済みローカルソースの`go/types`による意味解析

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
- class／packageの構文解析結果をキャッシュして2回目以降を高速化。sequence／activity／stateの意味解析は毎回実行
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

PowerShellインストーラーを使うと、管理者権限やJavaの事前準備なしでumlgenとクラス図のSVG生成環境をセットアップできます。Javaの`sequence`／`activity`／`state`には、別途フルJDK 17以上が必要です（PlantUML同梱の実行環境では代用できません）。

```powershell
$installer = "$env:TEMP\install-umlgen.ps1"
Invoke-WebRequest https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-windows.ps1 -OutFile $installer
powershell -NoProfile -ExecutionPolicy Bypass -File $installer -InstallPlantUML
```

インストール後にPowerShellを開き直し、`umlgen version`で確認します。画面の開き方から図の生成、更新、アンインストールまでの説明は[Windowsセットアップガイド](docs/windows-setup.md)を参照してください。

### Goからインストール

開発版をソースからビルドするにはGo 1.25以降とCコンパイラが必要です。

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

### v0.6.0候補の動作環境

| 用途 | 要件 |
| --- | --- |
| 配布バイナリー | Linux amd64（glibc 2.35以上）、macOS arm64／amd64、Windows amd64。macOS 15／Windows runnerでネイティブ配布検証。Alpine／Linux arm64は配布対象外 |
| ソースビルド | Go 1.25以上、CGO有効、Cコンパイラー。通常の配布バイナリー利用にはGo不要 |
| Java class／package／component、class／packageのwatch | JDK不要（構文解析） |
| Java sequence／activity／state | フルJDK 17以上をPATHに設定（`java -version`と`javac -version`を確認）。必要なら`--java-release 17`で対象言語レベルを固定 |
| Goの各図 | 配布バイナリーだけでローカル解析。対象のbuild／実行／依存downloadは行わない |
| SVG／PNG | ローカルPlantUML。server方式は明示選択時だけ送信。PlantUMLのJava要件は意味解析用JDKとは別 |
| Git差分 | GitをPATHに設定 |

候補版はPRのCI artifact `release-assets`（4種類のアーカイブと`SHA256SUMS.txt`）から確認できます。展開後は必ずチェックサムを検証してください。候補の番号`0.6.0-rc.1`は配布リハーサル用で、GitHub Releaseを公開したという意味ではありません。候補インストールは[macOS](docs/macos-setup.md#公開前の候補版を試す)／[Windows](docs/windows-setup.md#公開前の候補版を試す)の手順を参照してください。

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

オプションは対象パスの前後どちらにも指定できます。コマンドより前のグローバルフラグは先に解析し、`--config class class ./src`のように設定ファイル名がコマンド名と同じ場合も、値を消費してから実行コマンドを選びます。

Goの匿名struct／interfaceを含むフィールド・引数・戻り値は、型表記を1行のリテラルとして描画します。波括弧やstruct tagなどはPlantUMLの文字エスケープに変換するため、後続のフィールドを隠さず表示できます。解析モデルとキャッシュには元の型表記を保持します。

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

Javaのデフォルトパッケージは空のpackageブロックを作らず、トップレベルに型を描画します。名前付きパッケージとの混在にも対応します。

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

## パッケージ依存図（開発版）

`class --package-grouping`は型を囲む枠の設定です。`package`は**package自体を端点**にして、利用側→依存先の関係を集約します。

```bash
# Javaの宣言型からpackage間依存を生成
umlgen package ./src/main/java

# Goの型関係とimportを別々に数え、外部import pathも表示
umlgen package . --language go --dependency-source both --external show --explain

# package近傍を2hopまで表示。配置方向とは別の指定
umlgen package ./src --focus com.example.service --depth 2 --direction out \
  --layout-direction left-to-right

# 循環グループに属するpackageのみ表示
umlgen package ./src --cycles only --format svg --renderer local
```

| `--dependency-source` | 表す根拠 |
| --- | --- |
| `declared-types`（既定） | 継承、実装、フィールド、引数、戻り値として解決できた型関係 |
| `imports` | 元ファイルに書かれたimport宣言。Goのblank／dot importや型0件ファイルも対象 |
| `both` | 上記を別々に集約。辺の`types`は一意な型関係レコード数、`imports`は元の宣言数 |

既定出力は`package-diagram.puml`です。`output.file`を省略または`null`にした設定では、`class-diagram.puml`を上書きしません。明示した`output.file`はコマンド共通です。`--output`／`--format plantuml|svg|png`、キャッシュ、レンダラー設定も利用できます。

```yaml
diagrams:
  package:
    dependency_source: declared-types
    external: hide
    cycles: mark
```

- 優先順位はCLI明示値→YAML→既定値。YAMLのsource／outputは設定ファイル基準、CLIのパスはcwd基準です。`--exclude`は設定リストを置き換えます。
- `--include`とpackage名の除外は解決済みgraphへ適用します。非表示にした既知のpackageを外部・未解決へ再分類しません。
- `--external hide|show`の既定は`hide`。Goの外部import pathは識別できますが、存在・型・build互換性は未検証です。Javaで所有packageが分からないimportは未解決として残します。
- `--cycles off|mark|only`の既定は`mark`。通常宣言／importで成立する循環と、推定実装を必要とする候補循環を区別します。循環は関係選択後・focus／外部非表示前の解析範囲で判定し、`off`では判定しません。
- `--focus`は一意なcanonical package名を指定します。同名別文脈があれば、エラーに示す`絶対rootパス::package名`を使います。Javaは解析root、Goは最寄りのmodule root（moduleなしでは所在ディレクトリ）で区別します。`--depth 0`は指定packageだけです。
- `--relations inheritance,implementation,field,parameter,return`は型関係だけを絞ります。`imports`との併用は引数エラー、`both`ではimport宣言を消しません。
- `--explain`は型／importの位置・hash・解析時のソース抜粋、失敗・未解決・既知の非表示対象、推定実装メソッド、表示前の循環範囲を説明します。通常図は変わらず、未指定時には詳細抜粋用のソース本文を保持しません。
- YAMLのclass専用visibility／members／relations／package_grouping／classAttributeIconSizeはpackage図へ適用しません。class専用CLI flagは拒否します。package用skinparamは`linetype`／`shadowing`のみです。新しい`diagrams`内の未知キー・namespace・型も設定エラーです。

部分解析失敗は警告と診断を残し、成功した入力から生成します。全件失敗は終了コード3で既存出力を保持します。関数本体・外部依存のソース・Java classpath・Go build tagsは解析せず、循環はコンパイル可否や実行時の呼び出し順を表しません。表示上の「依存0件」をリポジトリ全体の依存なしと解釈しないでください。

Java／Goの実ソース、依存種別ごとの件数、曖昧な型・外部import・循環の読み方は[パッケージ図の利用例](examples/package-diagrams/README.md)を参照してください。[性能比較](docs/performance.md)には測定条件とclassとの差を記載しています。全フラグは`umlgen package --help`で確認できます。

### package差分（開発版）

```bash
umlgen diff main...HEAD --diagram package --dependency-source both --explain
umlgen diff HEAD~1..HEAD ./src --diagram package --language go \
  --format svg --renderer local --output package-changes.svg
```

比較元／先の**全選択ソースと`go.mod`を固定したGit commit**から解析し、未変更ファイルに残る根拠も集約します。既定出力は`package-change-diagram.puml`。`diff`の既定は従来のclass図のままです。

- package差分の単一revisionは`revision..HEAD`。`A..B`は両commit、`A...B`はmerge-baseとBを比較し、完全なSHAを図・説明に残します。working tree・staged・untrackedは含めません。
- 辺の追加／削除、同じ辺の根拠変更、変更なしの文脈を区別し、`types`／`imports`を比較前後別に表示します。行移動・ソースhashだけの変化は依存変更にしません。ファイルrenameは根拠変更、package／module identityの変更は削除＋追加で、renameの推測統合はしません。
- `--external`／`--cycles only`は比較後の両側unionを絞る表示条件です。片側で循環から外れただけの辺を削除扱いにしません。型関係の`--relations`とimportの独立性も維持します。
- 部分解析失敗は不確かな差分と診断を生成して終了コード3、片側全件失敗は既存出力を保持して終了コード3。未解決／曖昧な参照があるpackageの辺や関係する循環では、不確かな追加・削除・解消を断定しません。
- invocation時の設定・include／excludeを両commitへ同じ条件で適用します。対象はGit repo内のパス。片側で削除済みの対象も指定でき、選択したsource symlink／submoduleは明示エラーにします。build条件や外部本文は解析しません。
- package差分では`--focus`／`--depth`／`--direction`とclass専用flagを拒否します。全体package図のfocusと既存class diffの近傍指定は従来どおりです。

再現可能なGit操作例は[利用例](examples/package-diagrams/README.md)、snapshotと終了コードの詳細は[差分機能](docs/pr-diff.md#パッケージ依存の差分開発版)を参照してください。既存GitHub Actions workflowの既定コマンドや配布版は変更していません。

## 静的シーケンス図（開発版）

`sequence`は、指定したentryからの**静的な呼び出し・構造化された分岐やループ**を描きます。実行traceではなく、引数の実行値・分岐結果・ループ回数・スケジュールを推定しません。

```bash
# リポジトリの代表fixtureで、呼び出しと根拠のソースを確認
umlgen sequence testdata/sequence/go --language go --entry PlaceOrder --explain
umlgen sequence testdata/sequence/java --language java --entry PlaceOrder \
  --format svg --renderer local -o sequence.svg

# entry直下だけ表示し、参加者数・メッセージ数にも上限を設ける
umlgen sequence ./src --entry 'sample.Checkout.PlaceOrder(boolean,sample.Repository,java.lang.Runnable)' \
  --max-call-depth 0 --max-participants 15 --max-messages 50
```

- entryは一意な単純名・完全名・引数型を含むsignature・`context::signature`。曖昧なら候補を表示して終了コード2にします。
- 既定上限は展開深さ3・参加者30・メッセージ200。再帰・上限・非対応の位置と省略を残します。動的dispatchは代替候補で、全候補の本体を直列に展開しません。Goの`go`／`defer`も同期呼び出しとして展開しません。
- Goは選択済みローカルpackageと`unsafe`だけを解決し、goコマンド・driver・download・cgo・外部ソース読み込みを行いません。標準ライブラリも`unsafe`以外はロードせず、不足する型情報を診断します。
- Java sequence／activity／stateのみ**JDK 17以上**が必要です（既定`--java-release 17`）。組込みの信頼済みhelperでparse／analyzeのみを行い、対象コード・annotation processor・build・pluginを実行しません。project classpath／sourcepathは使わず、外部依存不足は診断します。既存Java class／packageにJDKは不要です。
- `--analysis-timeout`は既定30秒。Java helperを期限付きで終了し、Goは解析段階間の協調的な期限確認を行います。Goの型検査中を強制中断する制限ではありません。
- 構文／型エラーがあってもentryが利用可能なら診断付きの図を出し終了コード3。entryが利用不能なら既存図を保持します。非対応構文は位置付きの警告・barrier・省略で示します。
- 既定出力は`sequence-diagram.puml`。共有output／renderer／相対パス規約は従来どおり。`diagrams.sequence`にentry・上限・Java release・解析期限を設定できます。class／package専用flag・layout・`--no-cache`・sequence差分は対応していません。

実行できる両言語の例、YAML、動的呼び出し・例外・channelの読み方は[シーケンス図の利用例](examples/sequence/README.md)を参照してください。全フラグは`umlgen sequence --help`で確認できます。

## 単一関数アクティビティ図（開発版）

`activity`は、指定関数内の**分岐先・ループの接続先・終了先**をCFGとして描きます。低レベルの制御辺を失わないよう、PlantUMLの明示的なactivityノードとラベル付き接続を使います。入れ子のif／whileへ無理に変換せず、実行trace・値・到達可能性の証明とは区別します。

```bash
umlgen activity testdata/activity/go --language go \
  --function ProcessOrder --explain -o go-activity.puml
umlgen activity testdata/activity/java --language java \
  --function processOrder --format svg --renderer local -o java-activity.svg
```

- `--function`は一意な単純名・完全名・signature・`context::signature`。曖昧な指定では候補を表示して終了コード2にし、以前の図を保持します。
- if／else、式内の`&&`／`||`、ループのゼロ回・back edge・update、switch、fallthrough、break／continue、早期returnを扱います。Goのgoto／ラベルと組込みpanic、Javaのdo／while・ラベル・明示throw・三項演算にも対応します。呼び出しは単一関数内のactionで、呼び出し先へ展開しません。
- **未対応ノードは実行の終了ではなく、解析の境界です。** Goのdefer／recover・go・select・channel／iterator、Javaのtry／catch／finally／resource・synchronized・assert・switch式等は位置付きのterminal barrierと警告にし、そこから架空の正常継続辺を出しません。後続の構文ノードが非接続で残る場合もあります。
- 暗黙の例外／panic、初期化処理、スケジューリング、callee内部の実行は推定しません。関数リテラル／lambda／匿名classの本体を宣言位置で実行扱いにしません。
- 既定の`--max-nodes 500`にはentry／exitと合流ノードを含みます。超過は終了コード2で、途中までの図を成功出力せず既存図を保持します。必要なら明示的に上限を増やしてください。
- `--explain`は取得済みソースの位置・抜粋、ノードIDと種類、辺の接続先と種類、診断を表示します。描画時にソースを再読込しません。
- 構文／型エラーがあっても指定関数が利用可能なら部分図と診断を出して終了コード3。利用不能なら既存図を保持します。非対応警告だけなら終了コード0ですが、全経路の解析成功を意味しません。
- 解析境界はsequenceと共通です。Goは選択済みローカルソースのみ、Javaは**JDK 17以上**と信頼済みhelperを使い、対象コード・build・processor・依存downloadを実行しません。`--java-release`はJavaのみ、`--analysis-timeout`は既定30秒（Goは協調的な確認）です。
- 既定出力は`activity-diagram.puml`。共有output／renderer／exclude／設定相対パスに対応します。`--no-cache`・focus・layout・class／package／sequence専用flagとactivity差分は対応しません。

```yaml
diagrams:
  activity:
    function: processOrder
    max_nodes: 500
    java_release: 17
    analysis_timeout: 30s
```

CLI指定は設定より優先します。未知のnamespace／キーや型違いは設定エラーです。全フラグは`umlgen activity --help`で確認できます。

## 明示状態遷移図（開発版）

`state`は、YAMLで明示した状態機械と、指定した型・フィールドのソース代入候補を照合します。**enumの列挙や変数名から、実行時の状態機械を自動復元する機能ではありません。**

```bash
umlgen state testdata/state/go --language go \
  --state-config testdata/state/go/machine.yaml --explain -o go-state.puml
umlgen state testdata/state/java --language java \
  --state-config testdata/state/java/machine.yaml \
  --format svg --renderer local -o java-state.svg
```

状態定義は通常の`.umlgen.yaml`とは別の、厳密な`version: 1` YAMLです。以下はGoのサンプルに対応します。Javaでは定数名を`NEW`／`OPEN`／`CLOSED`等の実ソースに合わせます。

```yaml
version: 1
type: Connection
field: state
initial: closed
states:
  - name: closed
    value: Closed
  - name: open
    value: Open
transitions:
  - from: closed
    to: open
    event: Open
    guard: connection accepted
    action: open transport
```

- 実線と初期マーカーは`configured`な宣言です。guard／actionは表示用文字列で、任意コード・シェル・条件式を評価しません。初期状態は必須で、Goのゼロ値やJavaの初期化から推定しません。
- 型は一意な単純名・完全名・`context::qualified-name`で選び、直接宣言されたフィールドを指定します。名前だけのフィールド照合はせず、別の型・隠されたフィールド・ローカル変数を混同しません。`--type`／`--state-field`で明示的に上書きできます。
- `states[].value`は任意です。指定すると、取得済みソースに束縛された定数／enum member／対応する観測literal・定数式へ、canonical ID→完全名→単純名→literal表記の順で対応付けます。Goは`go/types`の定数値を使用し、Javaは定数参照・literal・対応する括弧／単項式に限定します（一般の算術式は未知値として保持）。未知・曖昧な指定、同じ値への複数状態の束縛はエラーです。省略した状態は設定由来のままで、ソースの値を補いません。利用可能なselectorとkeyは`--explain`で確認できます。
- **破線は、直前状態が不明なソース代入候補です。** lexical guardが一致しても遷移元・event・同一実行インスタンスを証明しません。代入候補から全状態への辺を作ったり、設定の実線を自動追加したりしません。同じ代入先の候補は1本の破線に位置とCFG coverageを併記します。
- 実行時の値が不明なRHSは未対応付け候補として表示します。別関数・constructor・lambda・alias・並行更新や初期化の範囲も診断します。CFG上で非接続の代入や未対応barrier内の書込みを「実行された」としません。Goのcomposite initializerは候補を残しますが、専用の書込みノードがないため親のreturn／assignmentへ便宜的に対応付けません。
- `--max-nodes`は候補を含む**関数ごとのCFG**のhard capで既定500。class／package／sequence専用flag・layout・`--no-cache`・state差分は非対応です。複合／履歴／並行状態、時間制約、完全なライフサイクル検証も行いません。
- 定義の未知キー・型違い・未宣言状態・重複／矛盾した同一from／event／guardの遷移、型／フィールド／値の選択失敗、上限超過は終了コード2で既存図を保持します。解析可能な対象に構文／型エラーが残る場合は診断付きの図と終了コード3。画像失敗は従来どおり5で元のPlantUMLを残します。
- 既定出力は`state-diagram.puml`。JavaはJDK 17以上、Goは選択済みソースのみで、対象build・実行・processor・downloadなし。既存AST／宣言とCFGを共有し、別parse／二重compileはしません。Javaの同一完全名classが複数rootで競合する場合はjavac診断を維持し、不確かなフィールド束縛を作りません。

`.umlgen.yaml`の`diagrams.state`には`state_config`・`type`・`field`・`max_nodes`・`java_release`・`analysis_timeout`を設定できます。`state_config`は共有設定ファイルのディレクトリ基準、CLIで渡す`--state-config`は現在の作業ディレクトリ基準です。型／フィールドはCLI→`diagrams.state`→状態定義の順。既存output／renderer／exclude／期限の規約を維持します。設定と入力のSHA256、元ソースの位置・抜粋、CFG node IDとcoverageは`--explain`で辿れます。

## 明示コンポーネント図（開発版）

`component`は、既存のpackage依存を**利用者が明示した境界**へ集約します。package名から責務・サービス・実行時通信・配置を推定する機能ではありません。Javaも構文解析のみでJDK不要です。

```bash
umlgen component testdata/package/go --language go \
  --boundary-config testdata/component/go/architecture.yaml --explain -o go-component.puml
umlgen component testdata/package/java --language java \
  --boundary-config testdata/component/java/architecture.yaml \
  --format svg --renderer local -o java-component.svg
```

境界定義は共有設定とは別の、厳密な`version: 1` YAMLです。以下は独自プロジェクト用の例です。

```yaml
version: 1
components:
  - name: API
    match:
      - language: go
        package_prefix: example.test/app/api
        go_module: example.test/app
        source_root: ../src
    provides: [Orders]
    requires: [Storage]
  - name: Storage
    match:
      - language: go
        package_prefix: example.test/app/storage
        go_module: example.test/app
    provides: [Storage]
```

- 各selectorは必須`language: go|java`と、1つ以上の制約を持ちます。行内はAND、同じcomponent内の複数行はOR。`package_prefix`はGoの`/`・Javaの`.`の区切り単位で一致し、`api`は`api2`を含みません。glob・正規表現ではありません。
- `go_module`は取得した`go.mod`のmoduleパスとの完全一致で、Go専用です。Java module・Maven／Gradle・`go.work`・依存ライブラリ一覧は解析しません。
- `source_root`は境界YAMLからの相対パスまたは絶対パスで、**解析文脈の物理rootとの完全一致**です。Goはmodule root（moduleなしはpackage directory）、Javaは選択targetのroot（単一ファイルなら親directory）。任意のサブdirectory配下を選ぶ条件ではありません。symlink・同一filesystem上の大小文字aliasを同じ物理rootとして扱い、空白・角括弧等は文字どおりのパスです。存在しない通常の将来rootは未使用診断になります。
- 複数componentに所属するpackageは終了コード2で旧図を保持します。先勝ち・最長prefix優先はありません。同じcomponentの複数selectorに一致することは許容します。未割当packageは文脈別の独立ノード、未使用境界は空ノードと診断で残します。外部packageをcomponentへ所属させません。
- component内依存は根拠を保持して自己矢印を描かず、境界間の辺には元package／型関係／importを保持します。`--dependency-source declared-types|imports|both`の既定は`both`。型関係レコード数とimport宣言数は別単位で、実行回数ではありません。`--external hide|show`の既定は`hide`で、非表示にしても元根拠は説明に残します。未解決／曖昧参照から架空の端点を作りません。
- `provides`／`requires`は表示用の明示契約です。**同名でもproviderへ自動接続しません。** ソースから公開API・呼び出し・配線を推定せず、設定値・コード・build tool・processor・scriptを実行しません。
- `--explain`は境界YAMLの位置／SHA256、Go moduleの取得根拠、元package ID・型関係・推定根拠・importの位置／hash／取得済みソース抜粋、内部／非表示依存、未割当・未使用・元の解析診断を表示します。部分的な解析失敗はファイルごと除外し、診断付きの図と終了コード3。全件失敗は旧図を保持します。未解決・外部非表示・解析失敗を「依存なし」の証拠にしません。
- 既定出力は`component-diagram.puml`。SVG／PNG・renderer・期限・exclude・`--no-cache`に対応します。focus／include／cycles／relations／layout・component差分は非対応。manifestの配置宣言は別の`deployment`コマンドで扱います。

共有`.umlgen.yaml`では次のように設定します。CLIが設定を上書きし、`boundary_config`は共有設定ファイルのdirectory基準、CLIの`--boundary-config`は現在directory基準です。

```yaml
diagrams:
  component:
    boundary_config: architecture.yaml
    dependency_source: both
    external: hide
```

## 宣言された配置図（開発版）

`deployment`は、**manifestに宣言された希望構成**を描きます。実際の配置node・稼働replica・health・疎通・通信を観測する機能ではありません。ソース解析、Docker／kubectl／Helm／build tool／生成scriptの実行、クラスタ問い合わせは行いません。

```bash
umlgen deployment --manifest testdata/deployment/compose.yaml \
  --boundary-config testdata/deployment/architecture.yaml \
  --mapping-config testdata/deployment/compose-mapping.yaml \
  --explain --format svg --renderer local -o compose-deployment.svg

# このサンプルはselectorなしServiceを含むため、図を生成して終了コード3を返します。
umlgen deployment --manifest testdata/deployment/kubernetes.yaml \
  --boundary-config testdata/deployment/architecture.yaml \
  --mapping-config testdata/deployment/kubernetes-mapping.yaml \
  --explain --format png --renderer local -o kubernetes-deployment.png
```

### 対応する固定subset

- **Compose Specification:** 単一documentの`services`、宣言image、短縮／長形式のport、network所属と暗黙default、named volume mount、外部network／volume宣言、短縮／長形式`depends_on`の起動条件。`service_healthy`等は設定値で、health確認済みではありません。bind mountのhost pathや任意のdriver設定は図へ取り込みません。
- **Kubernetes:** core `v1`のPod／Service／List、`apps/v1`のDeployment／StatefulSet／DaemonSet、`batch/v1`のJob。multi-documentとList内のresourceを読み、kind・宣言namespace・nameで分離します。同一identityの重複はエラー。省略namespaceを`default`へ補完しません。container／init／ephemeral containerのimage、明示されたdesired replicas、Service／node portを表示します。
- **Service selectorは候補だけ。** 非空selectorと、同じ宣言namespaceのPod／template labelsの一致だけを破線候補にします。名前の類似、namespace越し、未展開labelから接続を作りません。selectorなし／空／一致なしは端点不明、ExternalNameは宣言DNS aliasで、実endpoint・DNS解決・network通信の証明ではありません。
- `${...}`・template・未指定build出力・未対応image参照は未解決のままです。`.env`／`env_file`／include／別manifest／Dockerfileを勝手に読みません。`extends`・profiles・network_mode等の未対応構成を診断し、未評価の継承からdefault networkを推定しません。Compose／Kubernetesの完全schema検証ではなく、build・image存在・APIへの適用可否も保証しません。
- 環境変数・Secret／ConfigMapの値、labelsの値、raw YAML抜粋を保持・表示しません。manifest／mappingのYAML alias／custom tag／重複keyは受理せず、構文・型エラーにも入力値を出しません。imageはliteral名／tagまたは64桁hexの`sha256` digestに対応し、他のdigest・credential風URL・未対応参照は安全なunknownにします。共有設定`.umlgen.yaml`の既存YAML mergeは維持します。

### componentとの明示対応

対応は任意です。省略するとcomponent／sourceとartifactの対応は不明のままです。`--boundary-config`は[component境界定義](#明示コンポーネント図開発版)から名前・位置・hashを読みますが、ソースを走査せず、`match`をmanifestへ適用しません。

`--mapping-config`には別の厳密な`version: 1` YAMLを指定します。mappingを指定する場合はboundaryも必要です。

```yaml
version: 1
bindings:
  - component: API
    target: {kind: compose-service, name: api}
  - component: Storage
    target: {kind: compose-service, name: storage}
```

Kubernetesではtargetを`{kind: Deployment, namespace: demo, name: api}`等とします。対応するkindは`compose-service`／Pod／Deployment／StatefulSet／DaemonSet／Job／Service。宣言済みcomponentと、取得済みmanifestの**完全一致するidentity**だけを結び、明示的な多対多対応は許容します。不明なcomponent／target・重複binding・未知keyはエラー。同名image／service／componentから自動対応せず、明示対応もsource→imageのbuild来歴を証明しません。未対応componentは空ノードと診断を残します。

### 設定・終了コード・説明

既定出力は`deployment-diagram.puml`。`--manifest-format auto|compose|kubernetes`、SVG／PNG、既存renderer・期限・title・quiet／verboseに対応します。位置引数によるproject走査、language／focus／include／exclude／cache／layout／差分flagは非対応です。入力は毎回、明示ファイルだけを取得します。

```yaml
diagrams:
  deployment:
    manifest: compose.yaml
    manifest_format: auto
    boundary_config: architecture.yaml
    mapping_config: deployment-mapping.yaml
```

設定内の各パスは共有設定ファイル基準、明示CLIパスは現在directory基準です。CLIが設定を上書きします。

- 正常0。未対応構成・未解決image／endpoint等は診断付き図と**終了コード3**。Secret／ConfigMapの意図的な値省略だけでは3にしません。
- 入力IO失敗1、構文・schema・重複identity・不明binding等は2で旧図を保持します。対応するtopologyが1件もない入力も2。出力失敗4、画像失敗5では生成済みPlantUMLと説明を残します。
- `--explain`はmanifest・boundary・mappingのSHA256、元document番号と行、各宣言・辺の種類と不確かさを表示します。機密値を含み得るraw snippetsは再読込・表示せず、端点不明や非対応箇所を「通信なし」の証拠にしません。

## 保存に追従するwatch（開発版）

`watch`はJava／Goの**class／package図**を継続再生成します。既定はclass。既存の解析キャッシュ・設定・図別flagを再利用し、ソースや任意のhook／buildを実行しません。

```bash
# classは省略可能。設定変更・追加・削除・renameにも追従
umlgen watch ./src --language go -o diagrams/classes.puml

# package図と画像も監視。ローカル画像生成にはPlantUMLが必要
umlgen watch package ./src --language java --dependency-source both \
  --format svg --renderer local -o diagrams/packages.svg

# 大きなscopeではポーリング間隔を調整
umlgen watch class ./src --interval 1s --debounce 300ms --verbose
```

- **Ctrl+Cで終了**します。進行中のrendererをキャンセルし、未公開stageを破棄して終了コード0で戻ります。通常のCLIと同じrenderer設定・期限・明示server送信規約を維持します。
- `--interval`は内容snapshotのポーリング間隔で既定`500ms`、正数。`--debounce`は最後の変更後の静穏期間で既定`250ms`、0以上。連続保存をまとめ、世代を並行実行しません。生成中の変更も次の観測で処理し、入力が変わっていない失敗を無限retryしません。
- 対象sourceの内容とファイル集合、共有設定ファイル（省略時`.umlgen.yaml`の新規作成も）、関係する`go.mod`とauto言語選択markerを監視します。同じmtimeを保った保存も内容hashで検知します。include／focusで非表示になる型も通常解析に必要なsourceなら対象です。exclude・Goのtest file除外は通常の規約を維持します。
- 設定は毎回再読込し、source範囲・output・renderer等の変更を適用します。新しい出力先へ成功図を公開しても、以前の出力先は勝手に削除しません。起動時は有効な設定と存在するtargetが必要ですが、空directoryは明示的な空図を生成できます。開始後の最後のsource削除やroot削除でも古い型を空図へ置き換えます。
- **完全成功した世代だけ公開**します。部分的なparse／読取失敗、設定エラー、画像失敗では、前回成功した`.puml`と画像を両方保持します。入力の修正で再開し、生成中に設定が壊れて同じ内容へ戻った場合も未公開世代を取りこぼしません。単発class／packageの部分図仕様は変更しません。未解決参照等の通常診断を解析成功と混同せず、図の限界は従来の説明に残します。
- 出力parent内の専用`.umlgen-watch-*` directoryで生成し、入力の再確認後に公開します。生成物・stage・非source拡張子のeditor一時ファイルは観測対象から外します。source／設定／moduleへの出力衝突は、未作成fileやparentのsymlink aliasも検査して拒否します。最終出力fileそのものがsymlink・directory等の場合も受理しません。
- 各fileの置換はatomicで、複数fileの途中失敗・キャンセルでは既に置換したfileを戻します。**同時readerに対する複数file一括のOS transactionではありません。** 新規fileはstageのumaskを、既存fileは現在のpermissionを維持します。
- `--quiet`は通常の更新通知を抑え、失敗はstderrへ残します。`--explain`・`--verbose`・`--no-cache`も通常どおり利用できます。watchの時間設定はCLI専用で、新しいYAML namespaceは追加しません。

変更がない回も、選択sourceの内容と関連metadataを読みます。大きなrepositoryでは対象／exclude／間隔を調整してください。Javaの構文解析はJDK不要ですが、画像はrenderer要件に従います。sequence／activity／state／component／deployment、Git diff、ブラウザ自動更新、任意commandの監視は対象外です。

## 解析キャッシュ

class／package／componentのJava／Go構文解析結果はOSのユーザーキャッシュ領域へ保存され、変更のない2回目以降の実行で再利用されます。キャッシュキーにはファイル内容、umlgenバージョン、言語別解析スキーマ、設定が含まれます。出力先だけを変えた場合は安全に再利用します。componentの境界・module情報は実行ごとに現在の入力へ適用します。sequence／activity／stateはこのキャッシュを使用せず、取得した現在の入力から毎回意味解析します。

Goではpackage節などの構文情報だけを再利用し、packageパスは各実行で現在の`go.mod`とソースの配置から決定します。同じソースを別moduleへ置いた場合や、`go.mod`のmodule名・配置ディレクトリを変更した場合も、以前のpackageパスを引き継ぎません。祖先に`go.mod`がなければpackage節の名前を使います。解析スキーマが変わった場合、旧エントリーは自動的にmissになります。

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

キャッシュはファイル単位で、package節・import宣言と位置・型／メンバー宣言・内容ハッシュを保持します。型を持たないJavaのpackage宣言ファイルやGoの関数ファイルも再利用し、複数型のimportを重複保存しません。ソース本文・AST・ソースファイルのパスは保存せず、ネットワーク通信も行いません。破損したエントリーは破棄して自動的に再解析します。

言語別キャッシュディレクトリは新規作成時に`0700`を指定し、umaskによる制限を維持します。既存ディレクトリのアクセス権限は変更しません。ソースの絶対パス解決が失敗した場合は、空のパスを解析対象に混ぜず、探索エラーとして返します。

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

開発版workflowでは、PR本文の`umlgen` blockから差分図の`--include`／`--relations`／`--depth`／`--direction`だけを安全に指定できます。本文編集で再生成するには、callerの`pull_request.types`に`edited`も含めます。全体図には本文フィルタを適用せず、適用引数・比較commit・本文hashをSummaryと`review-scope.json`へ残します。[構文・上限・失敗時の扱い](docs/github-actions.md#pr本文でレビュー範囲を指定する開発版)を確認してください。既存の参照先を対応workflowのcommitへ更新するまでは利用できません。

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
  # 省略時はclass-diagram.puml／package-diagram.puml
  # file: diagrams/model.puml
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

diagrams:
  package:
    dependency_source: declared-types
    external: hide
    cycles: mark
```

`language`は`auto`、`java`、`go`から選択できます。`auto`は対象ファイルと`go.mod`、`pom.xml`、Gradle設定から判定します。JavaとGoが混在して判定できない場合は、`--language`または設定ファイルで明示してください。

優先順位は、コマンドライン、`--config`で指定した設定、`.umlgen.yaml`、デフォルト値の順です。

## SVG／PNG出力

SVG・PNG出力にはPlantUMLレンダラーが必要です。デフォルトでは`plantuml`コマンドを探し、見つからなければ`PLANTUML_JAR`を使います。`renderer`を`server`に設定した場合のみ、生成済みPlantUMLをPlantUML Serverへ送信します。機密な型名・package名等を含み得ます。

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
  server_url: ""   # server使用時のみ（生成済みPlantUMLを外部送信）
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
| 5 | SVG／PNGレンダリングエラー（`.puml`は保持） |

## 現在の解析範囲

### Java

Javaの宣言構文はTree-sitterの構文木から取得します。特定のフレームワークやビルドツールに依存せず、一般的なJavaソースからクラス図を生成することを基本方針としています。明示的import、ワイルドカードimport、同一パッケージ、入れ子型を使ってプロジェクト内の型を解決します。

annotation宣言とその要素、enum定数、record、generic型パラメーター、abstract／final／sealed／non-sealed型、static／abstractメンバーを図へ反映します。generic型やwildcard型に含まれるプロジェクト内の型も、解決できる範囲で関係へ反映します。

型・メソッド・コンストラクターで宣言された型パラメーターは、そのスコープ内で同名の実型へ解決せず、未解決参照にも数えません。非staticな入れ子型は外側のパラメーターを引き継ぎ、static／暗黙staticな入れ子型とは区別します。実際の型引数や完全修飾名の参照は従来どおり解決対象です。境界の完全な型検査・型引数の代入を行うものではありません。

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
- `type X = ...`はaliasの構文情報として保持するが、独立したclassや偽の`value`フィールドとして図へ出さない。`type X string`などの名前付き定義型は従来の表現を維持する
- 型宣言とreceiverで宣言した型パラメーターを区別し、`Box[T]`のメソッドが`Box[U]`をreceiverに使っても、`U`を同名の実型へ結び付けない
- aliasの展開・alias経由のメソッド転送は行わないため、aliasを使った参照が未解決になる場合がある。宣言未取得のreceiverだけの部分解析と、既知のaliasへのreceiverは混同しない

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

`auto`レンダラーは、コマンド未検出と実行失敗を区別して表示します。`renderer was not found`の場合はPlantUMLのインストールとPATHを確認してください。`renderer failed`の場合は、併記された構文エラー・終了ステータス・タイムアウトなどの原因を確認してください。`PLANTUML_JAR`が設定されている場合のフォールバックは維持し、画像生成に失敗しても元の`.puml`ファイルは残します。

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
