import { defineConfig } from 'vitepress'

// YSNB OJ 文档站：任务导向（我要做什么 → 怎么做 → 怎么验证 → 举例）。
// 结构即读者分型：使用者 / 部署运维 / 开发者 / API 参考 / 历史存档。
// 双语：root = 简体中文，/en/ = English（docs/en/ 镜像目录）。
const zh = {
  label: '简体中文',
  lang: 'zh-CN',
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
            { text: 'BPF LSM 试点（未实施）', link: '/development/bpf-lsm-pilot' },
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
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdated: { text: '最后更新', formatOptions: { dateStyle: 'short' } },
    editLink: {
      pattern: 'https://github.com/Asanagl/ysnb-oj/edit/main/docs/:path',
      text: '在 GitHub 上编辑此页',
    },
  },
}

const en = {
  label: 'English',
  lang: 'en-US',
  link: '/en/',
  themeConfig: {
    nav: [
      { text: 'Home', link: '/en/' },
      { text: 'Guide', link: '/en/guide/user-guide' },
      { text: 'Ops', link: '/en/operations/deploy' },
      { text: 'Development', link: '/en/development/architecture' },
      { text: 'API', link: '/en/reference/api' },
      {
        text: 'GitHub',
        link: 'https://github.com/Asanagl/ysnb-oj',
      },
    ],
    sidebar: {
      '/en/guide/': [
        {
          text: 'Guide',
          items: [
            { text: 'User guide', link: '/en/guide/user-guide' },
            { text: 'Admin & authoring', link: '/en/guide/admin-guide' },
            { text: 'SPJ & interactive', link: '/en/guide/interactive' },
          ],
        },
      ],
      '/en/operations/': [
        {
          text: 'Deploy & operate',
          items: [
            { text: 'Deployment', link: '/en/operations/deploy' },
            { text: 'Ops runbook', link: '/en/operations/maintenance' },
          ],
        },
      ],
      '/en/development/': [
        {
          text: 'Development',
          items: [
            { text: 'Handover', link: '/en/development/handover' },
            { text: 'Architecture', link: '/en/development/architecture' },
            { text: 'Stack & config', link: '/en/development/tech-stack' },
            { text: 'Judge & sandbox', link: '/en/development/judge-sandbox' },
            { text: 'BPF LSM pilot (planned)', link: '/en/development/bpf-lsm-pilot' },
            { text: 'Showcase', link: '/en/development/showcase' },
          ],
        },
      ],
      '/en/reference/': [
        {
          text: 'API reference',
          items: [{ text: 'Public API', link: '/en/reference/api' }],
        },
      ],
    },
    outline: { level: [2, 3], label: 'On this page' },
    docFooter: { prev: 'Previous', next: 'Next' },
    lastUpdated: { text: 'Last updated', formatOptions: { dateStyle: 'medium' } },
    editLink: {
      pattern: 'https://github.com/Asanagl/ysnb-oj/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
  },
}

export default defineConfig({
  title: 'YSNB OJ',
  description: '完全自研的在线评测系统——文档与运维手册',
  cleanUrls: true,
  // Pages 部署在仓库子路径下，没有 base 会全站丢样式
  base: '/ysnb-oj/',
  // 仓库主页的 README 面向开源访客；本配置面向部署/运维/开发者
  locales: { root: zh, en },
  themeConfig: {
    search: { provider: 'local' },
    footer: {
      message: 'You Submit, Never Be rejected.',
      copyright: 'MIT License · YSNB OJ',
    },
  },
})
