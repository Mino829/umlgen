# umlgen自身のパッケージ依存図

[`umlgen-packages.puml`](umlgen-packages.puml)は、このリポジトリのGoソースを`umlgen package`で解析したPlantUMLです。リポジトリのルートで次を実行すると再生成できます。

```sh
make diagram
```

実際のCLI呼び出しは`Makefile`を参照してください。対象は`cmd/umlgen`と`internal`以下のGo packageです。`testdata`と`examples`は入力から除外し、モジュール外の依存は図に表示しません。`imports`を選ぶことで、Goのimport宣言に基づくパッケージ間の辺を表示します。辺の件数は実行時の呼び出し回数ではありません。Go build tagsや外部依存の本文は解析しません。

CIのLinuxジョブは同じコマンドでPlantUMLを再生成し、コミット済みの図と一致するか確認します。さらにローカルPlantUMLでSVGを生成し、PlantUMLとSVGをCI artifactとして保存します。手元でSVGを作る場合は、PlantUMLをインストールして次を実行してください。

```sh
plantuml -tsvg docs/diagrams/umlgen-packages.puml
```

図のartifact名は`umlgen-self-package-diagram`です。配布物の集約は`umlgen-linux-amd64`・`umlgen-darwin-arm64`・`umlgen-darwin-amd64`・`umlgen-windows-amd64`だけを選び、図を配布ディレクトリへ混在させません。集約後の「正確に4アーカイブ」とchecksumの検証は維持します。

ソース変更で図が変わった場合は`make diagram`を実行し、生成差分を同じPRに含めてください。
