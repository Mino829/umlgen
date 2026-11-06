# パッケージ依存図をソースから読む

開発版の`umlgen package`を使うJava／Goの例。リポジトリのルートで実行する。fixtureは循環・未解決参照・曖昧な名前を意図的に含む**構文解析用の入力**であり、コンパイル可能なアプリケーションではない。

## 生成する

```sh
go build -o /tmp/umlgen-package-example ./cmd/umlgen

/tmp/umlgen-package-example package testdata/package/java --language java \
  --dependency-source both --external show --explain \
  --output /tmp/java-packages.puml

/tmp/umlgen-package-example package testdata/package/go --language go \
  --dependency-source both --external show --explain \
  --output /tmp/go-packages.puml
```

画像は同じコマンドに`--format svg --renderer local`を追加し、出力先を`/tmp/java-packages.svg`などへ変更する。`--format png`も利用できる。ローカルの`plantuml`が必要。明示的にserverを選択しない限り、生成したPlantUMLを外部へ送信しない。

## 依存の種類を比較する

`--dependency-source`だけを切り替える。以下は`--external show`、既定の`--cycles mark`での実出力。辺は利用側→依存先を表す。

| 言語 | dependency-source | 表示package数 | 表示辺数 |
| --- | --- | ---: | ---: |
| Java | declared-types | 5 | 3 |
| Java | imports | 5 | 6 |
| Java | both | 5 | 6 |
| Go | declared-types | 5 | 4 |
| Go | imports | 6 | 6 |
| Go | both | 6 | 6 |

- `types`は一意な宣言型関係の数、`imports`はソース内のimport宣言数。参照回数・実行回数ではなく、両者を足した値も表示しない。
- 両言語の`domain → service`と`service → domain`は`both`でそれぞれ`types=1; imports=1`。同じ辺の根拠を2種類に分けて数える。
- `bootstrap → service`は`types=0; imports=1`。Javaの[`package-info.java`](../../testdata/package/java/bootstrap/package-info.java)、Goの[`bootstrap.go`](../../testdata/package/go/bootstrap/bootstrap.go)には型宣言がない。`declared-types`では辺が消えるが、解析したpackage自体は残る。関数本体は型依存として数えない。
- Goの`example.test/telemetry`はblank importから得た外部import path。`imports`／`both`でのみ表示される。依存先の存在・型・build互換性を確認したわけではない。

## 説明から根拠へ戻る

Javaの[`app/App.java`](../../testdata/package/java/app/App.java)では、9行目の`Service service`が`sample.app → sample.service`の型関係の根拠、3行目のimportがimport依存の根拠になる。`--explain`はそれぞれの位置・ソースhash・解析時の最大3行の抜粋を表示する。

一方、10行目の`Item`には`sample.domain.Item`と`sample.alternate.Item`の2候補がある。候補を表示し、勝手に片方へ型依存を結ばない。11行目の`Missing`と`package-info.java`の`missing.remote.Client`も未解決として残り、架空のpackage端点にはしない。wildcard importのpackage依存と、曖昧で解決できない型依存は別物。

Goの[`app/app.go`](../../testdata/package/go/app/app.go)と[`app/backup.go`](../../testdata/package/go/app/backup.go)は同じalias `model`を別のimport pathに使う。説明では`App.Item`が`domain.Item`、`Backup.Item`が`alternate.Item`を指し、ファイルごとのimport scopeが保たれる。

## 循環と表示範囲を分ける

```sh
# domain/serviceの循環グループだけを残す
/tmp/umlgen-package-example package testdata/package/java --language java \
  --dependency-source both --cycles only --explain \
  --output /tmp/java-cycles.puml

# domainだけを表示。解析範囲の循環が消えたことにはならない
/tmp/umlgen-package-example package testdata/package/java --language java \
  --dependency-source both --focus sample.domain --depth 0 --explain \
  --output /tmp/java-focus.puml
```

`domain`と`service`が1つのSCC（強連結成分）になる。focusで片方を隠しても、選択した解析範囲での循環と既知の非表示packageを説明する。SCCのメンバー一覧は呼び出し順ではない。Goではcanonical名を`example.test/packagefixture/domain`などに置き換える。

