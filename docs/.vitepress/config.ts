import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'
import type { DefaultTheme } from 'vitepress'

const docsRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const dayjsEsm = path.join(docsRoot, 'node_modules/dayjs/esm/index.js')

const githubRepo = 'https://github.com/go-idp/inlets'

// GitHub Pages project URL: https://go-idp.github.io/inlets/
const siteBase = '/inlets/'

const sharedTheme = {
  socialLinks: [{ icon: 'github', link: githubRepo }],
  search: {
    provider: 'local' as const,
  },
}

const enSidebar: DefaultTheme.SidebarItem[] = [
  {
    text: 'Introduction',
    collapsed: false,
    items: [
      { text: 'What is Inlets', link: '/guide/introduction' },
      { text: 'Install', link: '/guide/install' },
      { text: 'Quick start', link: '/guide/quick-start' },
      { text: 'Examples', link: '/guide/examples' },
    ],
  },
  {
    text: 'Server',
    collapsed: false,
    items: [
      { text: 'Overview', link: '/server/overview' },
      { text: 'Run', link: '/server/run' },
      { text: 'Configuration', link: '/server/configuration' },
      { text: 'Advanced', link: '/server/advanced' },
    ],
  },
  {
    text: 'Client',
    collapsed: false,
    items: [
      { text: 'Overview', link: '/client/overview' },
      { text: 'HTTP tunnel', link: '/client/http-tunnel' },
      { text: 'TCP tunnel', link: '/client/tcp-tunnel' },
      { text: 'Authentication', link: '/client/authentication' },
      { text: 'Advanced', link: '/client/advanced' },
    ],
  },
  {
    text: 'Reference',
    collapsed: false,
    items: [
      { text: 'Architecture', link: '/reference/architecture' },
      { text: 'CLI reference', link: '/reference/cli' },
    ],
  },
  {
    text: 'Deep dives',
    collapsed: true,
    items: [
      { text: 'Admin config visualization', link: '/features/ADMIN_CONFIG_VISUALIZATION' },
      { text: 'Public monitor session TTL', link: '/features/PUBLIC_MONITOR_SESSION' },
      { text: 'New protocol notes', link: '/features/NEW_PROTOCOL_ISSUES' },
      { text: 'Release notes (2026-03-15)', link: '/features/RELEASE_NOTES_2026-03-15' },
    ],
  },
  {
    text: 'Community',
    collapsed: true,
    items: [
      { text: 'FAQ', link: '/community/faq' },
      { text: 'Changelog', link: '/community/changelog' },
    ],
  },
]

const zhSidebar: DefaultTheme.SidebarItem[] = [
  {
    text: '入门',
    collapsed: false,
    items: [
      { text: '什么是 Inlets', link: '/zh/guide/introduction' },
      { text: '安装', link: '/zh/guide/install' },
      { text: '快速上手', link: '/zh/guide/quick-start' },
      { text: '使用示例', link: '/zh/guide/examples' },
    ],
  },
  {
    text: '服务端',
    collapsed: false,
    items: [
      { text: '概述', link: '/zh/server/overview' },
      { text: '运行', link: '/zh/server/run' },
      { text: '配置文件', link: '/zh/server/configuration' },
      { text: '进阶', link: '/zh/server/advanced' },
    ],
  },
  {
    text: '客户端',
    collapsed: false,
    items: [
      { text: '概述', link: '/zh/client/overview' },
      { text: 'HTTP 隧道', link: '/zh/client/http-tunnel' },
      { text: 'TCP 隧道', link: '/zh/client/tcp-tunnel' },
      { text: '鉴权', link: '/zh/client/authentication' },
      { text: '进阶', link: '/zh/client/advanced' },
    ],
  },
  {
    text: '参考',
    collapsed: false,
    items: [
      { text: '架构', link: '/zh/reference/architecture' },
      { text: '命令行参考', link: '/zh/reference/cli' },
    ],
  },
  {
    text: '专题',
    collapsed: true,
    items: [
      { text: 'Admin 配置管理可视化', link: '/zh/features/ADMIN_CONFIG_VISUALIZATION' },
      { text: '公共监控会话时限', link: '/zh/features/PUBLIC_MONITOR_SESSION' },
      { text: '新协议说明', link: '/zh/features/NEW_PROTOCOL_ISSUES' },
      { text: '发行说明（2026-03-15）', link: '/zh/features/RELEASE_NOTES_2026-03-15' },
    ],
  },
  {
    text: '其他',
    collapsed: true,
    items: [
      { text: '常见问题', link: '/zh/community/faq' },
      { text: '更新日志', link: '/zh/community/changelog' },
    ],
  },
]

