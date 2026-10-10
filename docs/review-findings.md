# Code review findings

確認日: 2026-10-08

## 型参照の解決

**優先度: P2**

**状態: 修正済み（main）**

対象: `internal/relations/relations.go` (`Index.Resolve`)

完全修飾名としての参照が解析対象に見つからない場合でも、importの型名と参照の単純名が一致すると、そのimport先へ解決されることがあります。たとえば`other.Bar`が解析対象に存在せず、`com.acme.Bar`がimport・解析済みの場合、`other.Bar`を`com.acme.Bar`への依存として図示する可能性があります。

参照の修飾名がある場合はその修飾関係を保って解決し、解決できない型を別の同名型へ結び付けないようにします。

対応: 修飾名が異なるimport先との単純名一致、およびimportされていない型への全体検索による解決を廃止しました。明示的にimportした外側の型を通じた入れ子型の解決は維持しています。

## PlantUML aliasの衝突

**優先度: P2**

**状態: 修正済み（main）**

対象: `internal/plantuml/generator.go` (`safeAlias`)

alias生成時に`.`などの記号を`_`へ置換するため、異なる完全修飾名が同じaliasになることがあります。例: `a_b.c.Widget`と`a.b_c.Widget`はいずれも`T_a_b_c_Widget`になります。衝突すると型宣言や関係線の接続先が曖昧になります。

生成したaliasの一意性を保証し、衝突時にも実行ごとに変わらない識別子を付けます。

対応: すべての型の基本aliasを先に予約し、衝突する型には完全修飾名の順序に沿って一意な連番を付けます。

## 2026-10-09: PR差分図変更のレビュー

以下はPR差分図のレビュー指摘と対応です。

### ワークフローが修正前のリリースを使う

**優先度: P1 / 状態: 修正済み（main / v0.5.0）**

対象: `.github/workflows/umlgen-pr-diff.yml` の `umlgen-version` 既定値

ワークフローの既定値は`v0.4.0`のままリリースされており、`v0.4.0`のCLIは変更されたJavaファイルに型がない場合にエラーを返します。そのため、`package-info.java`だけを変更したPRでは、既定値のままだとワークフローが失敗していました。

対応方針: 新しいCLIを既定で使うよう`v0.5.0`へ更新しつつ、現行リリースを明示的に指定している場合にも動くよう、ワークフロー側にプレースホルダ処理を設けます。

対応: 既定値を`v0.5.0`に更新しました。`v0.4.0`が型なしエラーの1行だけを出した場合に限りプレースホルダを生成します。解析警告や他の終了コード1のエラーは失敗のまま扱い、Java変更がある場合の`no-java-changes`は`false`にします。

### 型のないファイルだけを削除するとエラーになる

**優先度: P2 / 状態: 修正済み（main / v0.5.0）**

対象: `internal/cli/cli.go` の全ファイル解析失敗判定

削除された`package-info.java`だけが差分にある場合、現在の対象ファイルは0件で、削除ファイルからも型は得られません。`len(types) == 0 && warnings == len(files)`が`0 == 0`で成立し、プレースホルダを生成する前に終了コード3を返します。

対応方針: 解析対象が0件のケースと、対象ファイルをすべて解析できなかったケースを区別します。削除された型なしファイルを使った回帰テストも追加します。

対応: 差分モードでは型なしの削除ファイルをプレースホルダに進めるようにし、回帰テストを追加しました。

### プレースホルダの説明と実際の終了条件が異なる

**優先度: P2 / 状態: 修正済み（main / v0.5.0）**

対象: `docs/pr-diff.md` の「表示可能な変更型がない場合」、`internal/cli/cli.go` のファイル数・解析失敗判定

ガイドは、変更型をすべて除外した場合や、変更ファイルがすべて構文エラーの場合も終了コード0になると説明しています。しかし、除外後の対象ファイルが0件ならファイル探索の段階で終了コード1、解析した全ファイルが構文エラーなら終了コード3です。

対応方針: 意図する成功条件を決めて実装とガイドを一致させ、両方のケースをテストで確認します。

対応: 対象ファイルが0件なら終了コード1、変更ファイルを解析できず表示可能な型がないなら終了コード3、型なしファイルを解析できた場合はプレースホルダで終了コード0とする条件をガイドに明記しました。変更ファイルの解析失敗テストも追加しました。

### プレースホルダ図のタイトルに改行が残る

**優先度: P2 / 状態: 修正済み（main / v0.5.0）**

対象: `internal/cli/cli.go` の `placeholderDiagram`

`--title`をそのままPlantUMLの`title`行に挿入しています。通常の図生成では改行と復帰を空白へ置換しているため、型がない場合だけ複数行のタイトルで図が壊れたり、別のPlantUML行として解釈されたりします。

対応方針: 通常の図生成と同じようにタイトルの改行を置換し、出力を確認します。

対応: プレースホルダでも改行・復帰を空白へ置換し、テストを追加しました。

### 追加型の色名がガイドと異なる

