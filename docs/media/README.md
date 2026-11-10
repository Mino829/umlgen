# 紹介動画の素材と検証範囲

READMEに掲載するumlgen v0.6.0の短い紹介素材です。

| ファイル | 仕様 |
| --- | --- |
| [umlgen-demo.mp4](umlgen-demo.mp4) | 42.3秒、1920×1080、30fps、H.264 / AAC、日本語音声・字幕 |
| [umlgen-preview.gif](umlgen-preview.gif) | 12秒、960×540、10fps、無音ループ |
| [umlgen-poster.png](umlgen-poster.png) | 1920×1080、静止画プレビュー |

## 実行結果と映像上の表現

正式リリースv0.6.0のGoサンプルで `class`、`--focus UserService --depth 1`、`--explain` を実行確認した結果を使用しています。動画中の `shop` はリポジトリ内の `testdata/go/compatibility` をコピーした作業ディレクトリ名です。

ターミナルは実ログを再組版した表示で、図は実生成結果から関係を抜粋・再配置しています。全体9型からfocus6型への変化は、このサンプルの表示型数です。`UserService → Repository` のfield依存は `service/service.go:11` の `repository Repository` に対応します。Goの埋め込みやinterface実装全般をコンパイラー検証済みとする映像ではありません。

紹介はクラス図の生成とfocus・explainに絞っています。インストールや各コマンドの詳細はルートREADMEと各ドキュメントを参照してください。Java対応の紹介は対象版の仕様に基づき、映像内の実行例にはGoを使用しています。

## 素材とライセンス

- umlgenのサンプルと生成結果：本リポジトリの[MIT License](../../LICENSE)。
- 映像・画像の図形と組版：この紹介のために制作した素材。第三者の写真・映像素材は使用していません。
- ナレーション：制作者がOpenAI Platformで生成したCedar音声。AI生成音声であることを動画とREADMEに明記しています。OpenAIの利用条件に従い、音声にMITやCC0での再許諾は主張しません。
- BGM：サイン波と和音から手続生成したオリジナル音源。第三者の録音・サンプルなし。生成したBGM出力はCC0-1.0。
- フォント：macOS付属のHiragino SansとMenlo/SFMono系を映像化したもの。フォントファイルは含めていません。

## 品質確認

30fpsのMP4全1,269フレームとGIF全120フレームをデコードし、形式・尺・音声の区間を確認しました。映像は他の配信用素材と合わせて文字の実描画領域を測定し、はみ出し・文字同士の重なりを検査しています。音声は元原稿と休止位置に基づいて段落単位で対応させています。自動音声認識による全文照合は行っていません。
