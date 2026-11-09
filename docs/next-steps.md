# 次にやること

更新日: 2026-11-10

## Issueの解消状況

#70・#47・#48はv0.6.0に含まれる。#14のwatchを含む準備変更はPR #81、最終の不足分は[PR #86](https://github.com/Mino829/umlgen/pull/86)でmainへ統合済み。#15（Homebrew／Scoopによる導入改善）と#82（共通IR整理）は今後の開発項目であり、v0.6.0の公開条件には含めない。以下の個別確認は開発当時の記録で、新たな再検証の結果ではない。

#70は[PR #83](https://github.com/Mino829/umlgen/pull/83)でmainへマージされ、v0.6.0に収録。Goの匿名struct／interfaceを含むフィールド・引数・戻り値を表示層で1行の安全な文字列に変換し、解析モデル・キャッシュは変更しない。実PlantUMLによるSVG回帰はflat／hierarchical／noneの全3表示で失敗前・成功後を確認し、Linux CIでも実行する。レンダラー未検出と実行失敗の診断を分離し、明示されたJARへのフォールバックは維持する。

ローカル確認: `go test ./...`、`go vet ./...`、CLI build、actionlint成功。実CLIでGoのSVG／PNGを生成し、画像上の後続フィールド・メソッドを確認した。Go／Javaのcold／warm／`--no-cache`でPlantUMLがバイト一致。実PlantUMLの構文エラーと空PATHの未検出は、ともに終了コード5・元PlantUML保持のまま異なる診断を表示する。OS別CIの結果はPRで確認する。

PR #83の初回Linux CIでは、`--no-install-recommends`によるGraphviz未導入でPlantUMLがエラー画像を返し、実SVG内のメンバー検証が失敗した。CIの依存へ`graphviz`を明示追加し、画像内容のassertは維持する。

### #47：中優先度レビュー項目（M1〜M7）

