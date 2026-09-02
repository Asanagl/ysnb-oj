import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js'
import katex from 'katex'

// OJ problem statements need code highlight and math; markdown-it with
// plugins covers both without a heavy bundle.
const md: MarkdownIt = new MarkdownIt({
  html: false,
  linkify: true,
  highlight(code: string, lang: string) {
    if (lang && hljs.getLanguage(lang)) {
      return `<pre class="hljs"><code>${hljs.highlight(code, { language: lang }).value}</code></pre>`
    }
    return `<pre class="hljs"><code>${md.utils.escapeHtml(code)}</code></pre>`
  },
})

// $...$ inline and $$...$$ block math, rendered after markdown to HTML so
// code blocks are left alone (statement bodies rarely nest the two).
function renderMath(html: string): string {
  html = html.replace(/\$\$([\s\S]+?)\$\$/g, (_, tex: string) => {
    try {
      return katex.renderToString(tex, { displayMode: true, throwOnError: false })
    } catch {
      return _
    }
  })
  return html.replace(/\$([^$\n]+?)\$/g, (_, tex: string) => {
    try {
      return katex.renderToString(tex, { displayMode: false, throwOnError: false })
    } catch {
      return _
    }
  })
}

export function renderStatement(src: string): string {
  return renderMath(md.render(src ?? ''))
}
