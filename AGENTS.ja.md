# AGENTS.md(日本語)

このリポジトリで作業する AI コーディングエージェント(および人間)向けの
ガイド。English version: [AGENTS.md](AGENTS.md).

## このプロジェクトの目的

`mermaid2pptx` は、Mermaid が出力した SVG(または `mermaid-cli` 経由の `.mmd`)
を、**PowerPoint ネイティブの編集可能なオブジェクト**(プリセット図形、図形に
接続されたコネクタ、テキストボックス)で構成された `.pptx` に変換する。画像
エクスポータではない。ユーザーが PowerPoint で開き、全要素を移動・編集できる
ことが目標。

変換結果を変える前に、ユーザー向けの挙動を `README.ja.md` で確認すること。

## ビルド・テスト・検証

```sh
go build -o mermaid2pptx ./cmd/mermaid2pptx   # ビルド
go test ./...                                 # ユニット + パッケージテスト
go vet ./... && gofmt -l .                    # クリーンであること(gofmt は何も出力しない)
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/mermaid2pptx  # クロスコンパイル確認
```

変更を終える前に必ず `gofmt -l .`・`go vet ./...`・`go test ./...` を実行する。
生成される `.pptx` に影響する変更の場合は、出力そのものも検証すること(下記)。
well-formed XML テストが通ることは、スライドの見た目が正しいことを保証しない。

### 生成物の検証

テストは既に以下を検証している: サンプルごとの要素数、ノード形状の種別、
コネクタ端点の座標、パッケージ内の全 XML パートが well-formed で図形がスライド
外に出ないこと。ジェネレータを変えたら、サンプルを再生成し `python-pptx`
(環境に導入済み)で確認する:

```sh
./mermaid2pptx -f sample/graph*.svg
python3 -c "from pptx import Presentation; p=Presentation('sample/graph1.pptx'); print(len(p.slides[0].shapes))"
```

最も強い検証は `TestConnectorEndpointsMatchSVG`。各エッジコネクタの端点を生成
XML から復元(flip/回転を戻す)し、SVG の `data-points` を同じ px→EMU 変換に
通した値と ~1 EMU 以内で一致することを検証する。この変換は `slide.go` の
`fitTransform` が単一の情報源で、ジェネレータとテストで共有している。幾何を
触ったらこのテストがガード。常にグリーンに保つこと。

#### 任意: PowerPoint 自身でレンダリングする (macOS)

`python-pptx` もテストも、こちらが書いた XML を読んでいるだけで、PowerPoint が
レイアウト時に何をするかは見ていない。`scripts/` 配下のプレーンテキスト
AppleScript 2 本がアプリケーション自身を検証に組み込む。これは**任意の手動
手順**で、macOS と Microsoft PowerPoint を必要とする。`go test` からは到達
しないので、テストは Go ツールチェインだけの任意のプラットフォームで通る。

```sh
osascript scripts/export-pdf.applescript sample
osascript scripts/connector-following.applescript sample/graph1.pptx VALID 40 -30
```

`export-pdf.applescript` はフォルダ内の各 `.pptx` を隣に PDF として書き出す。
PowerPoint のロックファイル (`~$` 前置き) と既に PDF がある deck はスキップし、
converted/skipped/failed を集計する。ラベルの再折り返しや図形の潰れはこれで見る。

`connector-following.applescript` は PDF では答えられない問いを扱う。指定ノードを
指定オフセット (pt) だけ動かし、`<deck>-moved.pptx` とその PDF を元ファイルの隣に
書き出し、全コネクタ端点の移動前後を報告する。動かしたノードに接続された端点は
移動するはずだが、接続済み端点はその時点で正対する接続サイトへ再ルーティング
されるため、与えたオフセットぴったりには動かない。ノード図形の名前は Mermaid の
ノード id、コネクタは `edge <from>-<to>` なので、名前は `.mmd` からそのまま分かる。

差分でレビューできるよう、コンパイル済み `.scpt` ではなく `.applescript` を
管理している。`osascript` はプレーンテキストをそのまま実行でき、ダブルクリック
可能な droplet が要るときは `osacompile` で作れる。

## アーキテクチャ

パイプライン: **SVG をパース → `Diagram` モデル → `slide1.xml`(DrawingML)を
生成 → zip で `.pptx` 化**。CLI はファイルを繋ぐだけで、`.mmd` の場合は先に
`mmdc` を呼んで一時 SVG を作る。

- `cmd/mermaid2pptx/main.go` — フラグ、`.mmd`→SVG レンダリング(`mmdc`)、
  ファイル I/O。
- `internal/convert/model.go` — 中間型: `Diagram` / `Node` / `Cluster` /
  `Edge` / `EdgeLabel` / `Line` / `TextBox` と `ShapeKind`。Mermaid のデフォルト
  テーマ色もここ。
- `internal/convert/svg.go` — 共通パーサ。`ParseMermaidSVG` がルートの
  `aria-roledescription` から型を判別してディスパッチする。`parseGraphDiagram`
  は dagre ベースの型(flowchart / state / class / er)を担当し、これらは
  `nodes / edgePaths / edgeLabels / clusters` の DOM 構造を共有する。
