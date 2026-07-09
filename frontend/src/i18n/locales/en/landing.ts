export default {
  batchImageGuide: {
    title: 'Batch Image Generation',
    description: 'Submit multiple prompts in one job and download the generated images when complete'
  },
  // Home Page
  home: {
    viewOnGithub: 'View on GitHub',
    viewDocs: 'View Documentation',
    docs: 'Docs',
    switchToLight: 'Switch to Light Mode',
    switchToDark: 'Switch to Dark Mode',
    dashboard: 'Dashboard',
    login: 'Login',
    getStarted: 'Get Started',
    goToDashboard: 'Go to Dashboard',
    aria: {
      primaryNav: 'Primary home navigation',
      bottomNav: 'Bottom quick navigation',
      endpoints: 'API endpoints'
    },
    cursor: {
      home: 'Home',
      light: 'Light',
      dark: 'Dark',
      enter: 'Enter',
      login: 'Login',
      start: 'Start',
      about: 'About',
      copy: 'Copy',
      read: 'Read',
      close: 'Close'
    },
    nav: {
      about: 'About',
      features: 'Work',
      workflow: 'Process',
      models: 'Services',
      pricing: 'Pricing'
    },
    bottomNav: {
      about: 'About',
      work: 'Work',
      process: 'Process',
      services: 'Services',
      contact: 'Start'
    },
    hero: {
      badge: 'Multi-model gateway / Observable billing / Protocol compatible',
      posterStatement: 'One reliable gateway for every AI model your product depends on.',
      posterSubstatement: 'Built for teams whose model layer has outgrown scattered provider keys.',
      metaLatency: 'Routing matrix online',
      metaModels: 'Claude · OpenAI · Gemini · Grok',
      metaControl: 'Usage · Billing · Failover',
      scrollCue: 'See the gateway',
      quickstartKicker: 'Gateway surface',
      titleLine1: 'One key.',
      titleLine2: 'Every model.',
      description:
        'Sub2API compresses Claude, OpenAI, Gemini, Grok, Qwen and more into one observable, billable, routable API gateway. Keep the official protocol experience while the gateway handles stability and cost control.',
      ctaPrimary: 'Get an API Key',
      ctaDocs: 'View Docs',
      baseUrlOpenai: 'OpenAI Compatible',
      baseUrlAnthropic: 'Anthropic Compatible',
      copy: 'Copy',
      copied: 'Copied',
      cards: {
        routing: { title: 'Smart Routing', desc: 'Multi-channel load balancing with automatic failover' },
        observability: { title: 'Real-time Observability', desc: 'Track usage and cost of every single call' },
        billing: { title: 'Pay As You Go', desc: 'Only pay for what you use, no monthly fees' }
      }
    },
    visual: {
      gatewayLabel: 'Routing Matrix',
      gatewayMeta: 'Live Gateway'
    },
    work: {
      kicker: 'Selected Work',
      index: '01 / 04'
    },
    value: {
      kicker: 'VALUE',
      title: 'Why Sub2API',
      subtitle: 'Professional, reliable, developer-friendly',
      items: {
        unified: {
          title: 'Unified Access',
          desc: 'One key for all models. Fully compatible with OpenAI Responses / Chat and Anthropic Messages protocols. Low-friction migration.'
        },
        observability: {
          title: 'Full Observability',
          desc: 'Real-time stats on requests, tokens and spend, with per-model and per-key breakdowns. Know exactly where every cent goes.'
        },
        elastic: {
          title: 'Elastic & Cost-efficient',
          desc: 'Smart multi-upstream routing with automatic failover. Full SSE streaming support. Fast, stable, usage-based billing.'
        },
        developer: {
          title: 'Built for Developers',
          desc: 'One-line setup scripts for Claude Code, Codex, Gemini CLI and more. Official SDKs work out of the box.'
        }
      }
    },
    workflow: {
      kicker: 'PROCESS',
      title: 'Migrate in Three Steps',
      subtitle: 'From sign-up to your first request in minutes.',
      steps: {
        register: {
          title: 'Register & Get a Key',
          desc: 'Sign up for free and create your API Key in the console.'
        },
        configure: {
          title: 'Point the Base URL',
          desc: 'Set the Base URL to Sub2API: /v1 for OpenAI, root path for Claude. Everything else stays the same.'
        },
        observe: {
          title: 'Call & Observe',
          desc: 'Use official SDKs or CLI tools directly, then watch usage, spend and channel health in real time.'
        }
      }
    },
    ecosystem: {
      kicker: 'SERVICES',
      title: 'Major models and tools, connected through one entrance',
      subtitle: 'Major models onboard, one endpoint for everything',
      more: 'More coming soon'
    },
    pricing: {
      kicker: 'PRICING',
      title: 'Limited-time Top-up Offer',
      subtitle: 'Transparent usage-based billing, credits never expire.',
      rateLabel: 'Top-up Rate',
      rateValue: '¥1 = $1',
      officialLabel: 'Market Reference',
      officialValue: '$1 ≈ ¥7.2',
      badge: 'Limited Time',
      note: 'Billed at official model prices. Top up now to lock in the rate.',
      cta: 'Top Up Now'
    },
    cta: {
      kicker: 'CONTACT',
      title: 'Give your AI traffic a stable entrance.',
      description: 'Sign up for free and send your first observable, billable and routable AI request within minutes.',
      button: 'Start with Sub2API'
    },
    about: {
      open: 'About Sub2API',
      close: 'Close',
      eyebrow: 'About the gateway',
      title: 'A trusted entrance for high-frequency AI calls.',
      body:
        'Sub2API is not another decorative model list. It brings account pools, keys, billing, failover and observability into one control plane. You keep the official protocol workflow; the platform makes every request routed, recorded and cost-bounded.',
      est: 'EST 2024',
      based: 'Built for developers and operators',
      principles: {
        outcomes: {
          title: 'Outcomes first',
          desc: 'Every capability points back to stable calls, low migration cost and explainable spend.'
        },
        signal: {
          title: 'Clear signal',
          desc: 'Usage, models, costs, errors and channel health should be legible at a glance.'
        },
        human: {
          title: 'Developer-first',
          desc: 'Keep the official SDK and CLI habits intact so adoption does not interrupt the workflow.'
        },
        pace: {
          title: 'Long-term stability',
          desc: 'Multiple accounts, channels and throttling policies reduce single-point risk.'
        }
      }
    },
    footer: {
      allRightsReserved: 'All rights reserved.',
      console: 'Console',
      apiExamples: 'API Examples'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key Usage',
    subtitle: 'Enter your API Key to view real-time spending and usage status',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Query',
    querying: 'Querying...',
    privacyNote: 'Your Key is processed locally in the browser and will not be stored',
    dateRange: 'Date Range:',
    dateRangeToday: 'Today',
    dateRange7d: '7 Days',
    dateRange30d: '30 Days',
    dateRange90d: '90 Days',
    dateRangeCustom: 'Custom',
    apply: 'Apply',
    used: 'Used',
    detailInfo: 'Detail Information',
    tokenStats: 'Token Statistics',
    dailyDetail: 'Daily Detail',
    modelStats: 'Model Usage Statistics',
    // Table headers
    date: 'Date',
    model: 'Model',
    requests: 'Requests',
    inputTokens: 'Input Tokens',
    outputTokens: 'Output Tokens',
    cacheCreationTokens: 'Cache Creation',
    cacheReadTokens: 'Cache Read',
    cacheWriteTokens: 'Cache Write',
    totalTokens: 'Total Tokens',
    cost: 'Cost',
    // Status
    quotaMode: 'Key Quota Mode',
    walletBalance: 'Wallet Balance',
    // Ring card titles
    totalQuota: 'Total Quota',
    limit5h: '5-Hour Limit',
    limitDaily: 'Daily Limit',
    limit7d: '7-Day Limit',
    limitWeekly: 'Weekly Limit',
    limitMonthly: 'Monthly Limit',
    // Detail rows
    remainingQuota: 'Remaining Quota',
    expiresAt: 'Expires At',
    todayExpires: '(expires today)',
    daysLeft: '({days} days)',
    usedQuota: 'Used Quota',
    resetNow: 'Resetting soon',
    subscriptionType: 'Subscription Type',
    subscriptionExpires: 'Subscription Expires',
    // Usage stat cells
    todayRequests: 'Today Requests',
    todayInputTokens: 'Today Input',
    todayOutputTokens: 'Today Output',
    todayTokens: 'Today Tokens',
    todayCacheCreation: 'Today Cache Creation',
    todayCacheRead: 'Today Cache Read',
    todayCost: 'Today Cost',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Total Requests',
    totalInputTokens: 'Total Input',
    totalOutputTokens: 'Total Output',
    totalTokensLabel: 'Total Tokens',
    totalCacheCreation: 'Total Cache Creation',
    totalCacheRead: 'Total Cache Read',
    totalCost: 'Total Cost',
    avgDuration: 'Avg Duration',
    // Messages
    enterApiKey: 'Please enter an API Key',
    querySuccess: 'Query successful',
    queryFailed: 'Query failed',
    queryFailedRetry: 'Query failed, please try again later',
    noDailyUsage: 'No daily usage data',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API Setup',
    description: 'Configure your Sub2API instance',
    database: {
      title: 'Database Configuration',
      description: 'Connect to your PostgreSQL database',
      host: 'Host',
      port: 'Port',
      username: 'Username',
      password: 'Password',
      databaseName: 'Database Name',
      sslMode: 'SSL Mode',
      passwordPlaceholder: 'Password',
      ssl: {
        disable: 'Disable',
        require: 'Require',
        verifyCa: 'Verify CA',
        verifyFull: 'Verify Full'
      }
    },
    redis: {
      title: 'Redis Configuration',
      description: 'Connect to your Redis server',
      host: 'Host',
      port: 'Port',
      password: 'Password (optional)',
      database: 'Database',
      passwordPlaceholder: 'Password',
      enableTls: 'Enable TLS',
      enableTlsHint: 'Use TLS when connecting to Redis (public CA certs)'
    },
    admin: {
      title: 'Admin Account',
      description: 'Create your administrator account',
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm Password',
      passwordPlaceholder: 'Min 8 characters',
      confirmPasswordPlaceholder: 'Confirm password',
      passwordMismatch: 'Passwords do not match'
    },
    ready: {
      title: 'Ready to Install',
      description: 'Review your configuration and complete setup',
      database: 'Database',
      redis: 'Redis',
      adminEmail: 'Admin Email'
    },
    status: {
      testing: 'Testing...',
      success: 'Connection Successful',
      testConnection: 'Test Connection',
      installing: 'Installing...',
      completeInstallation: 'Complete Installation',
      completed: 'Installation completed!',
      redirecting: 'Redirecting to login page...',
      restarting: 'Service is restarting, please wait...',
      timeout: 'Service restart is taking longer than expected. Please refresh the page manually.'
    }
  },

  // Common
}
