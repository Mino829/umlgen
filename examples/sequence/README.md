# 静的シーケンス図

Java／Goの指定entryから、ソース上の呼び出しと構造化制御領域を描く。実行trace・完全なCall Graph・実ビルドの検証ではない。公開版v0.6.0のCLIを使用でき、ソースからのビルドは必須ではない。

## 代表ソースを実行する

リポジトリrootで実行する。ビルドにはGo 1.25以上とCコンパイラ、Java sequenceには別途JDK 17以上の`java`が必要。画像にはlocal PlantUML等のレンダラーも必要だが、PlantUML用JREだけではJavaの意味解析はできない。

```bash
go build -o /tmp/umlgen-sequence-demo ./cmd/umlgen

/tmp/umlgen-sequence-demo sequence testdata/sequence/go \
  --language go --entry PlaceOrder --explain \
  -o /tmp/umlgen-go-sequence.puml

/tmp/umlgen-sequence-demo sequence testdata/sequence/java \
  --language java --entry PlaceOrder --java-release 17 --explain \
  -o /tmp/umlgen-java-sequence.puml

# 同じソースを実画像へ。生成済み.pumlも保持する
/tmp/umlgen-sequence-demo sequence testdata/sequence/go \
  --language go --entry PlaceOrder --format png --renderer local \
  -o /tmp/umlgen-go-sequence.png
/tmp/umlgen-sequence-demo sequence testdata/sequence/java \
  --language java --entry PlaceOrder --format svg --renderer local \
  -o /tmp/umlgen-java-sequence.svg
```

入力は[`testdata/sequence/go`](../../testdata/sequence/go)と[`testdata/sequence/java`](../../testdata/sequence/java)。どちらも外部project依存なしで解析できる。

| entry | 読み取れること／断定しないこと |
| --- | --- |
| Go `PlaceOrder` | `Input()`の後に`pricing.Total(...)`。`valid && Check()`の右辺は条件付き。if/else・loopを保持。`repo.Save`と`notify()`は動的候補で、本体を順次実行した図にしない |
| Java `PlaceOrder` | 引数中の`Input()`→`Pricing.Total(...)`、constructor、finalな直接呼び出しを展開。interfaceの`repository.Save`は候補、`Runnable.run`は動的対象のまま |
| Go `Exchange` | send／receive／selectを表示するが、channelの通信相手・待機解除・goroutine間の順序は補わない |
| Go `Closures`／Java `Lambdas` | リテラル／lambdaの作成と本体実行を分離。Goの即時呼び出しは展開するが、保存した関数値／lambdaを作成時に実行したことにしない |
| 両言語 `Recur` | 再帰を明示して打ち切る。無限に展開しない |
| Java `Exceptional` | try/catch/finallyを未対応barrierとして表示。finally後の`Store.Stored()`を通常の必須後続として描かず、位置付きの省略を残す |

Go `PlaceOrder`の`defer Cleanup()`は登録、`go Worker()`は非同期起動として表示する。両者の本体を同期展開せず、deferの終了時LIFO実行、goroutineのjoin・完了順序を生成しない。候補に`LocalRepository.Save`があっても、その`store.Stored()`を確定した呼び出しとして挿入しない。Javaはfinalな直接呼び出しの`LocalRepository.Save`だけを展開し、interface候補の同メソッドを重ねて実行した図にしない。

定数を含むソース式も表示上の根拠であり、採用分岐・実行値・実際のループ回数を評価した結果ではない。複数branchは代替経路で、表示メッセージ数は実行回数ではない。

## entryと上限

entryは一意な単純名、完全名、引数型を含むsignature、または`context::signature`。同名別root・overloadを最初の一致で選ばず、候補を表示して終了コード2にする。

```bash
# Overloadedは曖昧。int版を明示する
/tmp/umlgen-sequence-demo sequence testdata/sequence/java \
  --language java --entry 'sample.Checkout.Overloaded(int)' \
  -o /tmp/umlgen-overloaded.puml

# entry直下の呼び出しは残し、その本体の展開を止める
/tmp/umlgen-sequence-demo sequence testdata/sequence/go \
  --language go --entry PlaceOrder --max-call-depth 0 \
  --max-participants 8 --max-messages 5 --explain \
  -o /tmp/umlgen-bounded.puml
```

既定は深さ3・参加者30・メッセージ200。参加者上限にはentry自身も含む。上限・再帰・非対応は位置付きの省略として図と説明に残る。上限を超えて黙って切り捨てない。深さを下げても、選択ソース全体の型検査コストが定数になるわけではない。

## YAML

`.umlgen.yaml`の例。`source`と`output.file`は設定ファイル基準。CLIの明示指定はYAMLに優先し、`--exclude`は設定リストを置き換える。

```yaml
language: go
source: [./src]
output:
  file: ./artifacts/sequence.puml
  format: plantuml
diagrams:
  sequence:
    entry: PlaceOrder
    max_call_depth: 3
    max_participants: 30
    max_messages: 200
    java_release: 17
    analysis_timeout: 30s
```

`java_release`はJava解析時に使う。CLIの`--java-release`はJava専用。未知のnamespace・キー・誤った型は引数エラー。`output.file`が未指定またはnullなら`sequence-diagram.puml`を使い、class／packageの既定ファイルを上書きしない。明示した共有output設定は全コマンドで使うため、図ごとに出力先を分ける。

