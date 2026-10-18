# HTML 报告格式

架构复查渲染为操作系统临时目录中的单个自包含 HTML 文件。Tailwind 与 Mermaid 都来自 CDN。Mermaid 可靠地处理图状图表；手工搭的 div 与内联 SVG 处理更编辑化的视觉（体量图、剖面图）。两者混用：不要什么都靠 Mermaid，它会开始显得千篇一律。

## 骨架

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Architecture review for {{repo name}}</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script type="module">
      import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs";
      mermaid.initialize({ startOnLoad: true, theme: "neutral", securityLevel: "loose" });
    </script>
    <style>
      /* small custom layer for things Tailwind doesn't cover cleanly:
         dashed seam lines, hand-drawn-feeling arrow heads, etc. */
      .seam { stroke-dasharray: 4 4; }
      .leak { stroke: #dc2626; }
      .deep { background: linear-gradient(135deg, #0f172a, #1e293b); }
    </style>
  </head>
  <body class="bg-stone-50 text-slate-900 font-sans">
    <main class="max-w-5xl mx-auto px-6 py-12 space-y-12">
      <header>...</header>
      <section id="candidates" class="space-y-10">...</section>
      <section id="top-recommendation">...</section>
    </main>
  </body>
</html>
```

## 页眉

仓库名、日期，以及一个紧凑的图例：实心方框 = 模块，虚线 = 接缝（seam），红色箭头 = 泄漏，粗深色方框 = 深模块（deep module）。不要引言段落。直接进入候选方案。

## 候选卡片

图表承载重量。文字稀疏、平实，并毫不客套地使用（来自 `/codebase-design` 技能的）词表术语。

每个候选方案是一个 `<article>`：

- **Title**：简短，点出这次加深（例如 “Collapse the Order intake pipeline”）。
- **Badge row**：推荐强度（`Strong` = emerald，`Worth exploring` = amber，`Speculative` = slate），外加一个依赖类别标签（`in-process`、`local-substitutable`、`ports & adapters`、`mock`）。
- **Files**：等宽列表，`font-mono text-sm`。
- **Before / After diagram**：核心件。两栏并排。见下面的模式。
- **Problem**：一句话。哪里疼。
- **Solution**：一句话。改什么。
- **Wins**：项目符号，每条 ≤6 个词。例如 “Tests hit one interface”、“Pricing logic stops leaking”、“Delete 4 shallow wrappers”。
- **ADR callout**（如适用）：一行，放在琥珀色调的框里。

不要解释性段落。如果一张图需要一段话来让人看懂，就重画那张图。

## 图表模式

挑适合该候选方案的模式。混着用。不要每张图都长得一样。多样性本身就是重点的一部分。

### Mermaid 图（处理依赖 / 调用流的干活主力）

当要点是“X 调 Y，Y 调 Z，看看这团乱麻”时，用 Mermaid 的 `flowchart` 或 `graph`。把它包进 Tailwind 样式的卡片里，免得显得像空降的。用 classDef 把泄漏边染红、把深模块染深。时序图很适合“之前：6 次往返；之后：1 次”。

```html
<div class="rounded-lg border border-slate-200 bg-white p-4">
  <pre class="mermaid">
    flowchart LR
      A[OrderHandler] --> B[OrderValidator]
      B --> C[OrderRepo]
      C -.leak.-> D[PricingClient]
      classDef leak stroke:#dc2626,stroke-width:2px;
      class C,D leak
  </pre>
</div>
```

### 手工搭的方框与箭头（当 Mermaid 的布局跟你作对时）

模块就是带边框和标签的 `<div>`。箭头是绝对定位在相对定位容器之上的内联 SVG `<line>` 或 `<path>`。当你希望“之后”的图看起来是一个粗边框的深模块、内部灰掉时，就用这个，因为 Mermaid 渲染不出那种恰当的分量。

### 剖面图（适合分层的浅）

把横向条带（`h-12 border-l-4`）叠起来，展示一次调用穿过的各层。之前：6 层薄薄的、每层什么也没做。之后：1 条厚带，标着合并后的职责。

### 体量图（适合“接口和实现一样宽”）

每个模块两个矩形：一个表示接口的表面积，一个表示实现。之前：接口矩形几乎和实现矩形一样高（浅）。之后：接口矩形短，实现矩形高（深）。

### 调用图折叠

之前：函数调用树渲染成嵌套方框。之后：同一棵树折叠成一个方框，那些现在变成内部的调用以淡出的形式显示在它里面。

## 样式指引

- 偏向编辑化，而不是企业仪表盘。留白要慷慨。标题可选衬线体（`font-serif` 与 stone/slate 搭配得很好）。
- 颜色用得克制：一个强调色（emerald 或 indigo），加上用于泄漏的红色和用于警告的琥珀色。
- 图表保持约 320px 高，好让前后对比并排坐得舒服、不用滚动。
- 图内的模块标签用 `text-xs uppercase tracking-wider`，让它们读起来像示意图，而不是 UI。
- 唯一的脚本是 Tailwind CDN 和 Mermaid ESM 引入。报告在其余方面是静态的：没有应用代码，除 Mermaid 自身的渲染外没有交互。

## Top recommendation 章节

一张更大的卡片。候选方案名，一句话说明为什么，以及指向它那张卡片的锚点链接。就这样。

## 语气

平实的英语，简洁，但架构名词和动词直接来自 `/codebase-design` 技能。简洁不是漂移的借口。

**准确使用：**module、interface、implementation、depth、deep、shallow、seam、adapter、leverage、locality。

**绝不替换成：**component、service、unit（代替 module）· API、signature（代替 interface）· boundary（代替 seam）· layer、wrapper（当你指的是 module 时）。

**符合这种风格的措辞：**

- “Order intake module is shallow: interface nearly matches the implementation.”
- “Pricing leaks across the seam.”
- “Deepen: one interface, one place to test.”
- “Two adapters justify the seam: HTTP in prod, in-memory in tests.”

**Wins 项目符号**用词表术语点出收益：*“locality: bugs concentrate in one module”*、*“leverage: one interface, N call sites”*、*“interface shrinks; implementation absorbs the wrappers”*。不要写 *“easier to maintain”* 或 *“cleaner code”*，因为这些术语不在词表里，也不配占位置。

不要含糊其辞，不要清嗓子，不要“值得注意的是……”。如果一句话可以变成一条项目符号，就把它变成项目符号。如果一条项目符号可以砍掉，就砍掉。如果某个术语不在 `/codebase-design` 词表里，先去够一个在里面的，再考虑发明新的。
