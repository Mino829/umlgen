# Performance baseline

umlgenの性能回帰を検出するため、ファイル探索、Java解析、関係解決、PlantUML生成、warm cacheを個別に計測する。

## 実行方法

```bash
make bench
```

または：

```bash
go test ./internal/benchmarks -run '^$' -bench . -benchmem
```

fixtureはベンチマーク内で決定的に生成する。ファイル探索は100／1,000ファイル、その他のフェーズは100／1,000型を中心に計測する。

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

## 運用

- 性能に影響する変更では、同一マシンで変更前後を複数回計測する
- `benchstat`を利用する場合は、各状態を`-count=10`以上で取得する
- 大きな悪化がある場合は、scan、parse、relations、generationのどの段階かを切り分ける
- ベンチマークfixtureを変更した場合は、この文書の規模と参考値も更新する
