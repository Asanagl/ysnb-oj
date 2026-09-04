import { defineConfig } from 'vitepress'

// YSNB OJ 文档站：任务导向（我要做什么 → 怎么做 → 怎么验证 → 举例）。
// 结构即读者分型：使用者 / 部署运维 / 开发者 / API 参考 / 历史存档。
export default defineConfig({
  title: 'YSNB OJ',
  description: '完全自研的在线评测系统——文档与运维手册',
  lang: 'zh-CN',
  cleanUrls: true,
  // 仓库主页的 README 面向开源访客；本配置面向部署/运维/开发者
  themeConfig: {
    nav: [
      { text: '首页', link: '/' },
      { text: '使用指南', link: '/guide/user-guide' },
      { text: '部署与运维', link: '/operations/deploy' },
      { text: '开发', link: '/development/architecture' },
      { text: 'API 参考', link: '/reference/api' },
      {
        text: '仓库',
        link: 'https://github.com/Asanagl/ysnb-oj',
      },
    ],
    sidebar: {
      '/guide/': [
        {
          text: '使用指南',
          items: [
            { text: '选手手册', link: '/guide/user-guide' },
            { text: '管理与出题手册', link: '/guide/admin-guide' },
            { text: 'SPJ 与交互题约定', link: '/guide/interactive' },
          ],
        },
      ],
      '/operations/': [
        {
          text: '部署与运维',
          items: [
            { text: '部署', link: '/operations/deploy' },
            { text: '运维 Runbook', link: '/operations/maintenance' },
          ],
        },
      ],
      '/development/': [
        {
          text: '开发',
          items: [
            { text: '接手导读', link: '/development/handover' },
            { text: '架构总览', link: '/development/architecture' },
            { text: '技术栈与配置', link: '/development/tech-stack' },
            { text: '判题机与沙箱', link: '/development/judge-sandbox' },
            { text: '项目展示（脱敏）', link: '/development/showcase' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'API 参考',
          items: [{ text: '公开 API', link: '/reference/api' }],
        },
      ],
      '/archive/': [
        {
          text: '历史存档（只读）',
          items: [
            { text: '安全审计记录', link: '/archive/security-audit' },
            { text: '上线 Readiness 快照', link: '/archive/launch-readiness' },
            { text: 'E2E 测试编年史', link: '/archive/e2e-report' },
            { text: '早期云端联调报告', link: '/archive/cloud-test-report' },
          ],
        },
      ],
    },
    outline: { level: [2, 3], label: '本页目录' },
    search: { provider: 'local' },
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdated: { text: '最后更新', formatOptions: { dateStyle: 'short' } },
    editLink: {
      pattern: 'https://github.com/Asanagl/ysnb-oj/edit/main/docs/:path',
      text: '在 GitHub 上编辑此页',
    },
    footer: {
      message: 'You Submit, Never Be rejected.',
      copyright: 'MIT License · YSNB OJ',
    },
  },
})