- `internal/convert/compartment.go` — class/ER の区画箱(タイトル・メンバー・
  属性)を、箱 + テキストボックス + 区切り線に分解する。
- `internal/convert/sequence.go` — sequence 図パーサ(rect / line / text の
  平面配置。text は最小の外接 rect に取り込む)。
- `internal/convert/geom.go` — コネクタ配置の幾何: エッジがどの辺から出入り
  するか、プリセットコネクタに合わせる flip/rotate 変換。
- `internal/convert/slide.go` — DrawingML 生成(`GenerateSlideXML`)と px→EMU
  変換。`Options` にフォントと余白を持つ。
- `internal/convert/skeleton.go` — `.pptx` パッケージの静的パート
  (`slide1.xml` 以外すべて)と zip ライタ。

### 座標系

- SVG ピクセル、y 下向き。viewBox 原点(`MinX`/`MinY`)は非ゼロになりうる
  (sequence 図)。常に差し引くこと。
- OOXML は EMU: 96 dpi で `9525 EMU = 1 px`。スライドは 12192000 x 6858000 EMU。
- 複合状態などのネスト内容は translate された `<g class="root">` の中にある。
  パーサは walk しながら祖先の `translate()` を累積し、各 `parseX` は
  `dx, dy` オフセットを受け取る。ノードの transform を絶対座標だと仮定しない。

## 規約

- **標準ライブラリのみ。** production 依存を追加しない。module require の追加は
  必ず先にユーザーへ確認する。(`python-pptx` と `mermaid-cli` は外部の
  開発・検証ツールであって Go 依存ではない。)
- コードコメントは英語。周囲のコードと同程度の密度で、*何を* ではなく *なぜ*
  を説明する。
- `Diagram` モデルに追記していく小さく焦点の絞れた関数を好む。XML 文字列生成は
  `slide.go` / `skeleton.go` に閉じ込める。
- 生成 XML は `strings.Builder` への `fmt.Fprintf` で組み立てる。ユーザーテキスト
  は必ず `xmlEsc` を通す。

## 落とし穴(実際に踏んだもの)

- **Mermaid の DOM はバージョン間で変わる。** サンプルは mermaid-cli 11.15 で
  生成した。図形は固定パスではなく構造で検出している — 例: スタジアムノードは
  rect ではなく `<g>` で包まれたベジェ `<path>`、ひし形 `<polygon>` はノードの
  transform に加えて自身の `translate()` を持つ。mermaid 更新後に出力が壊れたら、
  コードを「直す」前に SVG 構造を再調査すること。
- **OOXML に対応する線端が無い矢印がある**(白抜き継承三角、カラスの足)。
  `arrowType` で意図的に近似している。
- **プリセットコネクタで表現できない経路がある。** U ターンや多段折れは
  フリーフォーム折れ線にフォールバックする。端点は正確だが曲線は Mermaid と
  異なる。
- エッジ端点はまずエッジ id(`L_from_to_n` / `id_from_to_n`)から解決し、
  次に最近傍境界の幾何で解決する。ノード id にアンダースコアが含まれうるため、
  id 分割は既知 id 集合に照合して検証する。
- **出力フォントを測る仕組みはパイプライン内に存在しない**(標準ライブラリのみの
  ためフォントメトリクスを持てない)。幅は Mermaid がブラウザ自身のフォントスタックで
  測った実測値に由来する。例外は `sequence.go` で、読み取れる実測ボックスが SVG に
  無いため `estTextWidth` が文字種ごとに推定する。`slide.go` の `labelWidthSafety`
  は既定フォントぶんの幅の差を吸収するもので、それより大幅に幅の広いフォント(等幅
  など)を `-font` に指定すると、エッジラベルに限らずノードラベルでも箱からはみ出す。
  任意のフォントを賄える値まで広げると既定出力が恒常的に緩くなるため、既定フォントに
  合わせてある。

## 新しいダイアグラム型の追加

1. `mmdc` でサンプルを生成し、SVG 構造を調べる(ルートの
   `aria-roledescription`、ノード/エッジのクラス、`data-points` の有無)。
2. dagre ベース(`nodes`/`edgePaths` を共有)なら `parseGraphDiagram` を拡張し、
   既存のノード/エッジパースを再利用する。そうでなければ `sequence.go` のような
   専用パーサを書く。
3. 図形をモデルの `ShapeKind` / `Line` / `TextBox` にマッピングする。新しい幾何が
   必要なときだけ `slide.go` に描画を追加する。
4. `sample/graphN.mmd` を追加してレンダリングし、`TestParseGraphN` と
   `TestGeneratePackage` へのファイル追加を行う。

## リポジトリ上の注意

- `_sandbox/` は内部資料で git 管理外。コミットしたり、その内容を追跡対象ファイル
  や生成物にコピーしたりしないこと。
- `sample/*.svg` はテストフィクスチャ(テストから参照される)、`sample/*.mmd` は
  その元、`sample/*.pptx` はデモ用の出力。ジェネレータを変えたら同期させること。
- 顧客名・口座番号その他の個人を特定できる情報を、コード・サンプル・生成物に
  含めないこと。
