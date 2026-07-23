export default {
  batchImageGuide: {
    title: '图片批量生成',
    description: '一次提交多条提示词，任务完成后可统一下载图片结果'
  },
  seo: {
    home: {
      title: 'Claude、OpenAI、Gemini AI API 网关',
      description: 'Poke API 是面向开发者与团队的 AI API 网关，一个 API Key 接入 Claude、OpenAI、Gemini 等主流模型，提供透明计费、稳定路由与隐私优先的数据处理。'
    }
  },
  // Home Page
  home: {
    viewDocs: '查看文档',
    docs: '文档',
    switchToLight: '切换到浅色模式',
    switchToDark: '切换到深色模式',
    dashboard: '控制台',
    login: '登录',
    getStarted: '立即开始',
    goToDashboard: '进入控制台',
    aria: {
      primaryNav: '主页主导航',
      footerNav: '页脚快捷导航',
      skipToContent: '跳到主要内容',
      endpoints: 'API 接入端点'
    },
    nav: {
      features: '原则',
      workflow: '接入',
      models: '模型',
      pricing: '定价'
    },
    story: {
      status: 'POKE ROUTING / LIVE',
      scroll: '滚动切换',
      previous: '上一幕',
      next: '下一幕',
      orbitLabel: 'Poke API 协议与模型路由轨道',
      orbitStatus: '路由在线',
      scenes: {
        real: {
          label: '真实模型',
          titleLine1: '满血模型',
          titleLine2: '不掺水',
          subtitle: '模型、协议、价格都摊开说。',
          description: '保留主流官方协议与模型能力，多通道路由降低单点波动；不靠模糊套餐隐藏实际调用。'
        },
        protocol: {
          label: '统一协议',
          titleLine1: '一个密钥',
          titleLine2: '多种协议',
          subtitle: '替换 Base URL，不改变开发习惯。',
          description: '沿用 OpenAI、Anthropic 兼容 SDK 与 CLI，在同一账户中切换主流模型。'
        },
        billing: {
          label: '透明账单',
          titleLine1: '每次调用',
          titleLine2: '都有据可查',
          subtitle: '价格、Token、状态与费用逐笔核对。',
          description: '调用前查看模型价格，调用后核对用量明细，只为实际使用付费。'
        }
      }
    },
    hero: {
      badge: '稳定 · 真实 · 平价',
      posterStatement: '满血模型\n不掺水。',
      posterSubstatement: '一个入口接入主流 AI 模型，价格透明，用量可查。',
      metaModels: 'Claude · OpenAI · Gemini · Grok',
      metaControl: '透明计费 · 用量可查',
      description:
        'PokeAPI 是面向开发者的 AI 聚合中转站。保留主流官方协议与模型能力，通过多通道路由降低单点影响，并按实际用量计费。',
      commitment: '长期运营承诺：绝不跑路。',
      ctaPrimary: '开始使用',
      ctaDocs: '阅读接入文档',
      baseUrlOpenai: 'OpenAI 兼容',
      baseUrlAnthropic: 'Anthropic 兼容',
      websiteNode: '网站节点',
      copy: '复制',
      copied: '已复制'
    },
    value: {
      kicker: '原则',
      title: '稳定、真实\n价格透明',
      subtitle: '不靠夸张承诺，只把模型、价格与用量说清楚。',
      items: {
        unified: {
          title: '真实模型',
          desc: '模型名称、协议与计费规则清楚展示，不用模糊套餐掩盖实际调用。'
        },
        observability: {
          title: '透明计费',
          desc: '按量计费，价格与用量可查，成本边界清楚。'
        },
        elastic: {
          title: '稳定路由',
          desc: '多通道路由与故障切换，尽量减少单点波动对调用的影响。'
        },
        developer: {
          title: '简单接入',
          desc: '兼容常用 SDK 与 CLI，替换 Base URL 和密钥即可接入。'
        }
      }
    },
    workflow: {
      kicker: '接入',
      title: '三步接入\n不改习惯',
      subtitle: '沿用常用 SDK 和 CLI，只替换接入地址与密钥。',
      steps: {
        register: {
          title: '创建账户与密钥',
          desc: '注册后在控制台创建 API Key。'
        },
        configure: {
          title: '替换 Base URL',
          desc: '按文档选择 OpenAI 或 Anthropic 兼容入口，再替换 Base URL 与密钥。'
        },
        observe: {
          title: '发起请求，核对用量',
          desc: '调用后在控制台核对模型、Token、费用与状态。'
        }
      }
    },
    ecosystem: {
      kicker: '模型',
      title: '主流模型，一个入口。',
      subtitle: '实际可用模型以控制台列表为准。',
      more: '查看控制台模型列表'
    },
    pricing: {
      kicker: '定价',
      title: '价格透明\n按量付费',
      subtitle: '模型单价、扣费规则和调用用量公开可查。',
      rateLabel: '计费方式',
      rateValue: '按量计费',
      officialLabel: '当前价格',
      officialValue: '以控制台为准',
      badge: '透明计费',
      note: '充值前看价格，调用后看明细。',
      cta: '进入控制台查看价格'
    },
    docsPanel: {
      kicker: '文档',
      title: '接入文档\n规则写清楚',
      description: '查看协议、Base URL、模型列表、计费与错误处理。',
      button: '打开文档站',
      unavailable: '文档地址暂未配置'
    },
    privacy: {
      kicker: '隐私',
      title: '只处理\n必要数据',
      description:
        '我们不为广告、画像或转售而采集数据。为完成账户、路由、计费与安全防护，只处理这些功能所必需的数据；具体范围以隐私政策为准。',
      minimum: '仅处理账户、路由、计费与安全所必需的数据。',
      noSale: '不出售用户数据。',
      noTraining: 'PokeAPI 不将用户数据用于模型训练。',
      noContent: '不保存提示词、回复正文等 API 请求内容。'
    },
    footer: {
      allRightsReserved: '保留所有权利。',
      backToTop: '返回顶部'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key 用量查询',
    subtitle: '输入您的 API Key 以查看实时消费金额与使用状态',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: '查询',
    querying: '查询中...',
    privacyNote: '您的 Key 仅在浏览器本地处理，不会被存储',
    dateRange: '统计范围:',
    dateRangeToday: '今日',
    dateRange7d: '7 天',
    dateRange30d: '30 天',
    dateRange90d: '90 天',
    dateRangeCustom: '自定义',
    apply: '应用',
    used: '已使用',
    detailInfo: '详细信息',
    tokenStats: 'Token 统计',
    dailyDetail: '按日明细',
    modelStats: '模型用量统计',
    // Table headers
    date: '日期',
    model: '模型',
    requests: '请求数',
    inputTokens: '输入 Tokens',
    outputTokens: '输出 Tokens',
    cacheCreationTokens: '缓存写入',
    cacheReadTokens: '缓存读取',
    cacheWriteTokens: '缓存写入',
    totalTokens: '总 Tokens',
    cost: '费用',
    // Status
    quotaMode: 'Key 限额模式',
    walletBalance: '钱包余额',
    // Ring card titles
    totalQuota: '总额度',
    limit5h: '5 小时限额',
    limitDaily: '日限额',
    limit7d: '7 天限额',
    limitWeekly: '周限额',
    limitMonthly: '月限额',
    // Detail rows
    remainingQuota: '剩余额度',
    expiresAt: '过期时间',
    todayExpires: '(今日到期)',
    daysLeft: '({days} 天)',
    usedQuota: '已用额度',
    resetNow: '即将重置',
    subscriptionType: '订阅类型',
    subscriptionExpires: '订阅到期',
    // Usage stat cells
    todayRequests: '今日请求',
    todayInputTokens: '今日输入',
    todayOutputTokens: '今日输出',
    todayTokens: '今日 Tokens',
    todayCacheCreation: '今日缓存创建',
    todayCacheRead: '今日缓存读取',
    todayCost: '今日费用',
    rpmTpm: 'RPM / TPM',
    totalRequests: '累计请求',
    totalInputTokens: '累计输入',
    totalOutputTokens: '累计输出',
    totalTokensLabel: '累计 Tokens',
    totalCacheCreation: '累计缓存创建',
    totalCacheRead: '累计缓存读取',
    totalCost: '累计费用',
    avgDuration: '平均耗时',
    // Messages
    enterApiKey: '请输入 API Key',
    querySuccess: '查询成功',
    queryFailed: '查询失败',
    queryFailedRetry: '查询失败，请稍后重试',
    noDailyUsage: '暂无按日用量数据',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API 安装向导',
    description: '配置您的 Sub2API 实例',
    database: {
      title: '数据库配置',
      description: '连接到您的 PostgreSQL 数据库',
      host: '主机',
      port: '端口',
      username: '用户名',
      password: '密码',
      databaseName: '数据库名称',
      sslMode: 'SSL 模式',
      passwordPlaceholder: '密码',
      ssl: {
        disable: '禁用',
        require: '要求',
        verifyCa: '验证 CA',
        verifyFull: '完全验证'
      }
    },
    redis: {
      title: 'Redis 配置',
      description: '连接到您的 Redis 服务器',
      host: '主机',
      port: '端口',
      username: '用户名（可选）',
      password: '密码（可选）',
      database: '数据库',
      usernamePlaceholder: '默认用户留空',
      passwordPlaceholder: '密码',
      enableTls: '启用 TLS',
      enableTlsHint: '连接 Redis 时使用 TLS（公共 CA 证书）'
    },
    admin: {
      title: '管理员账户',
      description: '创建您的管理员账户',
      email: '邮箱',
      password: '密码',
      confirmPassword: '确认密码',
      passwordPlaceholder: '至少 8 个字符',
      confirmPasswordPlaceholder: '确认密码',
      passwordMismatch: '密码不匹配'
    },
    ready: {
      title: '准备安装',
      description: '检查您的配置并完成安装',
      database: '数据库',
      redis: 'Redis',
      adminEmail: '管理员邮箱'
    },
    status: {
      testing: '测试中...',
      success: '连接成功',
      testConnection: '测试连接',
      installing: '安装中...',
      completeInstallation: '完成安装',
      completed: '安装完成！',
      redirecting: '正在跳转到登录页面...',
      restarting: '服务正在重启，请稍候...',
      timeout: '服务重启时间超出预期，请手动刷新页面。'
    }
  },

  // Common
}
