---
id: TASK-2
title: Stop PowerPoint from re-wrapping node labels Mermaid laid out on one line
status: In Review
assignee: []
created_date: '2026-09-08 21:29'
updated_date: '2026-09-11 04:05'
labels: []
milestone: m-3
dependencies: []
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
PowerPoint re-breaks node labels that Mermaid rendered on a single line, so the deck does not match the diagram it came from. Rendering the sample decks through PowerPoint itself shows three such labels:

- graph1 "エラー" — ellipse, node box 74.5px, Mermaid text width 47.5px, rendered as "エ" / "ラー"
- graph1 "入力チェック" — diamond, node box 178.8px, text 96px, rendered as "入力" / "チェック"
- graph2 "クライアント" — ellipse, node box 91.2px, text 96px, rendered on two lines

Mermaid's own line count is readable from each foreignObject's height in sample/*.svg (24px per line), so the expected result is checkable against the fixtures: all three are one line there, while "後処理バッチ" is two lines and is already reproduced correctly.

Cause: node labels call writeTxBody without noWrap, so bodyPr carries PowerPoint's default wrap="square". PowerPoint derives a non-rectangular preset's text area from an inscribed region much narrower than the bounding box — a diamond's is roughly half the box in each dimension — so a label that Mermaid measured against the full node box no longer fits and gets re-broken. The text insets are not to blame: they are already emitted as 18288 EMU (~2px), not PowerPoint's 91440 default. For "クライアント" even the full box (91.2px) is narrower than the text (96px); Mermaid lets a circle's label overflow the circle, whereas PowerPoint wraps it instead.

The mechanism to fix this already exists: txBody.noWrap emits wrap="none" and is used for edge labels and free text boxes, just not for node labels.

The catch is that turning wrap off everywhere would regress labels Mermaid itself wrapped. Long labels currently arrive as a single paragraph and rely on PowerPoint's wrapping to break them — no node label in the graph4 output has more than one paragraph today. So either Mermaid's own line breaks have to be reproduced as separate paragraphs before wrapping is disabled, or noWrap has to be applied only where Mermaid used a single line. The first is the more faithful of the two: Mermaid has already decided where the breaks go, and the generator's job is to carry that decision across rather than re-derive it.

Related: the note in AGENTS.md that nothing in the pipeline measures the output font. That warning is about label width and labelWidthSafety; this defect is a different mechanism (PowerPoint's own text area and wrap default), so it should be fixable without retuning that constant.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Every node label that Mermaid rendered on one line renders on one line in PowerPoint, across all samples
- [x] #2 Labels Mermaid itself wrapped keep Mermaid's line breaks; graph4's long descriptive labels do not regress into a single overflowing line
- [x] #3 A test asserts each node label's paragraph/line structure matches the line count implied by the source SVG's foreignObject height
- [x] #4 labelWidthSafety is left unchanged, or any change to it is justified separately
- [x] #5 gofmt -l ., go vet ./..., go test ./... all clean, and the samples are regenerated
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. svg.go の findLabel に、foreignObject が持つレイアウト情報 (ブラウザが折り返した幅 / 折り返して
   いない場合は 0) を返させる。判定は内側 div の white-space: mermaid は 1 行前提のラベルを
   nowrap、折り返したラベルを break-spaces + 固定 width で出すため、折り返しの有無を直接示す
   プロパティを見る (height/24 の行数計算は結果であって原因ではない)。
2. 文字幅モデルを 1 つに統一する: sequence.go の estTextWidth が持つ CJK 16px / ASCII 9px を
   runeWidthPx として切り出し、折り返し器と共用する。標準ライブラリのみの制約でフォント実測は
   できないため、既にある推定を再利用し、新しい推定系を増やさない。
3. 折り返し器 wrapParas を追加。既存の段落 (<br/> 由来) を保ったまま、CJK は文字間、ASCII は
   空白位置で貪欲に分割する。
4. parseNode / parseCluster:
   - nowrap ラベル → LabelNoWrap = true。改行位置は <br/> として SVG に記録済みで、
     段落構造がそのまま mermaid の行構成になる。推定を含まないので wrap を切って安全。
   - ブラウザ折り返しラベル → wrapParas で自前に段落へ分割し、LabelNoWrap は false のまま。
     SVG には改行位置が残っていない (行数と折り返し幅のみ) ため自前推定が避けられず、推定が
     外れたときに図形外へはみ出すより PowerPoint の折り返しを保険として残す方を採る。
     タスク記述は「段落化してから wrap を切る」を推していたが、切るのは推定を含まない側だけに
     限定する。
5. slide.go の writeNode / writeCluster が txBody.noWrap に n.LabelNoWrap を渡す。
   labelWidthSafety には触れない (AC #4)。
6. テスト TestNodeLabelLineCount: 全サンプルについて、SVG の foreignObject height から求めた
   行数 (height/24) と、パース結果のラベル段落数が一致することを assert する。加えて
   1 行ラベルを持つノードの txBody が wrap="none" を伴って出力されることを生成 XML 側で確認。
7. gofmt -l . / go vet ./... / go test ./... / windows クロスコンパイル、サンプル pptx 再生成、
   osascript scripts/export-pdf.applescript での目視確認 (可能な範囲で)。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## 原因と修正

ノードラベルだけ `writeTxBody` に `noWrap` を渡しておらず、`bodyPr` が PowerPoint 既定の
`wrap="square"` のままだった。PowerPoint は非矩形プリセットのテキスト領域を bounding box より
かなり内側に取る (ダイヤはおよそ各辺半分) ため、mermaid がノード箱いっぱいで測ったラベルが
入らなくなり折り返し直されていた。

修正は「行の決定権を生成側が持つ」方針に統一した。生成するラベルの段落 = mermaid が描いた行、
とした上で全ラベルを `wrap="none"` で出す。

## 調査で分かった前提の訂正

タスク記述は「mermaid の改行を段落として再現してから wrap を切る」を推していたが、SVG に改行位置が
残っているのは `<br/>` 由来の複数行 (`white-space: nowrap`、例「後処理<br/>バッチ」) だけだった。
ブラウザ折り返し由来 (`break-spaces` + 固定 width、graph4 の 250px ラベル 5 件) は
foreignObject の height (行数) と折り返し幅しか残っておらず、改行位置は失われている。
そのため実装は 2 系統になる:

- `white-space: nowrap` → パーサが既に `<br/>` を段落へ分解済み。推定は一切入らない。
- それ以外 → `wrapParas` で同じ幅で自前に折り返す (`internal/convert/textwrap.go`)。
  文字幅は `estTextWidth` が持っていた CJK 16px / ASCII 9px を `runeWidthPx` として切り出して共用し、
  推定系を 2 つに増やさないようにした (標準ライブラリのみでフォント実測はできない)。

## 途中で変えた判断

当初は自前折り返し分については `wrap="square"` を保険として残す設計にしていた (推定が外れても
図形外へはみ出さない、という理由)。しかし PowerPoint で実際に描かせたところ、graph4 のダイヤ
「送信内容の検証…」が 2 段落のまま 4 行に割られた。ダイヤの狭いテキスト領域に対しては保険の方が
害になる。mermaid 自身もダイヤや楕円のラベルは図形からはみ出させるので、全ラベル `wrap="none"` に
統一した (AC #2 はこの変更で初めて満たされた)。

## 検証

- `TestNodeLabelLineCount` (新規): 全サンプルについて SVG の foreignObject height から求めた行数と、
  生成 slide XML 上の段落数が一致すること、および `wrap="none"` で出ていることを確認する。
  class/ER はノードラベルがコンパートメントのテキストボックスへ分解され id で対応づけられないため、
  行数の多重集合として比較する (`checkCompartmentLines`)。
- PowerPoint 実描画 (`osascript scripts/export-pdf.applescript sample` → `pdftotext -layout`):
  報告された 3 件 (graph1「エラー」「入力チェック」、graph2「クライアント」) がいずれも 1 行。
  mermaid が 2 行で描いたもの (graph1「後処理/バッチ」、graph2「バックアップ/ストレージ」、
  graph3「審査/スコアリング」) は 2 行のまま。graph4 の折り返しラベル 5 件は 3,3,2,2,2 行で一致。
  graph6/7/8 も崩れなし。
- `osascript scripts/connector-following.applescript sample/graph1.pptx VALID 40 -30` → VERDICT: OK。
- `gofmt -l .` / `go vet ./...` / `go test ./...` / windows クロスコンパイル、いずれもクリーン。
  サンプル .pptx を再生成 (graph1-6 が変化。graph7/8 は元から noWrap 経路のため差分なし)。
- `labelWidthSafety` は未変更 (AC #4)。

## 付随して直した箇所・残っている点

- クラスタ (subgraph / 合成状態) ラベルとエッジラベルも同じ `applyLabelWrap` を通した。
  同一機構の不具合で、片方だけ直すと扱いが分かれるため。ただしブラウザ折り返しされた
  エッジラベルはサンプルに存在せず、その経路は実データで踏めていない。
- 自前折り返しの改行位置は文字幅推定に依存する。CJK は 16px で実測と一致するが ASCII は誤差があり、
  ラテン文字主体の長いラベルでは mermaid と改行位置がずれうる (行数が合うことは
  `TestNodeLabelLineCount` が検出する)。

## 外部レビュー (PR #4, Codex CLI) で変わった点

3 ラウンド回し、上の記述のうち幅推定に関する部分は置き換わった。

- **ラテン文字の幅を実測値に置換 (R2 [P2])**: 「CJK 16px / ASCII 9px」の均一モデルは、改行位置だけでなく
  **行数**まで誤らせることが指摘で判明した。250px 幅で 10 文字の `W` 4 語はブラウザが 4 行で描くのに
  推定では 2 行になり、`wrap="none"` のため図形外へはみ出す。現在は mermaid のスタイルシートが指定する
  フォントスタック先頭 `trebuchet ms` の実 advance を 16px で埋め込んでいる (`latinAdvance16`)。
  検証可能な選択だった点が重要で、`nowrap` ラベルの foreignObject width はブラウザ自身の実測値なので、
  サンプル内のラテン系ラベル 62 件に対し相対誤差の中央値 0.02% (最悪 4.7%、太字のクラス名) で一致する。
  均一 9px は最大 50% ずれていた。これは**入力側**レイアウトの再現であり、出力フォント (`-font`) は
  設計どおり不明のままなので、`labelWidthSafety` の既知の制約は残る (AC #4 の範囲外)。
- **行数較正 `fitLineCount` (R1 [P2])**: SVG が記録している行数に折り返し幅を二分探索で合わせる。
  advance テーブル導入後も残差 (スタック内の別フォント、テーブル外グリフ、擬似ボールド) の吸収役として残す。
- **行高の 24px 固定を廃止 (R2 [P2])**: mermaid の fontSize は設定可能で、20px の 2 行ラベル (60px) が
  3 行と解釈されていた。ルートの font-size (mermaid 自身のスタイルシート) と div の line-height から導出。
- **ラベル単位のフォントサイズ (R3 [P2], 部分対応)**: classDef によるノード個別サイズのうち、
  インラインスタイルで届くものは解決する。**スタイルシート規則として出力される場合は未対応**。
  解決にはノードのクラスと規則の突き合わせが必要だが、この環境に mermaid-cli が無く、実フィクスチャで
  DOM 形状を確認できない。AGENTS.md が「推測した DOM に対して直さない」と定めているため、
  意図的に踏み込まなかった。**フィクスチャを用意して後続タスクとするかはオーナー判断**。
- テスト追加: `TestFitLineCountLatin` / `TestFitLineCountMixedWidths` (幅の広い字・狭い字、改行位置と行幅)、
  `TestLabelLineCountFontSize` / `TestLabelLineCountNodeFontSize` (20px の図、ノード個別サイズ)、
  `TestSequenceLabelsUnwrapped` (graph6。パーサ側ラベルと出力側を段落数で突き合わせ、消失を検出)。
  いずれも修正前に失敗することを確認済み。
- 幅モデル変更に伴い graph4 (CJK に `API` が混ざるラベルで 1 文字分) と graph6 (同じモデルが
  sequence のテキストボックス寸法に効く) を再生成し、PowerPoint 実描画で行数一致を再確認した。

レビュー 3 ラウンドはループ上限。R3 の修正 (`d70a1fc`, `f3e545f`) は再レビュー未実施。
<!-- SECTION:NOTES:END -->
