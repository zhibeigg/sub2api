import { defineConfig } from "vitepress"

const siteUrl = "https://docs.poke2api.com"
const siteDescription = "Poke API - AI API 网关，一个 API Key 接入 Claude / OpenAI / Gemini"
const brandLogo = "/logo.png?v=0.54.55"

function canonicalPath(relativePath) {
  if (relativePath === "index.md") {
    return "/"
  }

  return `/${relativePath.replace(/\.md$/, "")}`
}

export default defineConfig({
  lang: "zh-CN",
  title: "PokeAPI",
  description: siteDescription,
  cleanUrls: true,
  lastUpdated: true,
  sitemap: {
    hostname: siteUrl
  },
  head: [
    ["link", { rel: "icon", type: "image/png", sizes: "512x512", href: brandLogo }],
    ["link", { rel: "apple-touch-icon", sizes: "512x512", href: brandLogo }],
    ["meta", { name: "theme-color", content: "#111310" }],
    ["meta", { name: "robots", content: "index, follow" }]
  ],
  transformHead({ pageData }) {
    const path = canonicalPath(pageData.relativePath)
    const url = `${siteUrl}${path}`
    const title = pageData.title || "Poke API"
    const description = pageData.description || siteDescription
    const structuredData = {
      "@context": "https://schema.org",
      "@graph": [
        {
          "@type": "WebSite",
          "@id": `${siteUrl}/#website`,
          name: "Poke API 文档",
          url: `${siteUrl}/`,
          inLanguage: "zh-CN",
          description: siteDescription
        },
        {
          "@type": "WebPage",
          "@id": `${url}#webpage`,
          url,
          name: title,
          description,
          inLanguage: "zh-CN",
          isPartOf: { "@id": `${siteUrl}/#website` }
        }
      ]
    }

    return [
      ["link", { rel: "canonical", href: url }],
      ["meta", { property: "og:type", content: "article" }],
      ["meta", { property: "og:site_name", content: "Poke API 文档" }],
      ["meta", { property: "og:url", content: url }],
      ["meta", { property: "og:title", content: title }],
      ["meta", { property: "og:description", content: description }],
      ["script", { type: "application/ld+json" }, JSON.stringify(structuredData)]
    ]
  },
  themeConfig: {
    logo: { src: brandLogo, alt: "PokeAPI" },
    siteTitle: "PokeAPI",
    nav: [
      { text: "首页", link: "/" },
      { text: "快速开始", link: "/guide/getting-started" },
      { text: "API 接入", link: "/guide/api-scripts" },
      { text: "管理员指南", link: "/guide/admin-user-group-restrictions" },
      { text: "控制台", link: "https://www.poke2api.com/home" }
    ],
    sidebar: [
      {
        text: "开始使用",
        collapsed: false,
        items: [
          { text: "快速开始", link: "/guide/getting-started" },
          { text: "Node.js 环境安装", link: "/guide/nodejs" }
        ]
      },
      {
        text: "命令行工具接入",
        collapsed: false,
        items: [
          { text: "Claude Code", link: "/guide/claude-code" },
          { text: "Codex (OpenAI)", link: "/guide/codex" },
          { text: "Gemini CLI", link: "/guide/gemini-cli" },
          { text: "TRAE SOLO", link: "/guide/trae-solo" },
          { text: "OpenClaw", link: "/guide/openclaw" },
          { text: "Hermes", link: "/guide/hermes" }
        ]
      },
      {
        text: "图形化管理工具",
        collapsed: false,
        items: [
          { text: "CC-Switch（图形版）", link: "/guide/cc-switch" },
          { text: "CC-Switch CLI", link: "/guide/cc-switch-cli" }
        ]
      },
      {
        text: "API 接入",
        collapsed: false,
        items: [
          { text: "API 脚本接入", link: "/guide/api-scripts" }
        ]
      },
      {
        text: "管理员指南",
        collapsed: false,
        items: [
          { text: "用户分组访问限制", link: "/guide/admin-user-group-restrictions" },
          { text: "池模式账户容量提醒", link: "/guide/admin-pool-capacity-alerts" }
        ]
      }
    ],
    search: {
      provider: "local",
      options: {
        translations: {
          button: {
            buttonText: "搜索文档",
            buttonAriaLabel: "搜索文档"
          },
          modal: {
            noResultsText: "未找到相关结果",
            resetButtonTitle: "清除查询",
            footer: {
              selectText: "选择",
              navigateText: "切换",
              closeText: "关闭"
            }
          }
        }
      }
    },
    footer: {
      message: "PokeAPI · AI API Gateway",
      copyright: "© 2026 PokeAPI · 保留所有权利"
    },
    outline: { label: "本页目录", level: [2, 3] },
    docFooter: { prev: "上一页", next: "下一页" },
    lastUpdatedText: "最后更新",
    returnToTopLabel: "返回顶部",
    darkModeSwitchLabel: "主题",
    sidebarMenuLabel: "菜单"
  }
})