[PR #84](https://github.com/Mino829/umlgen/pull/84)で実装・4環境CI成功、mainへマージ済み。v0.6.0に収録。

- M1：既存の`--config`手動スキップを標準flag解析へ置き換え、コマンド名と同じ設定ファイル名、値欠落、未知コマンドを回帰で固定した。
- M2：Goのaliasを専用の構文メタデータとして保持し、class／packageの図への投影から除外する。名前付き定義型は維持し、別ファイルのalias receiverも偽classにしない。宣言ファイルが未取得のreceiver部分解析とその推定実装は維持する。
- M3：Javaの型・メソッド・constructor・入れ子スコープ、Goの型・receiverの型パラメーター名を保持する。同名の実型への誤った関係線と未解決診断を除き、具体的な型引数・完全修飾名・static境界を維持する。構文cacheはGo `go-ast-v6`、Java `java-tree-sitter-v4`へ更新。
- M4：Javaデフォルトパッケージを空packageブロックで囲まず描画する。単独・名前付きpackageとの混在を実SVG回帰で検証し、対応するgoldenだけを更新した。
- M5：明示file／directory探索の`filepath.Abs`失敗を返す。macOSでは削除済みcwdも解決できたため、相対file読取は可能だが絶対パス取得が失敗する深いcwdを隔離processで作り、両経路を検証した。
- M6：既存の言語別cache directoryをchmodせず、新規directoryには`0700`を指定する。umaskによる制限を広げず、既存の`0750`／`0500`を回帰で維持する。
- M7：rootのdispatchからclass処理を分離し、`class_options.go`で引数・設定、`class_sources.go`で共有解析cacheを使った現在／Git baseのソース収集を処理する。`class.go`は範囲抽出と既存の生成／出力／rendererを接続する。

ローカル確認：`go test ./...`、`go vet ./...`、CLI build、actionlint成功。実CLIでGo／Javaの型パラメーター由来の偽辺が消え、具体型の辺が残ること、classのcold／warm／`--no-cache`で図・説明が一致することを確認した。packageの全3依存modeも同じ3cache条件で図・説明が一致。デフォルトパッケージの画像は修正前の終了コード5からSVG／PNG成功へ変わり、画像内容を確認。既存cache directoryの`0750`保持、`--config class class src`、公開Java demoのbefore／after／Git差分SVGも実行した。OS別CIの結果はPRで確認する。

### #48：低優先度レビュー項目（L1〜L7）

[PR #85](https://github.com/Mino829/umlgen/pull/85)で実装し、当時の最新headの登録check全13件が成功した。#84のmain統合後に旧`fix/medium-priority-review`へマージされたため、#85の`merged`表示だけではmainへの取り込みを保証しなかった。不足分はPR #86でmainへ統合し、v0.6.0に収録した。

- L1：classと差分placeholderのtitleも他図と同じ安全な文字エスケープに統一した。既存の生title文字列のassertは削除し、実SVGで記号・HTML風文字列・`%date()`がリテラルに残ることを検証する。titleを含む3つのgoldenだけを意図した表記へ更新した。
- L2：`gitdiff.Analyze`へworking directoryを明示し、独立repoの並列解析を回帰で固定した。CLIテストの手動cwd変更・復帰と旧helperを除去し、標準`t.Chdir`の非並列guardへ移行した。
- L3：Issueの「Go 1.25未リリース」という前提は[公式リリースノート](https://go.dev/doc/go1.25)と一致しない。`go.mod`のGo 1.25要件は維持し、実Go 1.25.0で全体テスト・buildと生成CLIのGo SVGを確認した。Go 1.23／1.24対応の追加は行わない。
- L4：登録済み一文字flagの結合と値の直結を扱い、単一dash長名・未知option・競合・`--`・flag風の値を区別する。root／各図／diff／watchの共有解析へ接続した。
- L5：除外パターンのdotヒューリスティックを除去した。`User.java`・`.generated`等はパス成分として扱い、類似名を巻き込まない。通常探索と両Git snapshotの回帰、明示file優先を維持する。
- L6：既定30秒、明示0は無期限、負値はエラーにした。0でもcaller cancellationは維持する。仮想時刻の31秒renderと30秒deadlineの比較、実CLIのYAML 0s／CLI期限上書き／watch Ctrl+Cで検証した。
- L7：Server画像32MiB・エラー本文8KiBを上限にし、既知長・長さ不明・chunkedに適用する。画像はstage後に公開し、上限超過等では前回画像と元PlantUMLを保持する。新規画像でumaskの制限を広げず、既存modeを維持する。

ローカル確認：標準GoとGo 1.25.0で`go test ./...`・build成功、標準Goのvet・Git／CLI race検査・actionlintも成功。実PlantUMLのSVG／PNGでtitleの記号がそのまま表示され、Go／Javaのファイル名除外、結合flag、class／packageのcache一致、独立repoの並列Git差分SVGを確認した。実CLIのローカルHTTP rendererで32MiB成功・1byte超過／stream超過／大きなエラー応答の失敗、前回画像・生成済みPlantUML保持、既存0640／新規umask077の権限、負の期限の送信前拒否を確認。無期限watchの描画中SIGINTは速やかに終了コード0となり、未公開stageを除去した。OS別CIは後続PRで確認する。

PR #85の初回Windows CIでは、並列Git回帰のGit形式`C:/...`とGo形式`C:\...`の文字列比較が失敗した。区切り表記のassertは削除し、`os.SameFile`による実directory同一性と、削除fileのrepo内相対path・内容の検証へ変更した。Windows skipやrepo混同検出の緩和は行わない。

## v0.6.0公開安定版

公開安定版は[v0.6.0](https://github.com/Mino829/umlgen/releases/tag/v0.6.0)。公開ソースcommitは`1fb64f6`（`1fb64f62990e299182c3d6de28b40efe37b9e82b`）。通常installerは最新公開版を取得し、PR差分workflowの既定CLI版も`v0.6.0`とする。

### 収録した変更（v0.5.0から）

- 関係の根拠・ソース抜粋・解析限界の`--explain`、package grouping・レイアウト・安全なskinparam、rendererの期限／空画像エラー修正。
- Java／Goのpackage図、循環・依存根拠・完全なGit snapshotによるpackage差分。
- 静的sequence、単一関数activity、明示状態遷移とソース代入候補を区別するstate。
- 明示component境界と契約、Compose／限定したKubernetes宣言のdeployment。通信実績・実配置・build来歴は推定せず、秘密値を図や説明に出さない。
- class／packageの安全なwatch。debounce、追加／削除、関連設定の再読込、失敗時の旧成果物保持、Ctrl+Cの中断。
- PR本文の許可された範囲指定、4環境の候補アーカイブ生成・インストール・実CLI受入検証、公開前artifactからのオフライン導入。
- #70の実PlantUML描画修正、#47の型パラメーター・alias・デフォルトpackage・cache権限修正、#48のtitle安全化・Git解析のcwd分離・短縮flag・dotを含む除外・renderer期限と応答上限。

PR #78→#79→#80の不足分と配布準備はPR #81、#85の不足分はmainをbaseとした[PR #86](https://github.com/Mino829/umlgen/pull/86)で統合済み。mainの[CI 42638978737](https://github.com/Mino829/umlgen/actions/runs/42638978737)と[CodeQL 42638978616](https://github.com/Mino829/umlgen/actions/runs/42638978616)は成功した。以下の受入条件・個別検証は公開準備当時の履歴として残し、公開版の現状と区別する。

公開は[Release workflow 42641183981](https://github.com/Mino829/umlgen/actions/runs/42641183981)で完了した。初回はmacOS arm64のhosted runner未確保で停止したが、タグ・検証条件を変えず失敗分を再実行し、4環境のnative build／導入smoke・checksum検証後に公開した。公開Releaseから4アーカイブと`SHA256SUMS.txt`を取得し、全checksum・Release APIのdigest／size・実行形式／architecture・Go 1.26.10／CGO・公開commitとの一致を確認。通常のmacOS installerの`latest`でv0.6.0を取得し、全図種・cache・Git差分・privacy・watch・不正候補拒否を再検証、公開Java demoのbefore／after／diff SVGも生成した。

### 公開前の受入条件（履歴）

1. リリース準備PRの通常CI（Linux／macOS arm64・amd64／Windows、JDK 17、全体テスト・vet・脆弱性検査・実CLI）と候補配布CIを通す。
2. `.github/workflows/release-build.yml`で`0.6.0-rc.1`の**ネイティブCGOバイナリー**を4種類生成する。Linuxの配布ビルドはUbuntu 22.04（glibc 2.35）を基準とする。
3. `release-assets`に4アーカイブと正確な`SHA256SUMS.txt`を集約する。実際に展開／候補インストールしたバイナリーで版・各図種・cache一致・Git差分の作業ツリー分離・配置図の秘密値非表示・watchの更新／削除／ネイティブ中断を確認する。macOS／Windowsでは未公開アーカイブのオフラインinstallerを使う。
4. 実PlantUMLで代表的なSVG／PNGを生成・確認し、候補artifactをダウンロードしてチェックサムと内容を確認する。各図の解析限界とJDK／renderer要件はREADME・利用例を維持する。
5. レビュー承認後にmainへマージし、別途公開を承認したうえで`v0.6.0`タグを作る。タグ専用workflowが同じ配布ビルド・受入検証を再実行し、成功後だけGitHub Releaseを作成する。RCタグを明示公開する場合はprerelease扱いでlatestにしない。**本準備作業ではタグもReleaseも作成しない。**

配布リハーサルはPR権限のread-only buildであり、タグもGitHub Releaseも不要。`scripts/package-release.py`はローカルのネイティブ候補を生成し、`scripts/smoke-release.py`はインストールしたCLIを検証する。macOS／Windowsの`--archive-dir`／`-ArchiveDirectory`は明示版とチェックサムを要求し、破損・重複checksum・不正member・版違いでは既存の実行ファイルを置き換えない。

統合候補のローカル確認（準備当時）：全package test・vet、Git／CLI／rendererのrace検査、配布workflowのactionlintに成功した。native `0.6.0-rc.1`アーカイブを作成し、オフラインinstallerで導入した実行fileで全図種・cache一致・汚れた作業ツリーを含めないGit差分・配置図の秘密値非表示・watch更新／削除／中断を確認。installerの破損・版違い・重複checksum・不正archive拒否と既存実行file保持も確認した。同じ導入済みCLIでGo構造型のSVGを3表示方式すべてで生成し、titleの制御構文風文字列と後続メンバーを検証、PNGも目視した。4環境のCI・downloadした候補artifact・main統合後の結果は統合PRに記録する。

準備段階ではmain統合と公開前受入検証までを対象とし、タグ作成・GitHub Release公開は別途承認とした。現在の公開状態は本節冒頭を参照。#15（Homebrew／Scoop）と#82（共通IR）は今後の開発項目として残す。

### v0.6.0の非対象

GUI／エディタ連携、他図種のwatch、実行trace、完全なdispatch／alias／例外解決、ユースケース要求モデル、追加OS／arch配布、新しい性能保証は追加しない。機能ごとの既知の解析限界を「完全解析」として売り出さない。

## v0.5.0公開時の完了記録（履歴）

1. **PR #40 を main にマージ** — 型なし差分のプレースホルダ生成、終了コードの整理、ワークフロー側の旧リリース対応を含む。
2. **v0.5.0 をリリース** — Linux／macOS arm64・amd64／Windows の配布アーカイブ、チェックサム、GitHub Release を公開済み。
3. **再利用ワークフローの既定 CLI 版を更新** — `.github/workflows/umlgen-pr-diff.yml` の `umlgen-version` 既定値を `v0.5.0` に変更。`v0.4.0` 向けのフォールバック処理は、明示的に旧版を指定した場合の互換性のため残している。
4. **Issue / ロードマップを整理** —
   - #11 PNG出力・レンダラー設定 → 完了として close
   - #16 ロードマップ → v0.4.0 / v0.5.0 完了、v0.6.0 / v0.7.0 に再編
   - #20 / #27 → 現行の OSS 機能・互換性・品質優先の計画と合わないため `not planned` で close
5. **ドキュメントを更新** — `review-findings.md`, `github-actions.md`, `pr-diff.md`, `macos-setup.md`, `windows-setup.md`, 開発版バージョン表記を `v0.5.0` に合わせて更新。

## 初期開発・PR記録（当時は未リリース、現在はv0.6.0に収録）

- **レンダラーの残存不具合 — [PR #62](https://github.com/Mino829/umlgen/pull/62)** — mainへマージ済み。タイムアウト後の出力パイプ待機と0バイト画像の成功扱いを修正。
- **`--explain`（Issue #12）— [PR #63](https://github.com/Mino829/umlgen/pull/63)** — mainへマージ済み、Issue #12は完了。継承・実装・フィールド・引数・戻り値の解決先、メンバー、ファイル、宣言行を表示し、複数根拠とJava／Goの宣言位置を保持。通常のPlantUML出力は維持。
- **確認済み** — `go test ./...`、`go vet ./...`成功。Java／Goの実CLIで宣言位置、コールド／ウォームキャッシュの説明一致、図の不変性、関係種別・focusによる絞り込み、差分図・削除ファイル・プレースホルダを確認。`class --help`／`-h`が引数エラーになる既存不具合も修正し、`--explain`のヘルプ表示を確認。
- **パッケージグルーピングとレイアウト（Issue #13）— PR #64／[PR #65](https://github.com/Mino829/umlgen/pull/65)** — mainへ統合済み。flat／hierarchical／none、縦／横方向、CLI・設定ファイル、安全なskinparam、Windowsのgolden改行対策を含む。
- **リモートCI** — PR #62／#63のLinux、Windows、macOS amd64／arm64およびCodeQL等のチェック成功を確認。
- **Windowsのgolden比較** — 初回CIでチェックアウト時のCRLF変換による不一致を検出。`.gitattributes`で対象fixtureをLFに固定し、`core.autocrlf=true`でも4つのfixtureのバイト列が変わらないことを確認。比較条件は緩めない。
- **main統合候補の確認** — `go test ./...`、`go vet ./...`成功。実CLIでJavaのSVG・GoのPNGを`--explain`と階層・横方向・skinparam併用で生成。コールド／ウォーム説明一致を確認。模擬レンダラーの1秒期限は約1.12秒で終了コード5、空画像も終了コード5となり、説明と`.puml`は保持。
- 上記は開発当時の確認記録。現在はv0.6.0に収録済み。

## 機能拡張の検討・実装記録（履歴）

合意した優先順位は、**学習者のコード理解 → チームの設計レビュー → 開発中の継続利用 → エディタ・画面操作**。詳細と最初の完了条件は[`development-direction.md`](development-direction.md)を参照。

既存の`--explain`を拡張する学習者向けの関係説明は[PR #66](https://github.com/Mino829/umlgen/pull/66)でmainへマージされ、v0.6.0に収録済み。関係の意味、解析時のソース抜粋、Goの暗黙実装を支えるメソッド、範囲・失敗・未解決・既知の非表示参照を扱う。実CLIとテストの当時の確認結果は[`development-direction.md`](development-direction.md)に記録。以下はレビュー支援からwatchへ進めた当時の開発記録であり、未完了の公開条件ではない。

PR #73の提案を受けた**PR本文からレビュー範囲を指定して安全に差分図を生成する導線**は、PR #74→#73を通してmainへ統合済み。checkoutしない検証job、4種類の許可flagと上限、JSON→NUL区切り引数、本文編集トリガー、古いrunの取消、Summaryと`review-scope.json`を接続した。差分図だけに条件を適用し、head全体図は従来の文脈を維持する。当時は配布版CLI v0.5.0で確認した。現在の既定CLIはv0.6.0で、PRソースをbuildしない。

確認済み: parserの正常／異常／非実行回帰、全体テスト・vet・actionlint、実配布版CLIとPlantUMLによる差分図・SVG、full図のバイト不変、変更なし・type-less・全削除・解析失敗。実GitHubの本文編集・取消、artifactの本文hash／head／filter一致も確認し、PR #74と統合後のPR #73は4 OSのCIが成功した。Go対象・全体図focus・watch・activity・GUIはこのレビュー導線に含めていない。

以下はv0.6.0に収録した各機能の個別開発時点の実装・検証記録。図別の制限と、当時の確認・統合状況を残す。「未リリース」「レビュー待ち」等は当時の状態で、現在の公開状態を示さない。

図の拡張計画の **P1-01（Goキャッシュのmodule文脈分離）**、**P1-02（ファイル単位の解析結果・import位置）**、**P1-03（PackageIDと依存根拠の集約）** は開発版で実装済み。

`internal/packagegraph`は、文脈を分離したpackage ID、型宣言／import別の根拠・件数、内部／外部／未解決／曖昧候補を保持する。Javaの所有型・wildcardの曖昧性、Goのファイル別aliasと実package名を扱い、推定実装先と通常のalias参照も混同しない。実ファイルの解析で全3modeのcold／warm／キャッシュ非使用一致、既存class CLIの図・説明とJava golden一致を確認。`go test ./...`、`go vet ./...`成功。

**P1-04（循環検出とpackage範囲抽出）** と **P1-05（package PlantUML生成）** も開発版で実装済み。通常／推定根拠を区別するSCC、関係選択後のin/out/bothとhop、cycles-only・外部非表示、focusで隠れた循環を保持する。図は別の生成APIでpackage端点・種類別件数・循環とscopeの凡例を描き、同名別文脈と未信頼文字列を安全に表示する。実Java／Goソースから17シナリオのSVG／PNGと文書の実出力例をPlantUML 1.2026.6で生成・確認。`go test ./...`、`go vet ./...`、既存class CLIのcold／warm／`--no-cache`とJava golden一致も確認した。

**P1-06（`package`コマンドとYAML接続）** と **P1-07（packageの根拠・解析範囲の説明）** も開発版で実装済み。`umlgen package`から全3mode、循環・外部表示、package focus、関係選択、SVG／PNG、`--explain`を利用できる。共有する探索・構文キャッシュ・出力処理を既存class経路から限定抽出し、図別の既定ファイルと新YAML namespaceの検証を追加した。

2026-11-02の確認: Java／Goの全3modeで実CLIのcold／warm／`--no-cache`説明一致と通常図の不変性、型0件のimport、推定実装・focusで隠れた候補循環、設定優先度・相対パス・exclude置換、部分／全件解析失敗・読取失敗を確認。実local PlantUMLでSVG／PNGを生成し、1秒期限と0バイト画像は終了コード5で`.puml`と説明を保持した。既存classのcache／layout／explainとJava golden一致、実Git差分の追加・変更・削除およびGit base抜粋も確認。`go test ./...`、`go vet ./...`成功。

**P1-08（受入検証の集約と利用例・性能比較）** も開発版で実装・検証済み。`testdata/package/{java,go}`とCLI受入回帰に、依存mode別の端点・辺・件数、曖昧／未解決、型0件のimport、Goのファイル別alias、循環を集約した。[利用例](../examples/package-diagrams/README.md)と[性能比較](performance.md)を追加し、Phase 1Aの完成条件を確認した。P1-08では新しい性能保証やpackage差分を追加せず、次のPhase 1Bへ分離した。

**Phase 1B（P1B-01〜04、完全snapshotに基づくpackage差分）** も開発版で実装・検証済み。`diff --diagram package`が両commitの全選択ソースと各go.modをGitから固定し、未変更ファイルに残る根拠、一部／最後の根拠削除、同件数の入替、file rename、package／module identity変更を区別する。dirty／staged／untrackedを含めず、既存class diffの既定は変更しない。比較後のexternal／cycles-only投影で偽の辺削除を作らず、解析失敗・未解決／曖昧参照に関係する辺／循環を不確かとして説明する。

2026-11-04の個別確認: Java／Goの全3依存modeでcold／warm／`--no-cache`とexplain有無の図一致、実local SVG／PNG、部分／全件parse失敗、模擬rendererの期限・空画像・異常終了、Go module変更と汚れた作業ツリーの分離、削除target・片側空・merge-base、行移動とfile renameを実CLIで確認。Java goldenと既存class／class diffの互換性、`go test ./...`、`go vet ./...`も成功。当時の確認には他OS CI・マージ・リリース・package差分の性能保証を含めていない。契約と利用例は[差分機能](pr-diff.md#パッケージ依存の差分)と[開発計画](uml-expansion-proposal.md)を参照。

**Phase 2（静的シーケンス図）** も開発版で実装・検証済み。Goは選択済みローカルソースの`go/types`、JavaはJDK 17以上の`JavacTask.parse/analyze`を採用し、対象のbuild・実行・annotation processor・依存downloadを行わない境界を固定した。`sequence --entry`、一意なselector、制御領域、直接呼び出し展開、動的候補、go／defer／channel、再帰・上限・非対応箇所の明示、取得済みソースによる説明、専用YAML・rendererを接続した。

2026-11-05の確認: 両言語の実CLIとlocal SVG／PNG、引数の呼び出し順・短絡・分岐・loop、動的候補の非展開、Goの非同期／defer登録・channel相手不明、Javaの例外barrierと後続省略、各上限・再帰・overload、外部依存不足・壊れたentryによる旧図保持を確認した。javacの構文回復が作った仮のentryで旧図を上書きする不具合を修正し、回帰を追加した。JDK不在時の既存Java class、空PATHでのGo解析、Java期限と継承pipe待機、renderer異常・期限・空画像、YAML相対パスと優先順位、文字列のPlantUML無害化も確認。既存Java golden、Java／Goのclass／package cache・説明、全package mode、実Gitのpackage差分と既定class diffは維持。`go test ./...`、`go vet ./...`成功。環境・実行例・限界は[利用例](../examples/sequence/README.md)と[開発計画](uml-expansion-proposal.md)に記録した。

PR公開前の追加修正: Goのパーサー回復宣言・閉じbrace欠落・内部リテラルをentry／候補から除外し、妥当な別関数と型エラーだけの部分解析は維持した。シーケンス投影では全代替経路が非対応barrierの場合の後続漏れ、未知eventの無表示打切り、空本体の深さ上限による偽の省略を修正。package差分ではGit replacement objectによる固定SHAの内容すり替えを無効化し、未対応module入力によるpackage追加・削除も不確かにした。実CLIで再現・結果を確認し、全体テスト・vet・build成功。両言語のSVG／PNGとJava class golden一致を再確認した。4 OSのCIにJDK 17と両言語の実CLI生成smokeを追加し、smoke shell blockはローカルmacOSで実行した。

Java protocolの追加修正: JDK 17／Windowsの非UTF-8既定charsetを想定した実JVM検証で、日本語メソッド名が`??`になる問題を再現。helperの出力writerをUTF-8固定にし、日本語entry選択・呼び出しIDを保持する回帰を追加した。

2026-11-06のCI修正: PR #71のWindows受入失敗は、fixtureの`RUNNER~1`形式と実際のGit根拠・診断の`runneradmin`形式の差だった。PR #72で物理rootへ揃え、厳密な根拠検証を維持した。PR #72はPR #71へマージ済みで、更新後の[PR #71 CI](https://github.com/Mino829/umlgen/actions/runs/42171703393)は4 OSすべて成功。PR #73を反映したmainとの統合にテキスト競合はなく、統合treeでも全体テスト・vet・build、両言語sequenceのSVG／PNG、全package modeのcache／説明一致、immutable Git差分、Java class goldenの一致を確認した。Windows skipやassertの緩和は行っていない。

共有する作業規約を[`CONTRIBUTING.md`](../CONTRIBUTING.md)へ追加した。実装依頼は検証・コミット・push・PR作成とURL報告までを完了条件にし、手元変更の保持、依存PRの明記、修正対象CI失敗の解消、無断マージ・リリースを行わないことを定めた。既存のignore方針を維持し、ローカル専用AGENTS.mdからこの規約を参照する。PR #69のマージを確認し、PR #71のbaseはmainへ更新済み。

**Phase 3A（単一関数アクティビティ図）は[PR #75](https://github.com/Mino829/umlgen/pull/75)で実装・検証済み。全4環境CI成功、mainへマージ済み・未リリース。** `activity --function`がGo／Javaの同じ取得済みAST・宣言から選択関数のCFGを作り、位置付きノードと型付き辺を描く。if／switch／loop・式内短絡・早期return、break／continue・label、Goのgoto／組込みpanic、Javaのthrow／三項を扱う。try/finally・defer/recover等はterminal barrierで、未知の継続を正常終了にしない。既定500ノードのhard cap、専用YAML、説明、既存rendererを接続した。

Phase 3Aの確認: 全体テスト（22 package成功）・vet・build、両言語の実CLIとlocal SVG／PNG、短絡RHSの接続先・label付きtransfer・returnの終了先・非対応ノードの終端、UTF-8 selector、上限超過による旧図保持。Goの定数unsafe演算・generic channel range、Javaのclass literal／constructor referenceと継承pipe期限を修正し、回帰を保持。既存sequenceの実SVG、全package mode、Java class goldenの一致を維持。CI smoke shellをローカル実行し、[公開後のCI](https://github.com/Mino829/umlgen/actions/runs/42179247787)も4環境すべて成功した。

**Phase 3B（明示状態図とソース代入候補）は[PR #76](https://github.com/Mino829/umlgen/pull/76)で実装・4環境CI成功後、featureブランチへマージ済み。当時はmain統合の[PR #77](https://github.com/Mino829/umlgen/pull/77)がレビュー待ち・未リリースだった。現在はv0.6.0に収録済み。** `state --state-config`にversion: 1の型・field・状態・初期状態・event／guard／action・遷移を指定し、configuredな実線と直前状態不明の破線候補を分離する。optionalなsource valueはcompiler定数／enum・観測literalへ意味上のkeyで束縛し、未知・曖昧・矛盾を拒否する。別型・shadowing・別rootを名前だけで混同せず、書込みの所有関数・lexical guard・位置・CFG coverageと未対応範囲を説明する。初期状態・遷移元・runtime instance・実行済み経路は推定しない。利用方法は[README](../README.md#明示状態遷移図)を参照。

Phase 3Bの確認: 全体テスト（23 package成功）・vet・build、両言語の実CLI・local SVG／PNG、明示遷移と候補の分離、未知RHS、field／capエラーによる旧図保持。レビューで見つかったGoの別定義型field混同・未評価initializerの誤ったCFG対応・同表記定数式のID衝突・range target内literalの所属漏れ、Javaの括弧付きreceiverを失敗前／成功後の回帰で修正。Go composite initializerは候補を残し、専用ノードがないことを診断する。既存activity／sequenceの実SVG、全package mode、Java class goldenの一致、追加CI shell blockのローカル実行とactionlint成功を確認。#76のマージ先は既に#75がマージされた後のfeatureブランチだったため、mainにはまだ状態図が含まれない。#77で状態図だけのmain統合差分を分離した。

**Phase 4A（明示コンポーネント境界）は[PR #78](https://github.com/Mino829/umlgen/pull/78)で実装・4環境CI成功。当時はレビュー待ち・未マージ・未リリースだったが、現在はv0.6.0に収録済み。** 厳密なversion: 1 YAMLからpackage prefix・exact Go module・exact物理source rootをAND／ORで照合し、重複所属をhard errorにする。未割当・空境界・外部を混同せず、内部／境界間／非表示依存に元package・型・import・推定根拠・位置・hashを保持する。provides／requiresは明示契約だけで同名providerへ自動接続しない。`component`・専用YAML・構文cache・renderer・説明を接続し、JavaもJDK不要。利用例と契約は[README](../README.md#明示コンポーネント図)を参照。

Phase 4Aの確認: 全体テスト（24 package成功）・vet・build・actionlint、両言語の全3依存modeでcold／warm／`--no-cache`の図・説明一致、実local Go SVG／Java PNG、空PATHで両言語の生成成功。境界競合時の終了コード2と旧SVG／PlantUML保持、部分失敗ファイルの除外と終了コード3、全件失敗時の旧図保持を実CLIで確認した。レビューで見つかったsymlink親参照とcase-insensitive filesystemのroot混同を失敗前／成功後で修正し、将来rootとWindowsの区切りを含む回帰を保持。既存class／packageの両言語cache／説明一致、sequence／activity／state／componentの実CI shell成功を確認した。main統合PR #77のheadをbaseとするレビュー用後続PRとして提出し、公開後のCIを確認する。

**Phase 4B（宣言された配置図）は[PR #79](https://github.com/Mino829/umlgen/pull/79)で実装・4環境CI成功。当時はレビュー待ち・未マージ・未リリースだったが、現在はv0.6.0に収録済み。** 単一Compose、固定したKubernetes API／kindとmulti-docを読み、image・network・named volume・起動条件・desired replicas・Service selector候補・未知endpointを分離する。別のstrict mapping YAMLでcomponentと完全一致targetだけを結び、source→imageのbuild来歴やruntime通信・実配置は補わない。環境変数・Secret／ConfigMapの値、labels値、raw YAMLを出力せず、対象ツール・生成script・dotenv読取・変数展開・manifest mergeは行わない。

Phase 4Bの確認: `go test ./...`（25 package成功）・vet・build・actionlint、Compose／Kubernetesの実CLI・local SVG／PNG・画像確認、空PATHでの図／説明一致、明示対応とsource hash／document行、selectorのnamespace分離を確認した。selectorなしService・未評価継承・unknown imageは図と終了コード3、構文／mappingエラー2・boundary IO1は旧図／画像を保持する。空renderer・1秒期限は5で元PlantUMLを保持。内部レビューのcredential風digest参照・継承からのdefault network誤推定・root YAML mergeのschema回避・boundary IO分類を失敗前／成功後の回帰で修正した。既存class／packageの両言語cache／説明一致、sequence／activity／state／component／deploymentの実CI shellも成功。PR #78をbaseとする別PRでレビューし、公開後のCIを確認する。

**Issue #14（安全なwatch）は本後続ブランチで実装・ローカル検証済み。** class／packageとJava／Go、内容snapshot・連続保存debounce・serial生成・関連config/module変更・追加削除renameを接続した。stageで完全生成し、部分parse・設定・画像失敗では前回成功したPlantUML／画像を保持する。最後のsource削除は明示的な空図。Ctrl+Cで描画をキャンセルし、未公開stageを破棄する。時間設定はCLIのみで、既存cache／図別flag・設定優先度を維持する。

watchの確認: 全体テスト（27 package）・vet・build・actionlint成功。実CLIでGoの4連続保存を1更新へ集約し、同mtimeのatomic保存、追加rename削除、設定エラー修復、source/output変更、go.mod変更、最後の削除、generated fileの非循環を確認。Java packageの実PNGとcache、部分parse／renderer失敗で両成果物を保持、描画中のSIGINTが0で速やかに終了することを確認した。空PATHのJava構文watchも成功。内部レビューのauto空図・parent alias経由の出力衝突・umask拡大・観測失敗後の世代取りこぼしを修正し、初回描画中のconfig復帰も実CLIで失敗前／成功後を確認。既存class／packageの両言語cache／説明一致、全図CI shellと新watch scriptのmacOS実行も成功。PR #79をbaseとするレビュー用PRで公開後の4環境CIを確認する。

PR #80の初回Windows CIでは、通常file配下のchildに対するnot-existとUnixのENOTDIRの差を検出した。未作成suffixを補う前に既存prefixがdirectoryであることを確認し、既存の拒否回帰を維持した。Windows skipやassertの緩和は行わず、修正後の4環境CIを確認する。

次の判断はmain統合PR #77・component PR #78・配置図PR #79・watch後続PRのレビューと統合。GUI・他図種watch・実行trace・完全なdispatch／alias／例外解決、配布リリースと性能保証は今回含めない。ユースケース図はactor／goal等の要求モデルの合意後に別途判断する。

## 進め方

- `--explain` とレイアウト設定は別 PR にする
- 各 PR では Java と Go の代表的な fixture、CLI ヘルプ、README、PlantUML 出力の回帰テストを更新し、macOS・Linux・Windows の CI を通す
- 新機能ごとにロードマップ #16 のチェック欄を更新する
