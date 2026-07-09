export default {
  batchImageGuide: {
    title: '图片批量生成',
    description: '一次提交多条提示词，任务完成后可统一下载图片结果'
  },
  // Home Page
  home: {
    viewOnGithub: '在 GitHub 上查看',
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
      bottomNav: '底部快速导航',
      endpoints: 'API 接入端点'
    },
    cursor: {
      home: '首页',
      light: '浅色',
      dark: '暗色',
      enter: '进入',
      login: '登录',
      start: '开始',
      about: '关于',
      copy: '复制',
      read: '阅读',
      close: '关闭'
    },
    nav: {
      about: '关于',
      features: '案例',
      workflow: '流程',
      models: '服务',
      pricing: '定价'
    },
    bottomNav: {
      about: '关于',
      work: '案例',
      process: '流程',
      services: '服务',
      contact: '开始'
    },
    hero: {
      badge: '多模型网关 / 可观测计费 / 协议兼容',
      posterStatement: '把所有 AI 模型入口，收束成一个真正可靠的网关。',
      posterSubstatement: '为已经跑起来的产品与团队，补上配得上规模的接入层。',
      metaLatency: '路由矩阵在线',
      metaModels: 'Claude · OpenAI · Gemini · Grok',
      metaControl: '用量 · 计费 · 故障切换',
      scrollCue: '查看接入方式',
      quickstartKicker: '网关接入面',
      titleLine1: '一个密钥',
      titleLine2: '接入所有模型',
      description:
        'Sub2API 将 Claude、OpenAI、Gemini、Grok、Qwen 等模型收束到一个可观测、可计费、可调度的 API 入口。保留官方协议体验，同时把稳定性和成本控制交给网关。',
      ctaPrimary: '获取 API Key',
      ctaDocs: '查看文档',
      baseUrlOpenai: 'OpenAI 兼容',
      baseUrlAnthropic: 'Anthropic 兼容',
      copy: '复制',
      copied: '已复制',
      cards: {
        routing: { title: '智能调度', desc: '多通道负载均衡，故障自动切换' },
        observability: { title: '实时可观测', desc: '每次调用的用量与费用尽在掌握' },
        billing: { title: '按量计费', desc: '用多少付多少，无固定月费' }
      }
    },
    visual: {
      gatewayLabel: '路由矩阵',
      gatewayMeta: '实时网关'
    },
    work: {
      kicker: '精选案例',
      index: '01 / 04'
    },
    value: {
      kicker: '价值',
      title: '为什么选择 Sub2API',
      subtitle: '专业、稳定、开发者友好',
      items: {
        unified: {
          title: '统一接入',
          desc: '一个密钥调用全部模型，完整兼容 OpenAI Responses / Chat 与 Anthropic Messages 协议，现有代码低成本迁移。'
        },
        observability: {
          title: '全链路可观测',
          desc: '请求数、Token、费用实时统计，按模型与密钥多维分析，每一分钱花在哪里一目了然。'
        },
        elastic: {
          title: '弹性调度与成本',
          desc: '多上游智能调度、自动故障切换，SSE 流式全接口支持，稳定快速且按量计费。'
        },
        developer: {
          title: '为开发者而建',
          desc: 'Claude Code、Codex、Gemini CLI 等工具一键脚本接入，官方 SDK 直接可用。'
        }
      }
    },
    workflow: {
      kicker: '流程',
      title: '三步完成迁移',
      subtitle: '从注册到发出第一个请求，只需几分钟。',
      steps: {
        register: {
          title: '注册获取 Key',
          desc: '免费注册账号，在控制台密钥管理中创建你的 API Key。'
        },
        configure: {
          title: '替换 Base URL',
          desc: '把 Base URL 指向 Sub2API：OpenAI 用 /v1，Claude 用根地址，其余保持不变。'
        },
        observe: {
          title: '调用与观测',
          desc: '用官方 SDK 或 CLI 工具直接调用，在控制台实时查看用量、费用与通道状态。'
        }
      }
    },
    ecosystem: {
      kicker: '服务',
      title: '主流模型与工具，一个入口全部连接',
      subtitle: '主流大模型持续接入，一个入口全部搞定',
      more: '更多持续接入'
    },
    pricing: {
      kicker: '定价',
      title: '限时充值优惠',
      subtitle: '透明按量计费，余额永不过期。',
      rateLabel: '充值汇率',
      rateValue: '¥1 = $1',
      officialLabel: '官方汇率参考',
      officialValue: '$1 ≈ ¥7.2',
      badge: '限时优惠',
      note: '按官方模型定价扣费，充值即享超低汇率。',
      cta: '立即充值'
    },
    cta: {
      kicker: '联系',
      title: '让你的 AI 调用有稳定入口。',
      description: '免费注册，几分钟内发出第一条可观测、可计费、可调度的 AI 请求。',
      button: '开始接入 Sub2API'
    },
    about: {
      open: '关于 Sub2API',
      close: '关闭',
      eyebrow: '关于网关',
      title: '为高频 AI 调用建立一个可信入口。',
      body:
        'Sub2API 不是又一个装饰性的模型列表，而是把账号池、密钥、计费、故障切换和可观测性放在同一个控制面里。你继续使用熟悉的官方协议，平台负责让每一次请求有去处、有记录、有成本边界。',
      est: '创立于 2024',
      based: '为开发者与运维而建',
      principles: {
        outcomes: {
          title: '结果优先',
          desc: '每个能力都围绕稳定调用、低迁移成本和可解释费用展开。'
        },
        signal: {
          title: '信号清晰',
          desc: '用量、模型、费用、错误和通道状态都要能被快速理解。'
        },
        human: {
          title: '开发者友好',
          desc: '保持官方 SDK 与 CLI 的使用习惯，让接入不打断工作流。'
        },
        pace: {
          title: '长期稳定',
          desc: '多账号、多通道与限流策略共同降低单点风险。'
        }
      }
    },
    footer: {
      allRightsReserved: '保留所有权利。',
      console: '控制台',
      apiExamples: 'API 示例'
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
    cacheCreationTokens: '缓存创建',
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
      password: '密码（可选）',
      database: '数据库',
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