export default withMermaid(
  defineConfig({
    base: siteBase,
    cleanUrls: true,

    head: [
      ['link', { rel: 'icon', type: 'image/svg+xml', href: `${siteBase}logo.svg` }],
    ],

    mermaid: {
      startOnLoad: false,
      theme: 'neutral',
      securityLevel: 'loose',
      flowchart: {
        htmlLabels: true,
        useMaxWidth: true,
      },
    },

    // Mermaid (and VitePress) use `import dayjs from 'dayjs'`. Under pnpm, Vite can
    // resolve the UMD `dayjs.min.js`, which has no ESM `default` in dev. Only the
    // bare specifier `dayjs` is aliased — `dayjs/plugin/*` must keep working.
    vite: {
      resolve: {
        dedupe: ['dayjs'],
        alias: [{ find: /^dayjs$/, replacement: dayjsEsm }],
      },
      optimizeDeps: {
        include: ['dayjs', 'mermaid'],
      },
      ssr: {
        noExternal: ['dayjs'],
      },
    },

    locales: {
      root: {
        label: 'English',
        lang: 'en',
        title: 'Inlets Go',
        titleTemplate: ':title · Inlets Go',
        description:
          'High-availability Go implementation of the inlets tunnel system (HTTP & TCP over WebSocket).',
        themeConfig: {
          ...sharedTheme,
          nav: [
            { text: 'Home', link: '/' },
            { text: 'Introduction', link: '/guide/introduction' },
            { text: 'Server', link: '/server/overview' },
            { text: 'Client', link: '/client/overview' },
            { text: 'Reference', link: '/reference/architecture' },
            { text: 'GitHub', link: githubRepo },
          ],
          sidebar: {
            '/': enSidebar,
          },
          editLink: {
            pattern: `${githubRepo}/edit/master/docs/:path`,
            text: 'Edit this page on GitHub',
          },
          footer: {
            message: 'Documentation for the Inlets Go project.',
            copyright: 'Copyright © present contributors',
          },
        },
      },
      zh: {
        label: '简体中文',
        lang: 'zh-CN',
        link: '/zh/',
        title: 'Inlets Go',
        titleTemplate: ':title · Inlets Go',
        description:
          'Inlets 隧道系统的 Go 实现（HTTP 与 TCP，基于 WebSocket），高可用客户端与服务端。',
        themeConfig: {
          ...sharedTheme,
          nav: [
            { text: '首页', link: '/zh/' },
            { text: '入门', link: '/zh/guide/introduction' },
            { text: '服务端', link: '/zh/server/overview' },
            { text: '客户端', link: '/zh/client/overview' },
            { text: '参考', link: '/zh/reference/architecture' },
            { text: 'GitHub', link: githubRepo },
          ],
          sidebar: {
            '/zh/': zhSidebar,
          },
          editLink: {
            pattern: `${githubRepo}/edit/master/docs/:path`,
            text: '在 GitHub 上编辑此页',
          },
          footer: {
            message: 'Inlets Go 项目文档',
            copyright: 'Copyright © 贡献者',
          },
        },
      },
    },
  }),
)
