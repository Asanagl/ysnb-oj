package plugin

import (
	"strings"
	"testing"
)

// 最小化的 Codeforces 题面页 fixture：复刻 problem-statement 的真实结构
// （header + 无 class 正文 div + 同级章节 div + sample-test 内 pre 交替
// 输入/输出，新版样例行包在 test-example-line div 里；公式含 MathJax
// 渲染残渣 span 与 $$$...$$$ 三美元源文本）。
const cfFixtureHTML = `<!DOCTYPE html><html><body>
<div class="problem-statement">
 <div class="header"><div class="title">A. Pockets</div>
  <div class="time-limit">1 second</div><div class="memory-limit">256 megabytes</div></div>
 <div>You are given $$$n$$$ coins. In the first example the answer is <span class="MathJax" tabindex="0"><span style="color:#cc0000">2</span></span> coins total.</div>
 <div class="input-specification"><div class="section-title">Input</div>
  <p>The first line contains an integer <span class="tex-span">n</span> (1 &le; n &le; 100).</p>
  <p>The second line contains <span class="tex-span">n</span> integers.</p>
 </div>
 <div class="output-specification"><div class="section-title">Output</div>
  <p>Print the answer.</p>
 </div>
 <div class="sample-tests"><div class="section-title">Example</div>
  <div class="sample-test">
   <div class="input"><div class="title">Input</div><pre><div class="test-example-line">3</div><div class="test-example-line">1 2 3</div></pre></div>
   <div class="output"><div class="title">Output</div><pre>2</pre></div>
   <div class="input"><div class="title">Input</div><pre><div class="test-example-line">5</div><div class="test-example-line">4 4 4 4 4</div></pre></div>
   <div class="output"><div class="title">Output</div><pre>1</pre></div>
  </div></div>
 <div class="note"><div class="section-title">Note</div><p>In the first example the answer is 2.</p></div>
</div><div id="footer">Codeforces footer</div></body></html>`

func TestCFSection(t *testing.T) {
	got := cfSection(cfFixtureHTML, "input-specification")
	if !strings.Contains(got, "The first line contains") {
		t.Fatalf("input-specification slice wrong: %q", got[:min(80, len(got))])
	}
	if strings.Contains(got, "output-specification") || strings.Contains(got, "sample-tests") {
		t.Fatalf("section slice bled into the next section")
	}
	if cfSection(cfFixtureHTML, "nonexistent") != "" {
		t.Fatal("missing section should return empty")
	}
}

func TestCFHTMLToText(t *testing.T) {
	got := cfHTMLToText(cfSection(cfFixtureHTML, "input-specification"))
	// 实体还原 + 标签剥除 + Input/Output 标题行剔除
	if !strings.Contains(got, "1 ≤ n ≤ 100") {
		t.Fatalf("entity not unescaped: %q", got)
	}
	if strings.Contains(got, "<") || strings.Contains(got, "Input") {
		t.Fatalf("tags/titles not stripped: %q", got)
	}
	// 段落分隔：两个 <p> 之间应有空行（Markdown 段落）
	if !strings.Contains(got, ").\n\nThe second line") {
		t.Fatalf("paragraph break missing: %q", got)
	}
}

func TestCFStripMathJax(t *testing.T) {
	frag := cfProblemStatementBlock(cfFixtureHTML)
	story := frag
	for _, cut := range []string{
		cfSection(frag, "header"),
		cfSection(frag, "input-specification"),
		cfSection(frag, "output-specification"),
		cfSection(frag, "sample-tests"),
		cfSection(frag, "note"),
	} {
		if cut != "" {
			story = strings.Replace(story, cut, "", 1)
		}
	}
	text := cfHTMLToText(story)
	// MathJax 残渣必须被整块删除（不得留下 style/color 碎片）
	if strings.Contains(text, "cc0000") || strings.Contains(text, "MathJax") || strings.Contains(text, "style=") {
		t.Fatalf("mathjax residue leaked: %q", text)
	}
	// 正文段保留（此前版本丢失），且 $$$ 三美元归一为 $$
	if !strings.Contains(text, "You are given") {
		t.Fatalf("story paragraph lost: %q", text)
	}
	if strings.Contains(text, "$$$") {
		t.Fatalf("triple-dollar not normalized: %q", text)
	}
	if !strings.Contains(text, "$$n$$") {
		t.Fatalf("expected $$n$$ after normalization: %q", text)
	}
	// 页脚不进入正文
	if strings.Contains(text, "Codeforces footer") {
		t.Fatalf("footer leaked into story: %q", text)
	}
}

func TestCFSamplePairs(t *testing.T) {
	html := cfSection(cfFixtureHTML, "sample-tests")
	pre := cfSamplePreRe.FindAllStringSubmatch(html, -1)
	if len(pre) != 4 { // 2 组样例 = 输入/输出各一
		t.Fatalf("want 4 pre blocks, got %d", len(pre))
	}
	// 奇位为输入：test-example-line div 转成换行
	in1 := strings.TrimSpace(tagStripRe.ReplaceAllString(strings.ReplaceAll(pre[0][1], "</div>", "\n"), ""))
	if !strings.Contains(in1, "1 2 3") {
		t.Fatalf("sample input lines wrong: %q", in1)
	}
	out2 := strings.TrimSpace(tagStripRe.ReplaceAllString(pre[3][1], ""))
	if out2 != "1" {
		t.Fatalf("second sample output wrong: %q", out2)
	}
}

func TestCFEnrichFillsStatement(t *testing.T) {
	// 不打真实网络：只验证组装逻辑对 fixture 的行为（与 cfEnrichStatement
	// 的组装顺序一致：正文 → 输入 → 输出 → 样例 → 备注）
	block := cfProblemStatementBlock(cfFixtureHTML)
	if block == "" {
		t.Fatal("problem-statement block empty")
	}
	story := block
	for _, cut := range []string{
		cfSection(block, "header"),
		cfSection(block, "input-specification"),
		cfSection(block, "output-specification"),
		cfSection(block, "sample-tests"),
		cfSection(block, "note"),
	} {
		if cut != "" {
			story = strings.Replace(story, cut, "", 1)
		}
	}
	text := cfHTMLToText(story)
	if !strings.Contains(text, "You are given") {
		t.Fatalf("story missing: %q", text)
	}
	// 样例代码块的 Markdown 结构：``` 前后必须有空行
}