sequenceは毎回、取得済みの現在の入力から意味解析する。class／packageの構文キャッシュを意味情報として流用しない。`--no-cache`、`--include`、型／packageのfocus・relations・visibility・layout・skinparam flagは受け付けない。既存class／package用YAML設定はsequenceへ適用しない。`diff --diagram sequence`も未対応。

## 解析と実行の境界

- **Go:** 標準`go/parser`＋`go/types`。importは選択済みローカルpackageと`unsafe`のみ。標準ライブラリを含む他のimportのexport data・ソースはロードしない。`fmt`等が不足すれば診断付きの部分解析になる。goコマンド・`GOPACKAGESDRIVER`・module download・cgo・project buildは実行しない。build tags／GOOSによるファイル選別・`go.work`／replace等を含む実ビルド環境の再現は行わない。
- **Java:** JDK標準の`JavacTask.parse`／`analyze`。指定releaseをinstalled JDKがサポートしている必要がある。project classpath／sourcepath／processorpathは空、annotation processing・暗黙のソースロード・code generationを無効にした組込みhelperだけを実行する。JDKの標準API以外の外部依存はロードしない。Maven／Gradle・対象のmain／constructor／static initializer・annotation processor・pluginは実行しない。対象を実行するための`.class`は生成しない。ambientなJava option／classpath変数を引き継がず、helperのメモリ・出力と実行時間を制限する。
- **期限:** `--analysis-timeout`は既定30秒。Java helperの終了と継承された出力pipe待機を制限する。Goは解析段階間の協調的な期限確認で、単一の`go/types`呼び出しを強制中断するwall-clock保証ではない。画像生成の期限は別の`--render-timeout`。
- **範囲:** 選択外の本文・reflection・DI・proxy・runtimeのクラス・関数値の全フローを解決しない。動的候補は選択ソースの型情報に基づく代替であり、完全な候補集合や実際のdispatch先とは主張しない。未対応構文・制御移動・例外経路の先を正常な直列呼び出しに置き換えない。

## 根拠と失敗の読み方

`--explain`はentryと候補のcanonical ID、backend、取得したソースのhash・UTF-8 byte位置・行／byte列・抜粋、診断、各省略理由を表示する。説明は解析した入力そのものから作り、後でファイルを読み直さない。図のラベルは一意な相対パスと短縮名で読みやすくし、完全なidentityは説明に保持する。公開JSON出力APIではない。

| 終了コード | sequenceの意味 |
| --- | --- |
| 0 | 対応範囲の投影を生成。動的候補・未解決dispatch・位置付きの非対応警告は残り得る |
| 1 | Java helper不足・実行失敗・解析期限等。生成前なら既存図を保持 |
| 2 | 不正flag／設定、entry未指定・曖昧、解析エラーなしでentryが存在しない |
| 3 | 構文／型／読取エラー。entryが利用可能なら部分図＋診断、利用不能なら既存図を保持 |
| 4 | 出力失敗 |
| 5 | 描画失敗・期限・0バイト画像。生成済み.pumlと要求した説明は保持 |

Go／Javaとも構文エラーが重なる宣言をentryとして採用せず、パーサーが回復生成したsignature・本体・閉じ括弧で前回図を上書きしない。Goでは壊れた宣言内の関数リテラルも独立候補として漏らさず、EOFで閉じbraceが欠落した本体も拒否する。一方、妥当な別関数や構文は正しいが型情報が不足するentryは、診断付きの部分解析として利用できる。regular source fileのみを読み、symlink／特殊ファイルは診断する。解析の不完全さを「呼び出しが存在しない」と解釈しない。

解析はローカルで行う。明示的な`--renderer server`では、呼び出し式・ソース上の条件・型名等を含む生成PlantUMLを指定serverへ送る。機密ソースにはlocal／jar rendererを使う。

## 確認した範囲

Darwin arm64、Go 1.26.4、JDK 26.0.1（対象release 17）、PlantUML 1.2026.6で両言語の実CLI・SVG／PNGを確認した。動的候補を直列展開しないこと、引数の呼び出し順、短絡・分岐・loop、Goのgo／defer／channel、Javaの非対応例外barrier、各上限・再帰、部分／entry利用不能、helper／renderer異常、YAML優先順位、Unicode／CRLF、PlantUMLへの文字列注入防止を確認した。既存classのJava golden、class／packageのcache／explain、全package依存mode、実Gitのpackage差分・既定class差分の互換性も確認した。

PR公開前の追加確認では、Goの壊れたheader／body／EOFで旧図保持と妥当な別関数の部分解析、Javaの全代替経路が非対応barrierとなるif/elseから後続呼び出しが漏れないこと、空の直接呼び出し本体は深さ0でも不要な省略にしないことを実CLIで確認した。CIにはJDK 17を明示導入し、4 OSすべてでビルド済みCLIから両言語の代表図を生成するsmokeを追加した。追加shell block自体はローカルmacOSで実行済みで、他OSの結果は実際のCIを確認するまで成功扱いにしない。

Java helperのprotocol出力はJVM既定charsetに依存せずUTF-8固定。非UTF-8が既定になり得るJDK 17／Windowsを想定し、実JVMのstdoutをwindows-1252にした際の日本語entry名・直接呼び出しIDの破損を再現・修正した。UTF-8以外のJVMでも、入力のソース位置・identityを別文字列へ置換しない。

開発当時の個別確認では`go test ./...`と`go vet ./...`が成功。他OS CI・マージ・リリース・sequenceの性能保証は、この個別確認の対象には含めていない。
