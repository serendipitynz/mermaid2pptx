# mermaid2pptx

*English version: [README.md](README.md).*

Mermaid のダイアグラムを、PowerPoint の編集可能なオブジェクト
(図形・コネクタ・テキスト)で構成された .pptx に変換する CLI ツール。
(mermaid-js 公式のツールではありません)

対応ダイアグラム: **flowchart / stateDiagram-v2 / classDiagram / erDiagram /
sequenceDiagram**(SVG ルートの `aria-roledescription` で自動判別)。

Go 標準ライブラリのみで実装。mac / Windows 向けに単一バイナリとして
クロスコンパイルできる。入力は mermaid が出力した SVG(依存なし)、
または `.mmd`(PATH 上の mermaid-cli を自動で呼び出して SVG 化)。

## 構成

```
cmd/mermaid2pptx/ CLI エントリポイント
internal/convert/ SVG パーサ・DrawingML 生成・PPTX パッケージ生成
sample/           サンプル (.mmd / mermaid-cli で生成した .svg /
                  本ツールで変換した .pptx)
```

サンプルの対象範囲:

- graph1 — flowchart LR・ノード形状網羅(スタジアム・平行四辺形・ひし形・
  サブルーチン・円・円柱・六角形)・逆方向エッジ・太線
- graph2 — flowchart ネストしたサブグラフ・点線・DB 円柱
- graph3 — flowchart classDef 色付け・循環エッジ・エッジラベル
- graph4 — flowchart `%%{init}%%`(curve: basis / wrappingWidth)・長文ラベルの
  自動折り返し・クラスタ間に立つひし形ゲート
- graph5 — stateDiagram-v2(開始/終了状態・複合状態・遷移ラベル)
- graph6 — sequenceDiagram(アクター・ライフライン・activation・Note・破線応答)
- graph7 — classDiagram(インターフェース・実現・集約)
- graph8 — erDiagram(エンティティ属性表・カーディナリティ)

## インストール

```sh
go install github.com/serendipitynz/mermaid2pptx/cmd/mermaid2pptx@latest
```

## ビルド

```sh
go build -o mermaid2pptx ./cmd/mermaid2pptx

# Windows 向け
GOOS=windows GOARCH=amd64 go build -o mermaid2pptx.exe ./cmd/mermaid2pptx
```

## 使い方

```sh
./mermaid2pptx input.svg              # input.pptx を生成(既存ファイルは上書きしない)
./mermaid2pptx input.mmd              # mmdc を呼び出して .mmd から直接変換
./mermaid2pptx -f input.svg           # 上書きを許可
./mermaid2pptx -o out.pptx input.svg  # 出力先を指定
./mermaid2pptx a.svg b.mmd            # 複数まとめて変換(混在可)
./mermaid2pptx -font "Meiryo" a.svg   # フォント指定(既定: Noto Sans JP)
./mermaid2pptx -margin 0.5 a.svg      # スライド余白(インチ、既定: 0.3)
./mermaid2pptx -mmdc /path/to/mmdc a.mmd  # mermaid-cli の場所を指定
./mermaid2pptx -version               # バージョン情報を表示して終了

# サンプルの再生成
./mermaid2pptx -f sample/graph*.svg
```

オプションは入力ファイルの前後・間のどこに置いてもよい
(`./mermaid2pptx a.svg -o out.pptx -f` も動作する)。