## Gitの両側から根拠の変化を読む

開発版の`diff --diagram package`はGitの完全な両snapshotを比較する。既存repoを変更せず、一時repoに「同じ辺を支える2ファイル」を作る例:

```sh
demo=$(mktemp -d)
git -C "$demo" init
git -C "$demo" config user.name "umlgen example"
git -C "$demo" config user.email "umlgen@example.test"
mkdir -p "$demo/app" "$demo/domain"
printf 'package domain;\npublic class Model {}\n' > "$demo/domain/Model.java"
printf 'package app;\nimport domain.Model;\nclass First { Model model; }\n' > "$demo/app/First.java"
printf 'package app;\nimport domain.Model;\nclass Second { Model model; }\n' > "$demo/app/Second.java"
git -C "$demo" add .
git -C "$demo" commit -m "two dependency proofs"
base=$(git -C "$demo" rev-parse HEAD)

rm "$demo/app/First.java"
git -C "$demo" commit -am "remove one proof"
(
  cd "$demo" &&
  /tmp/umlgen-package-example diff "$base..HEAD" --diagram package \
    --dependency-source both --explain --output /tmp/package-proof-change.puml
)
```

`app → domain`は**辺を保持した根拠変更**として青く表示され、beforeは`types=2; imports=2`、afterは`types=1; imports=1`になる。`Second.java`は変更されていないが、両snapshotの解析で残る根拠として数える。説明では削除根拠の`First.java`を比較元Gitの本文から抜粋する。`Second.java`も削除してcommitすると、最後の根拠が消えて初めて辺が赤の`removed`になる。

単一revisionのpackage差分は`revision..HEAD`、`A..B`は両commit、`A...B`はmerge-baseとB。dirty／staged／untrackedは`go.mod`も含めて除外する。過去の設定ファイルを自動適用せず、現在のCLI／YAML条件を両側へ同じように適用する。Goでは`--language go`を指定でき、同じソースバイト列でも各Git revisionのmoduleへ束縛する。

行移動・hash変更だけなら依存変更にしない。ファイルrenameは同じ辺の根拠変更、package／module名やmoduleディレクトリの変更はidentityの削除＋追加として扱う。renameの推定統合は行わない。

### 循環解消と不確かさ

`--cycles only`でも比較は表示を絞る前に行う。どちらかのsnapshotで循環メンバーだったpackageのunionを表示し、循環が解けた後も残っている辺を削除扱いにしない。

部分解析失敗では不確かな差分と説明を生成し、終了コード3を返す。片側の選択sourceが全件失敗なら前回図を保持する。未解決／曖昧な参照があるpackageの辺や関係する循環も`uncertain`として扱うため、参照は残っているのに型解決ができなくなっただけの状態を「依存削除」「循環解消」と断定しない。未解決診断だけの場合は構文解析失敗ではなく、終了コード0で診断と不確かさを表示する。

SVG／PNG・`--explain`・関係選択・外部表示・レイアウトは利用可能。package差分の`--focus`／`--depth`／`--direction`は未対応として明示エラーにする。既定の`diff`または`--diagram class`は従来のクラス差分のまま。全規約は[差分機能](../../docs/pr-diff.md#パッケージ依存の差分開発版)を参照。

## 保証しないこと

Java classpath／module解決、Go build tags、外部ソース本文、コンパイラーの型検査、実行時呼び出しは扱わない。Goの循環もコンパイル可否の判定ではない。未知の型を「存在しない」とせず、表示上の依存0件をリポジトリ全体の依存なしと解釈しない。

全体package図では部分解析失敗に診断を添えて成功した入力の図を生成する。全件失敗は終了コード3で既存出力を保持する。画像のタイムアウト・空画像・レンダラー失敗は終了コード5で`.puml`を保持する。package差分では上記のとおり部分解析失敗も終了コード3となる。

受入回帰は`go test ./internal/cli -run 'TestPackage(CLI|Diff)'`、全体package図の性能測定条件と結果は[性能基準](../../docs/performance.md)を参照。package差分の速度保証は追加していない。
