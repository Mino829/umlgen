# Macではじめるumlgen

この手順は、ターミナル操作に慣れていない方向けです。Apple Silicon（M1以降）とIntel Macの両方に対応します。導入済みのHomebrew、または管理者権限なしのインストーラーを利用できます。

## 1. ターミナルを開く

`command`キーとスペースキーを同時に押し、`ターミナル`と入力して「ターミナル」を開きます。

[Homebrew](https://brew.sh/)を導入済みの場合は、次のコマンドを1行ずつ貼り付けてReturnキーを押します。

```bash
brew tap mino829/umlgen https://github.com/Mino829/umlgen.git
brew install mino829/umlgen/umlgen
umlgen version
```

このリポジトリー自身をtapとして使うため、`brew tap`のURLは省略しません。Macの種類に合った公開バイナリーを固定SHA-256で検証して配置します。Go・JDK・PlantUMLの自動追加はありません。既存のスクリプト導入から移行する場合は、先に[旧インストーラーで導入した版の削除](#スクリプトで導入した場合の削除)を行い、`command -v umlgen`でHomebrew側のコマンドを確認してください。

### Homebrewを利用しない場合

```bash
installer="$TMPDIR/install-umlgen.sh"
curl -fL https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-macos.sh -o "$installer"
bash "$installer"
```

このインストーラーもMacの種類を自動判定し、GitHub Releaseの`SHA256SUMS.txt`と照合してから`~/.local/bin`へ配置します。更新・削除は導入方法を混在させず、下の対応する手順を使ってください。

この通常導入の対象は公開安定版（現在[v0.6.0](https://github.com/Mino829/umlgen/releases/tag/v0.6.0)）です。Java `sequence`／`activity`／`state`にはフルJDK 17以上が別途必要です。`java -version`と`javac -version`を確認してください。Javaのclass／package／componentおよびclass／packageのwatchはJDK不要です。画像生成のPlantUML要件は別です。

## 2. インストールを確認する

ターミナルを閉じて、新しく開き直します。次を実行します。

```bash
umlgen version
```

次のようにバージョンが表示されれば完了です。

```text
umlgen version 0.6.0
```

## 3. Javaプロジェクトへ移動する

例として、プロジェクトが書類フォルダーの`my-app`にある場合：

```bash
cd ~/Documents/my-app
```

Finderからプロジェクトフォルダーをターミナルへドラッグすると、パスを手入力せずに貼り付けられます。

## 4. クラス図を生成する

一般的なMaven／Gradleプロジェクト：

```bash
umlgen class ./src/main/java
```

プロジェクトのフォルダーに`class-diagram.puml`が作成されます。

複数モジュールをまとめて解析する場合：

```bash
umlgen class . --exclude target --exclude build
```

## SVG／PNG画像も生成する

SVG・PNG生成にはPlantUMLレンダラーが必要です。すでにHomebrewを利用している場合：

```bash
brew install plantuml
umlgen class ./src/main/java --format svg
umlgen class ./src/main/java --format png
```

Homebrewを利用していない場合は、まず`.puml`生成まで利用できます。

## Pull Requestで自動生成する

GitHub Actionsを使うと、MacへPlantUMLを追加しなくてもGitHub上で図を自動生成できます。

[GitHub Actions導入ガイド](github-actions.md)のworkflowをリポジトリへ追加してください。

## 更新

Homebrewで導入した場合：

```bash
brew update
brew upgrade mino829/umlgen/umlgen
umlgen version
```

新Release公開後に配布定義の更新PRがマージされると利用できます。更新PRのマージ前には従来版のままです。

スクリプトで導入した場合は、最初の3行のインストーラー取得・実行をもう一度行います。PATH設定は重複しません。特定バージョンを使う場合は`bash "$installer" --version v0.6.0`と指定します。

## 公開前の候補版を試す

v0.6.0は公開済みです。以下は今後の候補をオフラインで試す手順です。リリース準備PRの成功したCIからartifact `release-assets`をダウンロードし、アーカイブと`SHA256SUMS.txt`を同じフォルダーへ展開します。**artifactを作ったcommitのinstaller**を使い、版を明示してください。下の`0.6.0-rc.1`は過去の配布リハーサル例なので、試すartifactの実際の版へ置き換えます。通常の`latest`は未公開候補を取得しません。

```bash
# 候補checkoutのrepository rootで実行。既存の安定版とは別の場所へ配置
bash scripts/install-macos.sh --archive-dir "$HOME/Downloads/release-assets" \
  --version 0.6.0-rc.1 --install-dir "$HOME/.local/umlgen-rc" --skip-path-update
"$HOME/.local/umlgen-rc/umlgen" version
```

このmodeはネットワークを使わず、native archiveのchecksum・member・実行ファイルの版を検証してから配置します。出所を信頼できるartifactだけを使ってください。checksumだけで発行者を認証できるわけではありません。PATHは変更しません。終了時は同じcheckoutのuninstallerで`--install-dir "$HOME/.local/umlgen-rc" --skip-path-update`を指定できます。

## アンインストール

Homebrewで導入した場合：

```bash
brew uninstall mino829/umlgen/umlgen
brew untap mino829/umlgen
```

PlantUMLやJDKは別管理なので削除されません。

### スクリプトで導入した場合の削除

ターミナルで次を1行ずつ実行します。

```bash
uninstaller="$TMPDIR/uninstall-umlgen.sh"
curl -fL https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/uninstall-macos.sh -o "$uninstaller"
bash "$uninstaller"
```

umlgen本体と、インストーラーが`.zprofile`へ追加したPATH設定だけを削除します。`~/.local/bin`内のほかのファイルは削除しません。

## 困ったとき

### `command not found: umlgen`と表示される

ターミナルを一度閉じ、新しく開き直してください。それでも動かない場合：

Homebrewで導入した場合は`brew list mino829/umlgen/umlgen`で配置を確認し、[Homebrew公式のPATH設定](https://docs.brew.sh/FAQ#why-should-i-install-homebrew-in-the-default-location)も確認してください。以下の`~/.local/bin`と`.zprofile`の手順はスクリプト導入の場合だけです。

```bash
~/.local/bin/umlgen version
```

これで動く場合は、次を実行してからもう一度確認します。

```bash
source ~/.zprofile
umlgen version
```

### 「開発元を確認できない」と表示される

システム設定の「プライバシーとセキュリティ」にumlgenを許可するボタンが表示されている場合は、内容を確認してから許可できます。配布ファイルは[GitHub Releases](https://github.com/Mino829/umlgen/releases)で公開しています。

### `no Java source files found`と表示される

指定したフォルダーの下に`.java`ファイルがあるか確認してください。場所が分からない場合は、プロジェクトの一番上で次を試します。

```bash
umlgen class . --exclude target --exclude build
```

## 配布定義の更新（メンテナー向け）

Homebrewの`Formula/umlgen.rb`とScoopの`bucket/umlgen.json`は同じrepositoryで管理します。別のtap／bucket repositoryや外部認証情報は不要です。更新対象は**公開済み安定版の厳密な`X.Y.Z`／`vX.Y.Z`**だけです。`latest`、prerelease、build metadataは受け付けません。Python 3.10以上で実行します。

```bash
python3 scripts/update-package-managers.py --version v0.6.0
# 書き換えずに、公開版から生成した内容と一致するかを検査
python3 scripts/update-package-managers.py --version v0.6.0 --check
```

次回は公開済みの実際のタグに置き換えます。Release APIのtag・draft／prerelease・asset名／URL、`SHA256SUMS.txt`の形式と重複・欠落、実際にダウンロードしたmacOS／Windows archiveのSHA-256をすべて確認してから定義を書き換えます。GitHub APIのdigestがある場合はそれも照合します。checksumは改ざん検出用で、発行者の署名ではありません。API rate limitを避ける場合は任意で`GH_TOKEN`を設定できますが、公開Releaseの取得にsecretは必須ではありません。

Release workflowの成功後にActionsの **Update package managers** を手動実行し、`version`に公開タグを指定します。Releaseは`GITHUB_TOKEN`で作るため、`release.published`イベントによる別workflowの起動には依存しません。

- 既定の`create_pr=false`は読み取り権限だけで動き、更新定義とpatchをartifact `package-manager-update`へ保存します。job summaryのコマンドでpatchを取得し、人がbranch・commit・PRを作れます。**現在のrepositoryはActionsによるPR作成を許可していないため、この経路を使用します。**
- repository設定でActionsのPR作成を許可した場合だけ`create_pr=true`を選びます。専用jobが`contents: write`／`pull-requests: write`で同repositoryにPRを提案します。push／PR作成が禁止された場合は明確なwarningとartifactによる手動経路を残します。PATやGitHub App secretを暗黙に要求しません。
- tokenによるpush／PRではCIが自動開始しない、または実行承認が必要な場合があります。人が保留中workflowを承認するか、`gh workflow run ci.yml --ref <更新branch>`でCIを起動し、結果を確認してからレビュー・マージします。自動マージはしません。配布先はdefault branchなので、PRがマージされるまでは利用者へ反映されません。

CIは不正な版・checksum・Release metadataと、検証失敗時に既存定義が保たれることをofflineでテストします。native smokeは実際の公開v0.6.0をHomebrew（arm64／amd64）とScoop（Windows amd64）へ導入し、作業中定義にtap／bucketを更新して`version`・Javaクラス図・削除を確認します。初回の同一版ではupgradeのno-opに加えて再インストールを試し、次の公開版以降は本当の旧版からのupgradeを試します。ソースビルドや架空の版への書き換えはしません。

```bash
python3 -m unittest discover -s scripts -p test_package_managers.py
# 既存のumlgen tap／導入がない検証用Macでのみ実行
bash scripts/smoke-homebrew.sh
```

Windowsの検証用環境ではScoopを導入してから`./scripts/smoke-scoop.ps1`を実行します。両smokeともこのcheckoutの定義を一時的なGit tap／bucketとして使うため、公開mainに未マージのPRも検証できます。実際の公開assetへのネットワーク接続が必要です。
