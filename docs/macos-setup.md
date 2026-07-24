# Macではじめるumlgen

この手順は、ターミナル操作に慣れていない方向けです。Apple Silicon（M1以降）とIntel Macの両方に対応します。管理者権限は必要ありません。

## 1. ターミナルを開く

`command`キーとスペースキーを同時に押し、`ターミナル`と入力して「ターミナル」を開きます。

次のコマンドを1行ずつ貼り付けてReturnキーを押します。

```bash
installer="$TMPDIR/install-umlgen.sh"
curl -fL https://raw.githubusercontent.com/Mino829/umlgen/main/scripts/install-macos.sh -o "$installer"
bash "$installer"
```

インストーラーはMacの種類を自動判定し、対応するumlgenを`~/.local/bin`へインストールします。GitHub Releaseの`SHA256SUMS.txt`と照合してから配置します。

## 2. インストールを確認する

ターミナルを閉じて、新しく開き直します。次を実行します。

```bash
umlgen version
```

次のようにバージョンが表示されれば完了です。

```text
umlgen version 0.3.0
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

## SVG画像も生成する

SVG生成にはPlantUMLが必要です。すでにHomebrewを利用している場合：

```bash
brew install plantuml
umlgen class ./src/main/java --format svg
```

Homebrewを利用していない場合は、まず`.puml`生成まで利用できます。SVG環境の導入が分からない場合は[無料導入サポート](onboarding-support.md)で相談できます。

## Pull Requestで自動生成する

GitHub Actionsを使うと、MacへPlantUMLを追加しなくてもGitHub上で図を自動生成できます。

[GitHub Actions導入ガイド](github-actions.md)のworkflowをリポジトリへ追加してください。

## 更新

最初に使った3行のインストールコマンドをもう一度実行すると、最新Releaseへ更新できます。PATH設定は重複しません。

特定バージョンを使う場合：

```bash
bash "$installer" --version v0.3.0
```

## アンインストール

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
