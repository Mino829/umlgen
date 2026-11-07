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

PR #73の提案を受けた**PR本文からレビュー範囲を指定して安全に差分図を生成する導線**は、PR #74→#73を通してmainへ統合済み。checkoutしない検証job、4種類の許可flagと上限、JSON→NUL区切り引数、本文編集トリガー、古いrunの取消、Summaryと`review-scope.json`を接続した。差分図だけに条件を適用し、head全体図は従来の文脈を維持する。workflowは配布版CLI v0.5.0で動作し、PRソースをbuildしない。

確認済み: parserの正常／異常／非実行回帰、全体テスト・vet・actionlint、実配布版CLIとPlantUMLによる差分図・SVG、full図のバイト不変、変更なし・type-less・全削除・解析失敗。実GitHubの本文編集・取消、artifactの本文hash／head／filter一致も確認し、PR #74と統合後のPR #73は4 OSのCIが成功した。Go対象・全体図focus・watch・activity・GUIはこのレビュー導線に含めていない。

以下は同じ開発版へ統合するpackage／sequenceの実装・検証記録。図別の制限と、個別開発時点の確認、統合時の確認を区別する。新機能の配布リリースはまだ行っていない。

図の拡張計画の **P1-01（Goキャッシュのmodule文脈分離）**、**P1-02（ファイル単位の解析結果・import位置）**、**P1-03（PackageIDと依存根拠の集約）** は開発版で実装済み。

`internal/packagegraph`は、文脈を分離したpackage ID、型宣言／import別の根拠・件数、内部／外部／未解決／曖昧候補を保持する。Javaの所有型・wildcardの曖昧性、Goのファイル別aliasと実package名を扱い、推定実装先と通常のalias参照も混同しない。実ファイルの解析で全3modeのcold／warm／キャッシュ非使用一致、既存class CLIの図・説明とJava golden一致を確認。`go test ./...`、`go vet ./...`成功。

**P1-04（循環検出とpackage範囲抽出）** と **P1-05（package PlantUML生成）** も開発版で実装済み。通常／推定根拠を区別するSCC、関係選択後のin/out/bothとhop、cycles-only・外部非表示、focusで隠れた循環を保持する。図は別の生成APIでpackage端点・種類別件数・循環とscopeの凡例を描き、同名別文脈と未信頼文字列を安全に表示する。実Java／Goソースから17シナリオのSVG／PNGと文書の実出力例をPlantUML 1.2026.6で生成・確認。`go test ./...`、`go vet ./...`、既存class CLIのcold／warm／`--no-cache`とJava golden一致も確認した。

**P1-06（`package`コマンドとYAML接続）** と **P1-07（packageの根拠・解析範囲の説明）** も開発版で実装済み。`umlgen package`から全3mode、循環・外部表示、package focus、関係選択、SVG／PNG、`--explain`を利用できる。共有する探索・構文キャッシュ・出力処理を既存class経路から限定抽出し、図別の既定ファイルと新YAML namespaceの検証を追加した。

2026-11-02の確認: Java／Goの全3modeで実CLIのcold／warm／`--no-cache`説明一致と通常図の不変性、型0件のimport、推定実装・focusで隠れた候補循環、設定優先度・相対パス・exclude置換、部分／全件解析失敗・読取失敗を確認。実local PlantUMLでSVG／PNGを生成し、1秒期限と0バイト画像は終了コード5で`.puml`と説明を保持した。既存classのcache／layout／explainとJava golden一致、実Git差分の追加・変更・削除およびGit base抜粋も確認。`go test ./...`、`go vet ./...`成功。

**P1-08（受入検証の集約と利用例・性能比較）** も開発版で実装・検証済み。`testdata/package/{java,go}`とCLI受入回帰に、依存mode別の端点・辺・件数、曖昧／未解決、型0件のimport、Goのファイル別alias、循環を集約した。[利用例](../examples/package-diagrams/README.md)と[性能比較](performance.md)を追加し、Phase 1Aの完成条件を確認した。P1-08では新しい性能保証やpackage差分を追加せず、次のPhase 1Bへ分離した。

**Phase 1B（P1B-01〜04、完全snapshotに基づくpackage差分）** も開発版で実装・検証済み。`diff --diagram package`が両commitの全選択ソースと各go.modをGitから固定し、未変更ファイルに残る根拠、一部／最後の根拠削除、同件数の入替、file rename、package／module identity変更を区別する。dirty／staged／untrackedを含めず、既存class diffの既定は変更しない。比較後のexternal／cycles-only投影で偽の辺削除を作らず、解析失敗・未解決／曖昧参照に関係する辺／循環を不確かとして説明する。

