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

次は **P1-08（受入検証の集約と利用例・性能比較）**。Java／Goの代表fixture、既存class/diffを含む回帰、既存性能基準との比較を揃えてPhase 1Aの完成条件を確認する。今回、新しい性能保証やpackage差分は追加していない。詳細は[`uml-expansion-proposal.md`](uml-expansion-proposal.md)を参照。

## 進め方

- `--explain` とレイアウト設定は別 PR にする
- 各 PR では Java と Go の代表的な fixture、CLI ヘルプ、README、PlantUML 出力の回帰テストを更新し、macOS・Linux・Windows の CI を通す
- 新機能ごとにロードマップ #16 のチェック欄を更新する
