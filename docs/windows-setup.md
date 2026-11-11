# Windowsではじめるumlgen

この手順は、コマンド操作に慣れていない方向けです。Windows 10／11の64-bit PCを対象にしています。管理者としてPowerShellを起動する必要はありません。

## 1. PowerShellを開く

スタートメニューを開き、`PowerShell`と入力して「Windows PowerShell」を起動します。

[Scoop](https://scoop.sh/)とGitを導入済みの場合、次を1行ずつ貼り付けてEnterを押します。Gitがない場合は先に`scoop install git`で導入してください。

```powershell
scoop bucket add umlgen https://github.com/Mino829/umlgen.git
scoop install umlgen/umlgen
umlgen version
```

このrepository自身の`bucket/umlgen.json`を使い、公開Windows amd64バイナリーを固定SHA-256で確認して配置し、`umlgen`のshimを作ります。別のbucket repositoryや認証情報は不要です。既存のスクリプト導入から移行する場合は、先に[旧インストーラーで導入した版の削除](#スクリプトで導入した場合の削除)を行い、`Get-Command umlgen`でScoop側のコマンドを確認してください。

Scoop版はumlgen本体だけを導入します。Go・JDK・PlantUMLは一緒にインストールしません。Javaのclass／package／componentおよびclass／packageのwatchはJDK不要です。Java `sequence`／`activity`／`state`には、**別途フルJDK 17以上**をPATHに設定し、`java -version`と`javac -version`を確認してください。SVG／PNGにはPlantUMLとその実行環境が別途必要です。

### Scoopを利用しない場合

```powershell
$installer = "$env:TEMP\install-umlgen.ps1"
Invoke-WebRequest https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-windows.ps1 -OutFile $installer
powershell -NoProfile -ExecutionPolicy Bypass -File $installer -InstallPlantUML
```

従来のインストーラーは`%LOCALAPPDATA%\Programs\umlgen`へ`umlgen.exe`・SVG生成用PlantUMLを導入し、ユーザーPATHを設定します。GitHub Releasesから取得し、Release APIのSHA-256を確認してから展開します。同梱PlantUMLによるクラス図のSVG出力にJavaの事前導入は不要ですが、同梱の実行環境はJava意味解析用フルJDKの代わりにはなりません。更新・削除は導入方法を混在させないでください。

## 2. インストールを確認する

PowerShellを閉じて、新しく開き直します。次を実行します。

```powershell
umlgen version
```

次のようにバージョンが表示されれば完了です。

```text
umlgen version 0.6.0
```

## 3. Javaプロジェクトへ移動する

例として、プロジェクトが`C:\work\my-app`にある場合：

```powershell
cd C:\work\my-app
```

エクスプローラーでプロジェクトフォルダーを開き、アドレスバーへ`powershell`と入力してEnterを押す方法でも、その場所でPowerShellを開けます。

## 4. クラス図を生成する

一般的なMaven／Gradleプロジェクト：

```powershell
umlgen class .\src\main\java
```

プロジェクトのフォルダーに`class-diagram.puml`が作成されます。

SVG・PNG画像も生成する場合は、まずローカルPlantUMLを用意します。Scoopの場合の追加例（JavaとPlantUMLは別管理です）：

```powershell
scoop bucket add java
scoop install java/temurin17-jdk
scoop bucket add extras
scoop install extras/plantuml main/graphviz
java -version
javac -version
plantuml -version

# 今のPowerShellと、今後開くターミナルにJARの場所を設定
$env:PLANTUML_JAR = Join-Path (scoop prefix plantuml) 'plantuml.jar'
[Environment]::SetEnvironmentVariable('PLANTUML_JAR', $env:PLANTUML_JAR, 'User')
java -jar "$env:PLANTUML_JAR" -version
```

Scoopの`plantuml`はバッチ形式のshimなので、umlgenからは`PLANTUML_JAR`を使ってJava経由で起動します。`$env:`の設定は現在のPowerShellへ、`User`の設定は今後開くターミナル向けに保存します。`plantuml -version`だけではumlgenから起動できることを確認できないため、JARの版表示も確認してください。

この例はGraphvizも導入します。別のローカルrendererを用意済み、または従来の`-InstallPlantUML`付きインストーラーで導入した場合も、次の画像生成コマンドを利用できます：

```powershell
umlgen class .\src\main\java --format svg
umlgen class .\src\main\java --format png
```

たとえばSVGを選ぶと、次の2ファイルが作成されます。

```text
class-diagram.puml
class-diagram.svg
```

複数モジュールをまとめて解析する場合は、Javaソースを含む上位フォルダーを指定できます。

```powershell
umlgen class . --exclude target --exclude build --format svg
```

## Pull Requestで自動生成する

GitHub Actionsを使う場合、Windows PCへumlgenをインストールしなくてもGitHub上で図を自動生成できます。

[GitHub Actions導入ガイド](github-actions.md)のworkflowをリポジトリへ追加してください。

## 更新

Scoopで導入した場合：

```powershell
scoop update
scoop update umlgen
umlgen version
```

新Release公開後に配布定義の更新PRがマージされると利用できます。更新PRのマージ前には従来版のままです。

スクリプトで導入した場合は、最初のインストーラー取得・実行の3行をもう一度行います。PATHは重複しません。特定バージョンを使う場合は`powershell -NoProfile -ExecutionPolicy Bypass -File $installer -Version v0.6.0 -InstallPlantUML`と指定します。

## 公開前の候補版を試す

通常導入は公開安定版（現在[v0.6.0](https://github.com/Mino829/umlgen/releases/tag/v0.6.0)）を取得します。以下は今後の候補をオフラインで試す手順です。リリース準備PRの成功したCIからartifact `release-assets`をダウンロードし、ZIPと`SHA256SUMS.txt`を同じフォルダーへ展開します。**artifactを作ったcommitのinstaller**を使ってください。下の`0.6.0-rc.1`は過去の配布リハーサル例なので、試すartifactの実際の版へ置き換えます。

```powershell
# 候補checkoutのrepository rootで実行。安定版とは別の場所へ配置
$candidateDir = Join-Path $env:LOCALAPPDATA 'Programs\umlgen-rc'
./scripts/install-windows.ps1 `
  -ArchiveDirectory "$HOME\Downloads\release-assets" `
  -Version 0.6.0-rc.1 -InstallDir $candidateDir -SkipPathUpdate
& "$candidateDir\umlgen.exe" version
```

これは配布リハーサル用の版で、GitHub Release公開を意味しません。ローカルmodeは明示版を要求し、ネットワークを使わず、checksum・member・実行ファイルの版を検証してから配置します。PATHは変更せず、`-InstallPlantUML`は併用できません。画像には既存のローカルrendererを使ってください。出所を信頼できるartifactだけを使用し、checksumを発行者の認証と混同しないでください。削除は同じcheckoutのuninstallerに`-InstallDir $candidateDir -SkipPathUpdate -Confirm:$false`を指定します。

## アンインストール

Scoopで導入した場合：

```powershell
scoop uninstall umlgen
scoop bucket rm umlgen
```

別途導入したPlantUMLやJDKは削除されません。

### スクリプトで導入した場合の削除

PowerShellで次を1行ずつ実行します。

```powershell
$uninstaller = "$env:TEMP\uninstall-umlgen.ps1"
Invoke-WebRequest https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/uninstall-windows.ps1 -OutFile $uninstaller
powershell -NoProfile -ExecutionPolicy Bypass -File $uninstaller
```

umlgenの専用フォルダーと、追加したユーザーPATHだけを削除します。

## 困ったとき

### `umlgen`という名前が認識されない

PowerShellを一度閉じ、新しく開き直してください。それでも動かない場合：

Scoopで導入した場合は`scoop list`と`Get-Command umlgen`で配置を確認し、Scoopのshimsディレクトリー（通常`$HOME\scoop\shims`）がPATHに含まれるか確認してください。以下は従来のスクリプトで導入した場合だけの確認です。

```powershell
& "$env:LOCALAPPDATA\Programs\umlgen\umlgen.exe" version
```

これで動く場合は、Windowsへサインインし直すとPATHが反映されます。

### スクリプトの実行が禁止されている

従来のインストーラーでは手順にある`-ExecutionPolicy Bypass`を含めて実行してください。この指定はインストーラーを実行する1回だけに適用され、PC全体の設定は変更しません。Scoop自身の導入は[Scoop公式手順](https://scoop.sh/)に従ってください。

### Windows Defenderや会社のセキュリティ機能で止められる

セキュリティ機能を無効にしないでください。会社のPCでは管理者へ確認してください。インストーラーを使えない場合は、[GitHub Releases](https://github.com/Mino829/umlgen/releases)から`umlgen-windows-amd64.zip`と`SHA256SUMS.txt`をダウンロードし、社内ルールに従って確認してください。

### `API rate limit exceeded`と表示される

GitHubへのアクセスが一時的に集中しています。しばらく待ってから、同じインストールコマンドをもう一度実行してください。

### `no Java source files found`と表示される

指定したフォルダーの下に`.java`ファイルがあるか確認します。一般的には次の場所です。

```text
src\main\java
```

場所が分からない場合は、プロジェクトの一番上で次を試します。

```powershell
umlgen class . --exclude target --exclude build
```

## 配布定義の更新（メンテナー向け）

公開済みの厳密な安定版と`SHA256SUMS.txt`からHomebrew／Scoop両方を同時更新します。生成script・手動dispatch・PR権限がない場合のartifact経路・CI確認は[配布定義の更新手順](macos-setup.md#配布定義の更新メンテナー向け)を参照してください。Windowsのnative smokeは、既存のumlgen導入やbucketがない検証用PCでScoop導入後に`./scripts/smoke-scoop.ps1`を実行します。

SVG／PNGまで検証する場合は、前述のJDK・PlantUML・Graphvizを導入してから、`./scripts/smoke-scoop.ps1 -RenderOutputDirectory "$env:TEMP\umlgen-scoop-render"`を実行します。ScoopのJARを現在のプロセスへ設定し、導入した実CLIで両形式の画像とPlantUMLの一致を確認します。画像とPlantUMLは指定先に残し、一時bucketとumlgenを削除した後は元の`PLANTUML_JAR`設定へ戻します。CIでも同じ経路を実行し、画像をartifactへ保存します。
