# Performance baseline

umlgenの性能回帰を検出するため、ファイル探索、Java解析、関係解決、PlantUML生成、warm cacheを個別に計測する。package図ではJava／Goの解析・文脈束縛、graph構築、SCC／focus投影、PlantUML生成も分離する。画像レンダリングと実CLI全体は別に測る。

## 実行方法

```bash
make bench
```

または：

```bash
go test ./internal/benchmarks -run '^$' -bench . -benchmem
```

fixtureはベンチマーク内で決定的に生成する。ファイル探索は100／1,000ファイル、その他のフェーズは100／1,000型を中心に計測する。

package用fixtureは100／1,000型、10／100ファイル・package・辺を生成する。1ファイルに10型を置き、次のpackageを1回importし、各型が次のpackageの対応する型をフィールドで参照する有向ring。型関係は100／1,000件、import宣言は10／100件、全体が1つのSCCになる。Goでは意図的にコンパイル不可能な循環を含む構文解析用データである。

| Benchmark | 計測境界 |
| --- | --- |
| PackageParseContext | メモリー上の全ソースのparse＋言語adapterのBindContext。Goのmodule探索を含み、scan・ソース読取・cache I/O・graph構築・暗黙実装推定を除く |
| PackageBuildResolve | parse／束縛済みSourceUnitからsnapshot・索引・型／import解決・集約・安定順序まで。declared-types／imports／bothを別計測 |
| PackageProjection | 構築済みgraphの投影。cycles off、全体SCC、SCC＋depth 2の双方向focusを別計測 |
| GeneratePackagePlantUML | 解析済み全体／focus viewからテキスト生成のみ。SCC再計算・外部レンダラーは含まない |

focusでは5 package・4辺を表示し、元の10／100 packageのSCCを保持する。出力の`files/op`、`types/op`、`packages/op`、`edges/op`は入力規模、`visible-*/op`は投影後の規模。既存class benchmarkの単一package fixtureは変更していない。

## 参考値

2026-07-23、Apple M4 Max、Darwin arm64、Go 1.24系で1回計測した値。環境差があるため絶対的な性能保証ではなく、同一環境で変更前後を比較する基準として使用する。

| Benchmark | 規模 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| ScanJavaFiles | 100 files | 71,736 | 62,512 | 536 |
| ScanJavaFiles | 1,000 files | 669,006 | 645,219 | 5,053 |
| ParseJavaSource | 10 types | 72,169 | 47,088 | 681 |
| ParseJavaSource | 100 types | 685,060 | 453,507 | 6,624 |
| ResolveRelations | 100 types | 97,785 | 131,866 | 2,466 |
| ResolveRelations | 1,000 types | 1,346,055 | 1,905,079 | 31,713 |
| GeneratePlantUML | 100 types | 213,742 | 313,773 | 6,847 |
| GeneratePlantUML | 1,000 types | 3,161,072 | 4,444,901 | 100,905 |
| WarmJavaCache | 100 types | 212,975 | 133,448 | 933 |

同じ100型の単一Javaソースでは、warm cacheの読み込みは再解析の約31%の時間（約3.2倍高速）だった。実プロジェクトでの改善率はファイルサイズ、ストレージ、キャッシュヒット率によって変わる。

## フェーズの再計測（2026-11-04）

Apple M4 Max、Darwin arm64、Go 1.26.4。`go test ./internal/benchmarks -run '^$' -bench . -benchmem -count=3`成功、45条件を各3回計測した中央値。既存fixtureを変えずに再計測した値と旧参考値との差は以下のとおり。旧計測はGo 1.24系の単回測定であり、この作業の変更前／後を統制比較したものではない。統計的な退行判定や新しい許容閾値には使わない。