2026-11-04の個別確認: Java／Goの全3依存modeでcold／warm／`--no-cache`とexplain有無の図一致、実local SVG／PNG、部分／全件parse失敗、模擬rendererの期限・空画像・異常終了、Go module変更と汚れた作業ツリーの分離、削除target・片側空・merge-base、行移動とfile renameを実CLIで確認。Java goldenと既存class／class diffの互換性、`go test ./...`、`go vet ./...`も成功。当時の確認には他OS CI・マージ・リリース・package差分の性能保証を含めていない。契約と利用例は[差分機能](pr-diff.md#パッケージ依存の差分開発版)と[開発計画](uml-expansion-proposal.md)を参照。

**Phase 2（静的シーケンス図）** も開発版で実装・検証済み。Goは選択済みローカルソースの`go/types`、JavaはJDK 17以上の`JavacTask.parse/analyze`を採用し、対象のbuild・実行・annotation processor・依存downloadを行わない境界を固定した。`sequence --entry`、一意なselector、制御領域、直接呼び出し展開、動的候補、go／defer／channel、再帰・上限・非対応箇所の明示、取得済みソースによる説明、専用YAML・rendererを接続した。

2026-11-05の確認: 両言語の実CLIとlocal SVG／PNG、引数の呼び出し順・短絡・分岐・loop、動的候補の非展開、Goの非同期／defer登録・channel相手不明、Javaの例外barrierと後続省略、各上限・再帰・overload、外部依存不足・壊れたentryによる旧図保持を確認した。javacの構文回復が作った仮のentryで旧図を上書きする不具合を修正し、回帰を追加した。JDK不在時の既存Java class、空PATHでのGo解析、Java期限と継承pipe待機、renderer異常・期限・空画像、YAML相対パスと優先順位、文字列のPlantUML無害化も確認。既存Java golden、Java／Goのclass／package cache・説明、全package mode、実Gitのpackage差分と既定class diffは維持。`go test ./...`、`go vet ./...`成功。環境・実行例・限界は[利用例](../examples/sequence/README.md)と[開発計画](uml-expansion-proposal.md)に記録した。

PR公開前の追加修正: Goのパーサー回復宣言・閉じbrace欠落・内部リテラルをentry／候補から除外し、妥当な別関数と型エラーだけの部分解析は維持した。シーケンス投影では全代替経路が非対応barrierの場合の後続漏れ、未知eventの無表示打切り、空本体の深さ上限による偽の省略を修正。package差分ではGit replacement objectによる固定SHAの内容すり替えを無効化し、未対応module入力によるpackage追加・削除も不確かにした。実CLIで再現・結果を確認し、全体テスト・vet・build成功。両言語のSVG／PNGとJava class golden一致を再確認した。4 OSのCIにJDK 17と両言語の実CLI生成smokeを追加し、smoke shell blockはローカルmacOSで実行した。

Java protocolの追加修正: JDK 17／Windowsの非UTF-8既定charsetを想定した実JVM検証で、日本語メソッド名が`??`になる問題を再現。helperの出力writerをUTF-8固定にし、日本語entry選択・呼び出しIDを保持する回帰を追加した。

2026-11-06のCI修正: PR #71のWindows受入失敗は、fixtureの`RUNNER~1`形式と実際のGit根拠・診断の`runneradmin`形式の差だった。PR #72で物理rootへ揃え、厳密な根拠検証を維持した。PR #72はPR #71へマージ済みで、更新後の[PR #71 CI](https://github.com/Mino829/umlgen/actions/runs/42171703393)は4 OSすべて成功。PR #73を反映したmainとの統合にテキスト競合はなく、統合treeでも全体テスト・vet・build、両言語sequenceのSVG／PNG、全package modeのcache／説明一致、immutable Git差分、Java class goldenの一致を確認した。Windows skipやassertの緩和は行っていない。

共有する作業規約を[`CONTRIBUTING.md`](../CONTRIBUTING.md)へ追加した。実装依頼は検証・コミット・push・PR作成とURL報告までを完了条件にし、手元変更の保持、依存PRの明記、修正対象CI失敗の解消、無断マージ・リリースを行わないことを定めた。既存のignore方針を維持し、ローカル専用AGENTS.mdからこの規約を参照する。PR #69のマージを確認し、PR #71のbaseはmainへ更新済み。

**Phase 3A（単一関数アクティビティ図）は[PR #75](https://github.com/Mino829/umlgen/pull/75)で実装・検証済み。全4環境CI成功、mainへマージ済み・未リリース。** `activity --function`がGo／Javaの同じ取得済みAST・宣言から選択関数のCFGを作り、位置付きノードと型付き辺を描く。if／switch／loop・式内短絡・早期return、break／continue・label、Goのgoto／組込みpanic、Javaのthrow／三項を扱う。try/finally・defer/recover等はterminal barrierで、未知の継続を正常終了にしない。既定500ノードのhard cap、専用YAML、説明、既存rendererを接続した。

Phase 3Aの確認: 全体テスト（22 package成功）・vet・build、両言語の実CLIとlocal SVG／PNG、短絡RHSの接続先・label付きtransfer・returnの終了先・非対応ノードの終端、UTF-8 selector、上限超過による旧図保持。Goの定数unsafe演算・generic channel range、Javaのclass literal／constructor referenceと継承pipe期限を修正し、回帰を保持。既存sequenceの実SVG、全package mode、Java class goldenの一致を維持。CI smoke shellをローカル実行し、[公開後のCI](https://github.com/Mino829/umlgen/actions/runs/42179247787)も4環境すべて成功した。

**Phase 3B（明示状態図とソース代入候補）は[PR #76](https://github.com/Mino829/umlgen/pull/76)で実装・4環境CI成功後、featureブランチへマージ済み。main統合は[PR #77](https://github.com/Mino829/umlgen/pull/77)でレビュー待ち・未リリース。** `state --state-config`にversion: 1の型・field・状態・初期状態・event／guard／action・遷移を指定し、configuredな実線と直前状態不明の破線候補を分離する。optionalなsource valueはcompiler定数／enum・観測literalへ意味上のkeyで束縛し、未知・曖昧・矛盾を拒否する。別型・shadowing・別rootを名前だけで混同せず、書込みの所有関数・lexical guard・位置・CFG coverageと未対応範囲を説明する。初期状態・遷移元・runtime instance・実行済み経路は推定しない。利用方法は[README](../README.md#明示状態遷移図開発版)を参照。

Phase 3Bの確認: 全体テスト（23 package成功）・vet・build、両言語の実CLI・local SVG／PNG、明示遷移と候補の分離、未知RHS、field／capエラーによる旧図保持。レビューで見つかったGoの別定義型field混同・未評価initializerの誤ったCFG対応・同表記定数式のID衝突・range target内literalの所属漏れ、Javaの括弧付きreceiverを失敗前／成功後の回帰で修正。Go composite initializerは候補を残し、専用ノードがないことを診断する。既存activity／sequenceの実SVG、全package mode、Java class goldenの一致、追加CI shell blockのローカル実行とactionlint成功を確認。#76のマージ先は既に#75がマージされた後のfeatureブランチだったため、mainにはまだ状態図が含まれない。#77で状態図だけのmain統合差分を分離した。

**Phase 4A（明示コンポーネント境界）は[PR #78](https://github.com/Mino829/umlgen/pull/78)で実装・4環境CI成功、レビュー待ち・未マージ・未リリース。** 厳密なversion: 1 YAMLからpackage prefix・exact Go module・exact物理source rootをAND／ORで照合し、重複所属をhard errorにする。未割当・空境界・外部を混同せず、内部／境界間／非表示依存に元package・型・import・推定根拠・位置・hashを保持する。provides／requiresは明示契約だけで同名providerへ自動接続しない。`component`・専用YAML・構文cache・renderer・説明を接続し、JavaもJDK不要。利用例と契約は[README](../README.md#明示コンポーネント図開発版)を参照。

Phase 4Aの確認: 全体テスト（24 package成功）・vet・build・actionlint、両言語の全3依存modeでcold／warm／`--no-cache`の図・説明一致、実local Go SVG／Java PNG、空PATHで両言語の生成成功。境界競合時の終了コード2と旧SVG／PlantUML保持、部分失敗ファイルの除外と終了コード3、全件失敗時の旧図保持を実CLIで確認した。レビューで見つかったsymlink親参照とcase-insensitive filesystemのroot混同を失敗前／成功後で修正し、将来rootとWindowsの区切りを含む回帰を保持。既存class／packageの両言語cache／説明一致、sequence／activity／state／componentの実CI shell成功を確認した。main統合PR #77のheadをbaseとするレビュー用後続PRとして提出し、公開後のCIを確認する。

**Phase 4B（宣言された配置図）は本後続ブランチで実装・ローカル検証済み。** 単一Compose、固定したKubernetes API／kindとmulti-docを読み、image・network・named volume・起動条件・desired replicas・Service selector候補・未知endpointを分離する。別のstrict mapping YAMLでcomponentと完全一致targetだけを結び、source→imageのbuild来歴やruntime通信・実配置は補わない。環境変数・Secret／ConfigMapの値、labels値、raw YAMLを出力せず、対象ツール・生成script・dotenv読取・変数展開・manifest mergeは行わない。

Phase 4Bの確認: `go test ./...`（25 package成功）・vet・build・actionlint、Compose／Kubernetesの実CLI・local SVG／PNG・画像確認、空PATHでの図／説明一致、明示対応とsource hash／document行、selectorのnamespace分離を確認した。selectorなしService・未評価継承・unknown imageは図と終了コード3、構文／mappingエラー2・boundary IO1は旧図／画像を保持する。空renderer・1秒期限は5で元PlantUMLを保持。内部レビューのcredential風digest参照・継承からのdefault network誤推定・root YAML mergeのschema回避・boundary IO分類を失敗前／成功後の回帰で修正した。既存class／packageの両言語cache／説明一致、sequence／activity／state／component／deploymentの実CI shellも成功。PR #78をbaseとする別PRでレビューし、公開後のCIを確認する。

次の判断はmain統合PR #77・component PR #78・配置図後続PRのレビューと統合。watch・GUI・実行trace・完全なdispatch／alias／例外解決、配布リリースと性能保証は今回含めない。ユースケース図はactor／goal等の要求モデルの合意後に別途判断する。

## 進め方

- `--explain` とレイアウト設定は別 PR にする
- 各 PR では Java と Go の代表的な fixture、CLI ヘルプ、README、PlantUML 出力の回帰テストを更新し、macOS・Linux・Windows の CI を通す
- 新機能ごとにロードマップ #16 のチェック欄を更新する
