# 次にやること

確認日: 2026-11-01

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

- **レンダラーの残存不具合 — [PR #62](https://github.com/Mino829/umlgen/pull/62)** — タイムアウト後の出力パイプ待機と0バイト画像の成功扱いを修正。main向けに独立して提出済み。
- **`--explain`（Issue #12）— [PR #63](https://github.com/Mino829/umlgen/pull/63)** — 継承・実装・フィールド・引数・戻り値の関係について、解決先、メンバー、ファイル、宣言行をテキスト表示。複数根拠とJava／Goの宣言位置を保持し、キャッシュスキーマを更新。通常のPlantUML出力は維持。main向けに独立して提出済み。
- **確認済み** — `go test ./...`、`go vet ./...`成功。Java／Goの実CLIで宣言位置、コールド／ウォームキャッシュの説明一致、図の不変性、関係種別・focusによる絞り込み、差分図・削除ファイル・プレースホルダを確認。`class --help`／`-h`が引数エラーになる既存不具合も修正し、`--explain`のヘルプ表示を確認。
- **パッケージグルーピングとレイアウト（Issue #13）— [PR #64](https://github.com/Mino829/umlgen/pull/64)** — flat／hierarchical／none、縦／横方向、CLIと設定ファイル、安全な3種類のskinparamを実装。複数パッケージのgoldenを追加。Java／Goの実CLIとSVG・PNG表示、既定出力のバイト一致、Goのドメインだけのmodule root、focus／差分の色・タイトル・プレースホルダを確認。PR #63をベースに別PRとして提出済み。
- **リモートCI** — PR #62／#63のLinux、Windows、macOS amd64／arm64およびCodeQL等のチェック成功を確認。
- **Windowsのgolden比較** — 初回CIでチェックアウト時のCRLF変換による不一致を検出。`.gitattributes`で対象fixtureをLFに固定し、`core.autocrlf=true`でも4つのfixtureのバイト列が変わらないことを確認。比較条件は緩めない。
- マージ・リリース前のため、GitHubのIssue／ロードマップは未完了のまま。

## 次の作業

PR #62、PR #63、PR #64の順にレビュー・マージし、マージ後にIssue #12／#13とロードマップ #16の完了状態を更新する。CIを確認してからリリースする。

## 進め方

- `--explain` とレイアウト設定は別 PR にする
- 各 PR では Java と Go の代表的な fixture、CLI ヘルプ、README、PlantUML 出力の回帰テストを更新し、macOS・Linux・Windows の CI を通す
- 新機能ごとにロードマップ #16 のチェック欄を更新する