| 既存Benchmark | ns/op | B/op | allocs/op | 旧参考値からの時間差 |
| --- | ---: | ---: | ---: | ---: |
| ScanJavaFiles/files_100 | 70,796 | 36,816 | 336 | -1.3% |
| ScanJavaFiles/files_1000 | 592,279 | 373,121 | 3,053 | -11.5% |
| ParseJavaSource/types_10 | 88,568 | 57,720 | 918 | +22.7% |
| ParseJavaSource/types_100 | 854,414 | 556,267 | 8,931 | +24.7% |
| ResolveRelations/types_100 | 102,411 | 133,984 | 2,363 | +4.7% |
| ResolveRelations/types_1000 | 1,408,203 | 1,917,006 | 30,721 | +4.6% |
| GeneratePlantUML/types_100 | 243,252 | 342,816 | 6,969 | +13.8% |
| GeneratePlantUML/types_1000 | 3,583,789 | 4,799,614 | 101,958 | +13.4% |
| WarmJavaCache | 251,763 | 151,233 | 836 | +18.2% |

Java parseは旧参考値より約23〜25%、classテキスト生成は約13〜14%長い。原因を今回のpackage受入追加へ帰属させる対照実験はしていない。性能を改善したとは扱わず、次回以降はこの同一fixture・実行条件を比較の起点にする。

package各phaseの代表値（Buildはboth、投影は全体SCC、生成は解析済みview）:

| 言語 | 型数 | phase | ns/op | B/op | allocs/op |
| --- | ---: | --- | ---: | ---: | ---: |
| Java | 100 | parse + context | 979,041 | 588,164 | 9,511 |
| Java | 100 | build both | 168,603 | 288,633 | 2,676 |
| Java | 100 | SCC + projection | 5,421 | 11,240 | 37 |
| Java | 100 | generate full | 15,365 | 26,774 | 205 |
| Java | 100 | generate focus | 12,854 | 19,714 | 175 |
| Java | 1,000 | parse + context | 10,154,221 | 5,882,020 | 95,101 |
| Java | 1,000 | build both | 2,019,744 | 2,970,979 | 26,375 |
| Java | 1,000 | SCC + projection | 56,604 | 98,136 | 130 |
| Java | 1,000 | generate full | 163,240 | 257,531 | 1,839 |
| Java | 1,000 | generate focus | 110,664 | 190,939 | 1,179 |
| Go | 100 | parse + context | 278,971 | 321,443 | 4,551 |
| Go | 100 | build both | 168,748 | 327,053 | 2,866 |
| Go | 100 | SCC + projection | 5,498 | 11,240 | 37 |
| Go | 100 | generate full | 17,270 | 29,690 | 225 |
| Go | 100 | generate focus | 14,858 | 27,595 | 192 |
| Go | 1,000 | parse + context | 3,107,579 | 3,238,233 | 45,511 |
| Go | 1,000 | build both | 2,032,917 | 3,355,590 | 28,287 |
| Go | 1,000 | SCC + projection | 57,666 | 98,136 | 130 |
| Go | 1,000 | generate full | 183,745 | 274,889 | 2,039 |
| Go | 1,000 | generate focus | 120,216 | 199,231 | 1,285 |

imports／declared-types単独のBuild、SCCなし・focusありの投影も同じコマンドで計測する。package生成は型ではなく集約されたpackage端点を描くため、既存class生成との時間差だけで実装効率は比較しない。focusでも元のSCCメンバー・範囲の凡例を保持するので、描く端点を5個に絞っても出力量や全体処理が定数になるわけではない。

## 実CLIのclass／package比較（2026-11-04）

Apple M4 Max、Darwin arm64、Go 1.26.4。同一ビルドで各条件5回、経過時間の中央値と`/usr/bin/time -l`のpeak RSSの最大値。画像・`--explain`は含まない。プロセス起動、探索、ソース読取、必要なcache I/O、関係解決／package集約、投影、PlantUMLファイル書出しを含む。

