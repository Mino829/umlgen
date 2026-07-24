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

インストール先は`%LOCALAPPDATA%\Programs\umlgen`です。管理者権限やJavaの別途インストールは不要です。

インストーラーはGitHub Releasesからだけファイルを取得し、Release APIに記録されたSHA-256と一致することを確認してから展開します。

## 2. インストールを確認する

PowerShellを閉じて、新しく開き直します。次を実行します。

```powershell
umlgen version
```

次のようにバージョンが表示されれば完了です。

```text
umlgen version 0.3.0
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

SVG画像も生成する場合：

```powershell
umlgen class .\src\main\java --format svg
```

次の2ファイルが作成されます。

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

[GitHub Actions導入ガイド](github-actions.md)のworkflowをリポジトリへ追加してください。設定を任せたい場合は[無料導入サポート](onboarding-support.md)を利用できます。

## 更新

同じ3行のインストールコマンドをもう一度実行すると、最新Releaseへ更新できます。設定済みのPATHは重複しません。

特定バージョンを使う場合：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File $installer -Version v0.3.0 -InstallPlantUML
```

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

### `no Java source files found`と表示される

指定したフォルダーの下に`.java`ファイルがあるか確認します。一般的には次の場所です。

```text
src\main\java
```

場所が分からない場合は、プロジェクトの一番上で次を試します。

```powershell
umlgen class . --exclude target --exclude build
```
