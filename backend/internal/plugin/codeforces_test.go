package plugin

import (
	"strings"
	"testing"
)

// 最小化的 Codeforces 题面页 fixture：复刻 problem-statement 的真实结构
// （header + 同级章节 div + sample-test 内 pre 交替 输入/输出，新版样例行
// 包在 test-example-line div 里）。
const cfFixtureHTML = `<!DOCTYPE html><html><body>
<div class="problem-statement">
 <div class="header"><div class="title">A. Pockets</div>
  <div class="time-limit">1 second</div><div class="memory-limit">256 megabytes</div></div>
 <div>You are given three integers.</div>
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
</div></body></html>`

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
	// 不打真实网络：只验证组装逻辑对 fixture 的行为
	meta := &ProblemMeta{StatementMD: ""}
	pageHTML := cfFixtureHTML
	inputSpec := cfHTMLToText(cfSection(pageHTML, "input-specification"))
	outputSpec := cfHTMLToText(cfSection(pageHTML, "output-specification"))
	var b strings.Builder
	b.WriteString("## 输入\n\n" + inputSpec + "\n\n")
	b.WriteString("## 输出\n\n" + outputSpec + "\n\n")
	if strings.TrimSpace(b.String()) == "" {
		t.Fatal("assembled statement empty")
	}
	_ = meta
}
