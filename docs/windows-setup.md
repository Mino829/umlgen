# Windowsではじめるumlgen

この手順は、コマンド操作に慣れていない方向けです。Windows 10／11の64-bit PCを対象にしています。管理者としてPowerShellを起動する必要はありません。

## 1. PowerShellを開く

スタートメニューを開き、`PowerShell`と入力して「Windows PowerShell」を起動します。

黒または青い画面が表示されたら、次のコマンドを1行ずつ貼り付けてEnterを押します。

```powershell
$installer = "$env:TEMP\install-umlgen.ps1"
Invoke-WebRequest https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-windows.ps1 -OutFile $installer
powershell -NoProfile -ExecutionPolicy Bypass -File $installer -InstallPlantUML
```

この操作で次をインストールします。

- `umlgen.exe`
- SVG生成用のPlantUML
- コマンド名だけで起動するためのユーザーPATH設定

インストール先は`%LOCALAPPDATA%\Programs\umlgen`です。クラス図の生成と同梱PlantUMLによるSVG出力には、管理者権限やJavaの別途インストールは不要です。v0.6.0のJava `sequence`／`activity`／`state`には、**別途フルJDK 17以上**をPATHに設定し、`java -version`と`javac -version`を確認してください（PlantUMLの実行環境では代用できません）。class／package／componentおよびclass／packageのwatchはJDK不要です。

インストーラーはGitHub Releasesからだけファイルを取得し、Release APIに記録されたSHA-256と一致することを確認してから展開します。

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

SVG・PNG画像も生成する場合：

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

同じ3行のインストールコマンドをもう一度実行すると、最新Releaseへ更新できます。設定済みのPATHは重複しません。

特定バージョンを使う場合：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File $installer -Version v0.6.0 -InstallPlantUML
```

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

```powershell
& "$env:LOCALAPPDATA\Programs\umlgen\umlgen.exe" version
```

これで動く場合は、Windowsへサインインし直すとPATHが反映されます。

### スクリプトの実行が禁止されている

手順にある`-ExecutionPolicy Bypass`を含めて実行してください。この指定はインストーラーを実行する1回だけに適用され、PC全体の設定は変更しません。

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
