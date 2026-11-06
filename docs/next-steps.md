# 次にやること

確認日: 2026-11-06

## 完了済み

1. **PR #40 を main にマージ** — 型なし差分のプレースホルダ生成、終了コードの整理、ワークフロー側の旧リリース対応を含む。
2. **v0.5.0 をリリース** — Linux／macOS arm64・amd64／Windows の配布アーカイブ、チェックサム、GitHub Release を公開済み。
3. **再利用ワークフローの既定 CLI 版を更新** — `.github/workflows/umlgen-pr-diff.yml` の `umlgen-version` 既定値を `v0.5.0` に変更。`v0.4.0` 向けのフォールバック処理は、明示的に旧版を指定した場合の互換性のため残している。
4. **Issue / ロードマップを整理** —
   - #11 PNG出力・レンダラー設定 → 完了として close
   - #16 ロードマップ → v0.4.0 / v0.5.0 完了、v0.6.0 / v0.7.0 に再編
   - #20 / #27 → 現行の OSS 機能・互換性・品質優先の計画と合わないため `not planned` で close
5. **ドキュメントを更新** — `review-findings.md`, `github-actions.md`, `pr-diff.md`, `macos-setup.md`, `windows-setup.md`, 開発版バージョン表記を `v0.5.0` に合わせて更新。

## 実装・PR状況（未リリース）

