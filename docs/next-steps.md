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

次の実装は、既存の`--explain`を拡張する学習者向けの関係説明。関係の意味、宣言のソース抜粋、Goの暗黙実装を支えるメソッド、不完全な解析の状況を扱う。watch／導入改善は、この説明機能とレビュー支援の後に進める。リリースの実施は別途判断する。

## 進め方

- `--explain` とレイアウト設定は別 PR にする
- 各 PR では Java と Go の代表的な fixture、CLI ヘルプ、README、PlantUML 出力の回帰テストを更新し、macOS・Linux・Windows の CI を通す
- 新機能ごとにロードマップ #16 のチェック欄を更新する