両言語とも上記と同じring構造を一時ディレクトリに生成した。Javaは`bench.pNNN`に`Type0`〜`Type9`を置き、wildcard importと完全修飾フィールド参照、Goは`example.test/bench/pNNN`に同名の10 structを置き、通常importと値型フィールド参照を使用。phase benchmarkとは名前・宣言のバイト列が異なるため、phase時間の足し算とは比較しない。coldは各回空のumlgen cache、warmは事前に1回生成したcache、no-cacheは`--no-cache`。OSのページcacheは消去していない。

各セルは**経過ms / peak MiB**。

| 言語 | 型数 | コマンド | cold | warm | no-cache |
| --- | ---: | --- | ---: | ---: | ---: |
| Java | 100 | class | 10.55 / 13.28 | 7.20 / 12.33 | 7.33 / 12.64 |
| Java | 100 | package | 9.66 / 13.34 | 7.19 / 12.31 | 7.27 / 12.89 |
| Java | 1,000 | class | 39.96 / 18.70 | 16.30 / 17.39 | 22.62 / 18.55 |
| Java | 1,000 | package | 37.33 / 18.98 | 15.82 / 17.19 | 22.35 / 18.69 |
| Go | 100 | class | 8.88 / 12.91 | 7.61 / 12.50 | 6.83 / 12.30 |
| Go | 100 | package | 9.09 / 13.09 | 7.52 / 12.77 | 6.74 / 12.69 |
| Go | 1,000 | class | 34.79 / 18.62 | 22.53 / 17.88 | 19.40 / 17.95 |
| Go | 1,000 | package | 34.02 / 19.27 | 21.91 / 18.50 | 18.87 / 18.69 |

測定コマンドの形（`<ring-root>`は上記の構造で生成したソース）:

```sh
go build -o /tmp/umlgen-perf ./cmd/umlgen
UMLGEN_CACHE_DIR=/tmp/umlgen-perf-cache /usr/bin/time -l \
  /tmp/umlgen-perf package <ring-root> --language go \
  --dependency-source both --quiet --output /tmp/packages.puml
```

class比較では`package`を`class`へ変更し、`--dependency-source both`を外す。小さい入力では起動とcacheのJSON読取費用も影響し、Goのwarmは今回no-cacheより遅い。warmが常に高速、packageが常にclassより高速とは保証しない。旧Javaの単一ソース測定の「約3.2倍」をGoやCLI全体へ一般化しない。

## 外部画像レンダラーの費用

上記CLI測定とは別に、[利用例](../examples/package-diagrams/README.md)のJava／Goの`both --external show`を横配置・shadowing=falseで生成したPlantUMLを`plantuml -tsvg -pipe`／`-tpng -pipe`へ渡した。PlantUML 1.2026.6、Temurin OpenJDK 26.0.1、Graphviz 15.1.0。各3回の中央値、毎回のJVM起動・標準入出力を含む外部プロセスの経過時間であり、umlgenの探索・parse・graph構築・ファイル書出しは含まない。

| 入力 | 形式 | 外部renderer経過ms |
| --- | --- | ---: |
| Java（5 package・6辺） | SVG | 828.72 |
| Java（5 package・6辺） | PNG | 878.05 |
| Go（6 package・6辺） | SVG | 825.10 |
| Go（6 package・6辺） | PNG | 881.23 |

これは小さな図の参考値。レイアウト費用はgraph形状・図の大きさに依存し、1,000型のphase測定や別入力のCLI時間とは加算しない。SVGのpackage名・辺件数とPNGの端点・循環ラベル・凡例を別途確認した。

## 運用

- 性能に影響する変更では、同一マシンで変更前後を複数回計測する
- `benchstat`を利用する場合は、各状態を`-count=10`以上で取得する
- 大きな悪化がある場合は、scan、parse、relations、generationのどの段階かを切り分ける
- ベンチマークfixtureを変更した場合は、この文書の規模と参考値も更新する