- **レンダラーの残存不具合 — [PR #62](https://github.com/Mino829/umlgen/pull/62)** — mainへマージ済み。タイムアウト後の出力パイプ待機と0バイト画像の成功扱いを修正。
- **`--explain`（Issue #12）— [PR #63](https://github.com/Mino829/umlgen/pull/63)** — mainへマージ済み、Issue #12は完了。継承・実装・フィールド・引数・戻り値の解決先、メンバー、ファイル、宣言行を表示し、複数根拠とJava／Goの宣言位置を保持。通常のPlantUML出力は維持。
- **確認済み** — `go test ./...`、`go vet ./...`成功。Java／Goの実CLIで宣言位置、コールド／ウォームキャッシュの説明一致、図の不変性、関係種別・focusによる絞り込み、差分図・削除ファイル・プレースホルダを確認。`class --help`／`-h`が引数エラーになる既存不具合も修正し、`--explain`のヘルプ表示を確認。
- **パッケージグルーピングとレイアウト（Issue #13）— PR #64／[PR #65](https://github.com/Mino829/umlgen/pull/65)** — mainへ統合済み。flat／hierarchical／none、縦／横方向、CLI・設定ファイル、安全なskinparam、Windowsのgolden改行対策を含む。
- **リモートCI** — PR #62／#63のLinux、Windows、macOS amd64／arm64およびCodeQL等のチェック成功を確認。
- **Windowsのgolden比較** — 初回CIでチェックアウト時のCRLF変換による不一致を検出。`.gitattributes`で対象fixtureをLFに固定し、`core.autocrlf=true`でも4つのfixtureのバイト列が変わらないことを確認。比較条件は緩めない。
- **main統合候補の確認** — `go test ./...`、`go vet ./...`成功。実CLIでJavaのSVG・GoのPNGを`--explain`と階層・横方向・skinparam併用で生成。コールド／ウォーム説明一致を確認。模擬レンダラーの1秒期限は約1.12秒で終了コード5、空画像も終了コード5となり、説明と`.puml`は保持。
- 未リリース。機能の拡張順序は、次節の合意済み開発方針に従う。

## 次の作業

合意した優先順位は、**学習者のコード理解 → チームの設計レビュー → 開発中の継続利用 → エディタ・画面操作**。詳細と最初の完了条件は[`development-direction.md`](development-direction.md)を参照。

既存の`--explain`を拡張する学習者向けの関係説明は[PR #66](https://github.com/Mino829/umlgen/pull/66)でmainへマージ済み。関係の意味、解析時のソース抜粋、Goの暗黙実装を支えるメソッド、範囲・失敗・未解決・既知の非表示参照を扱う。実CLIとテストの確認結果は[`development-direction.md`](development-direction.md)に記録。関係線の変更と根拠の変化を扱うチーム向け機能を検討し、watch／導入改善はその後に進める。リリースの実施は別途判断する。

直近のゴールは、PR #73の提案を受けた**PR本文からレビュー範囲を指定して安全に差分図を生成する導線**。開発版では、checkoutしない検証job、4種類の許可flagと上限、JSON→NUL区切り引数、本文編集トリガー、古いrunの取消、Summaryと`review-scope.json`を接続した。差分図だけに条件を適用し、head全体図は従来の文脈として維持する。配布版CLI v0.5.0で動作し、未リリースのCLIをPRソースからbuildしない。

確認済み: parserの正常／異常／非実行回帰、全体テスト・vet・actionlintが成功。SHA-256検証済みの実配布版CLIとPlantUML JARで、本文の条件による差分図・SVG、full図のバイト不変、変更なし・type-less・全削除・解析失敗とreceipt／Summaryを確認した。実GitHubの本文編集時の再生成・取消とCIを確認し、PR作成とURL報告まで行う。Go対象・全体図focus・watch・activity・GUIは今回の範囲に含めない。

図の拡張計画のP1-08以降はPR #71等の別ブランチで進行しており、以下はこのmain系統で確認した状態を記録する。本文フィルタの実装と未マージの図拡張を混同しない。

図の拡張計画の **P1-01（Goキャッシュのmodule文脈分離）**、**P1-02（ファイル単位の解析結果・import位置）**、**P1-03（PackageIDと依存根拠の集約）** は開発版で実装済み。

`internal/packagegraph`は、文脈を分離したpackage ID、型宣言／import別の根拠・件数、内部／外部／未解決／曖昧候補を保持する。Javaの所有型・wildcardの曖昧性、Goのファイル別aliasと実package名を扱い、推定実装先と通常のalias参照も混同しない。実ファイルの解析で全3modeのcold／warm／キャッシュ非使用一致、既存class CLIの図・説明とJava golden一致を確認。`go test ./...`、`go vet ./...`成功。

**P1-04（循環検出とpackage範囲抽出）** と **P1-05（package PlantUML生成）** も開発版で実装済み。通常／推定根拠を区別するSCC、関係選択後のin/out/bothとhop、cycles-only・外部非表示、focusで隠れた循環を保持する。図は別の生成APIでpackage端点・種類別件数・循環とscopeの凡例を描き、同名別文脈と未信頼文字列を安全に表示する。実Java／Goソースから17シナリオのSVG／PNGと文書の実出力例をPlantUML 1.2026.6で生成・確認。`go test ./...`、`go vet ./...`、既存class CLIのcold／warm／`--no-cache`とJava golden一致も確認した。

**P1-06（`package`コマンドとYAML接続）** と **P1-07（packageの根拠・解析範囲の説明）** も開発版で実装済み。`umlgen package`から全3mode、循環・外部表示、package focus、関係選択、SVG／PNG、`--explain`を利用できる。共有する探索・構文キャッシュ・出力処理を既存class経路から限定抽出し、図別の既定ファイルと新YAML namespaceの検証を追加した。

2026-11-02の確認: Java／Goの全3modeで実CLIのcold／warm／`--no-cache`説明一致と通常図の不変性、型0件のimport、推定実装・focusで隠れた候補循環、設定優先度・相対パス・exclude置換、部分／全件解析失敗・読取失敗を確認。実local PlantUMLでSVG／PNGを生成し、1秒期限と0バイト画像は終了コード5で`.puml`と説明を保持した。既存classのcache／layout／explainとJava golden一致、実Git差分の追加・変更・削除およびGit base抜粋も確認。`go test ./...`、`go vet ./...`成功。

**P1-08（受入検証の集約と利用例・性能比較）** も開発版で実装・検証済み。`testdata/package/{java,go}`とCLI受入回帰に、依存mode別の端点・辺・件数、曖昧／未解決、型0件のimport、Goのファイル別alias、循環を集約した。[利用例](../examples/package-diagrams/README.md)と[性能比較](performance.md)を追加し、Phase 1Aの完成条件を確認した。P1-08では新しい性能保証やpackage差分を追加せず、次のPhase 1Bへ分離した。

**Phase 1B（P1B-01〜04、完全snapshotに基づくpackage差分）** も開発版で実装・検証済み。`diff --diagram package`が両commitの全選択ソースと各go.modをGitから固定し、未変更ファイルに残る根拠、一部／最後の根拠削除、同件数の入替、file rename、package／module identity変更を区別する。dirty／staged／untrackedを含めず、既存class diffの既定は変更しない。比較後のexternal／cycles-only投影で偽の辺削除を作らず、解析失敗・未解決／曖昧参照に関係する辺／循環を不確かとして説明する。

今回の確認: Java／Goの全3依存modeでcold／warm／`--no-cache`とexplain有無の図一致、実local SVG／PNG、部分／全件parse失敗、模擬rendererの期限・空画像・異常終了、Go module変更と汚れた作業ツリーの分離、削除target・片側空・merge-base、行移動とfile renameを実CLIで確認。Java goldenと既存class／class diffの互換性、`go test ./...`、`go vet ./...`も成功。他OS CI・マージ・リリース・package差分の性能保証は未実施。契約と利用例は[差分機能](pr-diff.md#パッケージ依存の差分開発版)と[開発計画](uml-expansion-proposal.md)を参照。

**Phase 2（静的シーケンス図）** も開発版で実装・検証済み。Goは選択済みローカルソースの`go/types`、JavaはJDK 17以上の`JavacTask.parse/analyze`を採用し、対象のbuild・実行・annotation processor・依存downloadを行わない境界を固定した。`sequence --entry`、一意なselector、制御領域、直接呼び出し展開、動的候補、go／defer／channel、再帰・上限・非対応箇所の明示、取得済みソースによる説明、専用YAML・rendererを接続した。

2026-11-05の確認: 両言語の実CLIとlocal SVG／PNG、引数の呼び出し順・短絡・分岐・loop、動的候補の非展開、Goの非同期／defer登録・channel相手不明、Javaの例外barrierと後続省略、各上限・再帰・overload、外部依存不足・壊れたentryによる旧図保持を確認した。javacの構文回復が作った仮のentryで旧図を上書きする不具合を修正し、回帰を追加した。JDK不在時の既存Java class、空PATHでのGo解析、Java期限と継承pipe待機、renderer異常・期限・空画像、YAML相対パスと優先順位、文字列のPlantUML無害化も確認。既存Java golden、Java／Goのclass／package cache・説明、全package mode、実Gitのpackage差分と既定class diffは維持。`go test ./...`、`go vet ./...`成功。環境・実行例・限界は[利用例](../examples/sequence/README.md)と[開発計画](uml-expansion-proposal.md)に記録した。

PR公開前の追加修正: Goのパーサー回復宣言・閉じbrace欠落・内部リテラルをentry／候補から除外し、妥当な別関数と型エラーだけの部分解析は維持した。シーケンス投影では全代替経路が非対応barrierの場合の後続漏れ、未知eventの無表示打切り、空本体の深さ上限による偽の省略を修正。package差分ではGit replacement objectによる固定SHAの内容すり替えを無効化し、未対応module入力によるpackage追加・削除も不確かにした。実CLIで再現・結果を確認し、全体テスト・vet・build成功。両言語のSVG／PNGとJava class golden一致を再確認した。4 OSのCIにJDK 17と両言語の実CLI生成smokeを追加し、smoke shell blockはローカルmacOSで実行した。

Java protocolの追加修正: JDK 17／Windowsの非UTF-8既定charsetを想定した実JVM検証で、日本語メソッド名が`??`になる問題を再現。helperの出力writerをUTF-8固定にし、日本語entry選択・呼び出しIDを保持する回帰を追加した。

2026-11-06のCI確認: [PR #71](https://github.com/Mino829/umlgen/pull/71)はLinux／macOS arm64・amd64が成功し、Windowsのpackage差分受入テストだけが失敗した。[失敗ログ](https://github.com/Mino829/umlgen/actions/runs/42030235476/job/127254532622)では、fixtureの期待パスが`RUNNER~1`形式、実際のGit根拠・診断が同一ディレクトリの`runneradmin`形式だった。fixture作成時に`filepath.EvalSymlinks`で物理rootへ揃え、元ファイル・削除根拠・解析失敗の厳密な位置検証を維持した。解析本体・出力・終了コードは変更せず、Windowsだけのskipやassertの緩和も行っていない。対象回帰、全体テスト、vet、buildが成功し、Java／Goの実CLIでroot aliasを使った削除・部分失敗の根拠とcold／warm／no-cache一致を確認した。Windows実行の最終結果は後続PRのCIで確認する。

共有する作業規約を[`CONTRIBUTING.md`](../CONTRIBUTING.md)へ追加した。実装依頼は検証・コミット・push・PR作成とURL報告までを完了条件にし、手元変更の保持、依存PRの明記、修正対象CI失敗の解消、無断マージ・リリースを行わないことを定めた。既存のignore方針を維持し、ローカル専用AGENTS.mdからこの規約を参照する。PR #69のマージを確認し、PR #71のbaseはmainへ更新済み。

次は **Phase 3A（単一関数アクティビティ図）** の開始条件を揃える。CFGの分岐・終了・短絡・例外辺とAST位置の対応を固定し、break／continue・finally／defer等を正常経路へ平坦化しない契約から進める。sequenceは実行traceや完全なdispatch解決ではなく、CFG・activity以降も実装済みとは扱わない。PR #71のWindows修正確認・マージ・リリース・sequence性能保証は未完了。リリースと配布物の検証は別途判断する。

## 進め方

- `--explain` とレイアウト設定は別 PR にする
- 各 PR では Java と Go の代表的な fixture、CLI ヘルプ、README、PlantUML 出力の回帰テストを更新し、macOS・Linux・Windows の CI を通す
- 新機能ごとにロードマップ #16 のチェック欄を更新する