**優先度: P3 / 状態: 修正済み（main / v0.5.0）**

対象: `docs/pr-diff.md` の「色の意味」

ガイドには追加型の色を`#lightgreen`と記載していますが、`internal/plantuml/generator.go`の出力は`#palegreen`です。

対応方針: ガイドを実際の出力に合わせます。

対応: ガイドを`#palegreen`に修正しました。

## 2026-10-10: v0.5.0公開後のコードレビュー

対象: `v0.4.0..3e0de1c` のコード変更。上記の修正済み指摘は重複して数えない。以下は**未修正**の指摘であり、レビュー時点の`main`に残っている。

### P1: PNGを出力先に指定すると画像も`.puml`も得られない

**状態: 修正済み（main）**

対象: `internal/cli/cli.go` の出力先の拡張子処理、`internal/renderer/renderer.go` の `outputPath`

`--format png --output /tmp/umlgen-review-image.png`を指定すると、CLIは`.svg`だけを`.puml`へ置き換えるため、PlantUML本文を指定された`.png`へ書く。レンダラーも同じパスを画像の出力先として計算する。ローカルPlantUMLでは画像が作られないまま終了コード0になり、サーバーレンダラーでは元のPlantUML本文を画像で上書きする。元の`.puml`を保持するという仕様に反する。

再現: `umlgen class testdata/java/compatibility/project --format png --output /tmp/umlgen-review-image.png`を実行すると、`Generated /tmp/umlgen-review-image.png`が2回表示された。`file`ではそのファイルはASCII textで、対応する`.puml`は存在しなかった。

対応: CLIで`.png`出力先も`.puml`へ正規化するよう修正し、レンダラー側で入力`.puml`と出力画像パスが同じになる場合はエラーにした。`--format png --output <path>.png`のCLI回帰テストを追加した。

### P1: 新しいレンダラー関連フラグを空白区切りで指定できない

**状態: 修正済み（main）**

対象: `internal/cli/cli.go` の `normalizeClassArgs`

新設した`--renderer`、`--server-url`、`--render-timeout`が値付きフラグ一覧にない。引数の並べ替え時に値だけが位置引数へ移動し、次のフラグが値として解釈される。READMEで案内しているフラグの通常の指定方法が失敗する。`--renderer=local`のような等号付き指定なら回避できる。

再現: `umlgen class testdata/java/compatibility/project --renderer local --format plantuml --output /tmp/umlgen-review-flags.puml`は終了コード2で`class accepts at most one target`。`--render-timeout 1s`を同様に指定すると`invalid value "--format" for flag -render-timeout`となった。

対応: `normalizeClassArgs`の値付きフラグ一覧に`--renderer`、`--server-url`、`--render-timeout`を追加した。空白区切り指定と等号付き指定の両方を確認するCLIテストを追加した。

### P2: レンダラーが画像を作らなくても成功を報告する

**状態: 修正済み（main）**

対象: `internal/renderer/renderer.go` の `runCommand`

外部コマンドの終了コードだけを見て画像ファイルの存在を確認せず、予想したパスを返している。レンダラーが終了コード0で画像を生成しなかった場合、CLIは`Generated ...svg`または`Generated ...png`と表示して終了コード0を返す。

再現: 終了コード0で画像を作らない模擬`plantuml`をPATHへ置いてSVGを生成したところ、CLIは終了コード0で`Generated /tmp/umlgen-review-sleep.svg`と表示したが、そのファイルは存在しなかった。

対応: `runCommand`でコマンド実行前に古い出力ファイルを削除し、コマンド成功後に出力ファイルの存在と通常ファイルであることを確認するようにした。画像が生成されなかった場合はレンダリングエラーを返す。終了コード0で画像を作らないレンダラーのテストを追加した。

### P2: ローカル／JARレンダラーにタイムアウトが効かない

**状態: 修正済み（main）**

対象: `internal/renderer/renderer.go` の `renderLocal`、`renderJar`、`runCommand`

`Config.Timeout`を利用するのはHTTPクライアントだけ。ローカルコマンドとJARは`exec.Command`で起動され、停止期限がないため、レンダラーが固まるとCLIも待ち続ける。

再現: 2秒待って終了する模擬`plantuml`に対して`--renderer=local --render-timeout=100ms`で実行したところ、コマンドは約2.1秒後に終了した。

対応: `Render`でタイムアウト付きの`context.Context`を作成し、`renderLocal`/`renderJar`の`exec.CommandContext`へ渡すようにした。期限切れ時には「rendering timed out」エラーを返し、`.puml`は保持する。タイムアウトを確認するレンダラーテストを追加した。

## 今回の確認

- `env GOCACHE=/tmp/umlgen-go-cache go test ./...`: 成功
- `env GOCACHE=/tmp/umlgen-go-cache go vet ./...`: 成功
- 上記のCLI再現は現在の`HEAD`をビルドした実行ファイルで確認。テスト成功はこれらの入力条件を網羅していない。