`.mmd` 入力には [mermaid-cli](https://github.com/mermaid-js/mermaid-cli)
(`npm install -g @mermaid-js/mermaid-cli`)が必要。SVG 入力は外部依存なしで
動作する。

## サンプルの SVG 生成

`sample/*.svg` は mermaid-cli で生成している:

```sh
mmdc -i sample/graph1.mmd -o sample/graph1.svg -I my-svg -b white
```

## 変換仕様

共通:

- スライドは 16:9(13.33 x 7.5 in)。図全体をアスペクト比を保って
  余白内にフィットさせ、フォントサイズも同率でスケールする
  (小さい図の拡大は 1.25 倍まで)。
- 塗り・枠線・文字色は SVG のスタイルをそのまま反映。
- `<br/>` は段落区切り、`<b>`/`<i>` は太字・斜体として変換。

flowchart / stateDiagram:

- ノード → PowerPoint プリセット図形にマッピング。
  - 矩形 `[ ]` / サブルーチン風 → `roundRect`
  - ひし形 `{ }` → `diamond`
  - 円柱 `[( )]` → `can`
  - 円 `(( ))` / 開始・終了状態 → `ellipse`
  - スタジアム `([ ])` → `roundRect`(完全な丸角)
  - 六角形 `{{ }}` → `hexagon`
  - その他の多角形(平行四辺形 `[/ /]` 等)→ カスタム図形(頂点をそのまま出力)
- エッジ → PowerPoint コネクタ(`curvedConnector2/3` / `straightConnector1`)。
  始点・終点は `stCxn`/`endCxn` でノード(複合状態はコンテナ)に接続される
  ため、図形を動かすとコネクタが追従する。プリセットで表現できない経路のみ
  フリーフォーム(折れ線)にフォールバック。
- エッジラベル → 枠線なし・半透明背景の角丸矩形。
- サブグラフ / 複合状態 → タイトル入り(上寄せ)の角丸矩形。
- 点線エッジ(`-.->`)は破線、太線エッジ(`==>`)は太い線になる。

classDiagram / erDiagram:

- クラス / エンティティ → 外枠の矩形 1 つ+区画テキスト(タイトル・メンバー・
  属性行)を SVG と同じ位置に置いた独立テキストボックス+区切り線。
- 関係線 → コネクタ(`stCxn`/`endCxn` 接続)。矢印は OOXML の線端で近似:
  継承/実現 `<|--` → 塗り三角、コンポジション/集約 `*--`/`o--` → ひし形、
  依存 → 開き矢印、カラスの足 `}o`/`}|` → 開き矢印、`|o` → 丸。

sequenceDiagram:

- アクター箱 / activation / Note → 矩形(内部にテキストを取り込み)。
- ライフライン → 細線、メッセージ → 矢印付き直線(破線応答対応)。
- メッセージラベル → 枠なしテキストボックス(幅はフォントから推定)。

## 制限

- Mermaid の `htmlLabels`(foreignObject ラベル)前提(sequence は SVG text)。
- エッジの曲線形状はプリセットコネクタによる近似で、Mermaid の
  ベジェ曲線と完全一致はしない(端点は一致)。
- 白抜き三角(継承)・カラスの足など OOXML に無い線端は近似表現。
- class / ER の区画テキストは箱と別オブジェクトのため、箱を動かすと
  テキストは追従しない(グループ化は今後の課題)。
- sequence の loop / alt 枠は汎用変換(線+テキスト)で、専用対応はしていない。
- 図形やラベル背景の大きさは、Mermaid がブラウザ上で自身のフォントスタックで
  測った幅から決めている。例外は sequence のメッセージラベルで、SVG に実測の
  ボックスが無いため文字種ベースの推定幅を使う。いずれにせよ出力フォントを測る
  仕組みは無く、大幅に幅の広いフォント(等幅など)を `-font` に指定すると
  テキストが箱からはみ出すことがある。寸法は既定フォントに合わせて調整してある。

## 貢献

Issue・Pull Request を歓迎します。開発フローと CI で実行されるチェックは
[CONTRIBUTING.ja.md](CONTRIBUTING.ja.md) を参照してください。

## ライセンス

[Apache License, Version 2.0](LICENSE) の下で公開。[NOTICE](NOTICE) および
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) も参照
(`sample/*.svg` には MIT ライセンスの Mermaid が生成したマークアップが埋め込まれている)。

「Mermaid」は mermaid-js プロジェクト、「PowerPoint」/ `.pptx` は Microsoft に
帰属する。本プロジェクトはいずれとも無関係で、これらのツール・形式との
相互運用のみを目的とする。
