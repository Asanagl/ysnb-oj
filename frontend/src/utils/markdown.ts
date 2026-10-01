import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js'
import katex from 'katex'

// OJ problem statements need code highlight and math; markdown-it with
// plugins covers both without a heavy bundle. html is on because the rich
// editor (TipTap) stores HTML in statement_md — off would render stored
// statements as escaped tags.
const md: MarkdownIt = new MarkdownIt({
  html: true,
  linkify: true,
  highlight(code: string, lang: string) {
    if (lang && hljs.getLanguage(lang)) {
      return `<pre class="hljs"><code>${hljs.highlight(code, { language: lang }).value}</code></pre>`
    }
    return `<pre class="hljs"><code>${md.utils.escapeHtml(code)}</code></pre>`
  },
})

// $...$ inline and $$...$$ block math, rendered after markdown to HTML.
// The replacement is naive: $...$ inside code blocks is rendered too, but
// statement bodies rarely nest the two.
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

// Sanitization must be the LAST step of the render chain: user HTML may
// embed $...$ inside an attribute value, and math rendering would splice
// quote-bearing KaTeX output out of that attribute. Sanitizing the final
// string catches the escape; DOMPurify defaults keep KaTeX's MathML and
// inline styles intact.
export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html)
}

export function renderStatement(src: string): string {
  return sanitizeHtml(renderMath(md.render(src ?? '')))
}
