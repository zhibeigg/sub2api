export default {
  batchImageGuide: {
    title: 'Batch Image Generation',
    description: 'Submit multiple prompts in one job and download the generated images when complete'
  },
  seo: {
    home: {
      title: 'Claude, OpenAI & Gemini AI API Gateway',
      description: 'Poke API is an AI API gateway for developers and teams. Use one API key for Claude, OpenAI, Gemini, and other leading models with transparent billing, stable routing, and privacy-first data handling.'
    }
  },
  // Home Page
  home: {
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
      footerNav: 'Footer quick navigation',
      skipToContent: 'Skip to main content',
      endpoints: 'API endpoints'
    },
    nav: {
      features: 'Principles',
      workflow: 'Access',
      models: 'Models',
      pricing: 'Pricing'
    },
    story: {
      status: 'POKE ROUTING / LIVE',
      scroll: 'Scroll to sequence',
      previous: 'Previous scene',
      next: 'Next scene',
      orbitLabel: 'Poke API protocol and model routing orbit',
      orbitStatus: 'Route online',
      scenes: {
        real: {
          label: 'Genuine models',
          titleLine1: 'Full models.',
          titleLine2: 'No dilution.',
          subtitle: 'Models, protocols, and prices stay in plain sight.',
          description: 'Provider-compatible protocols and model capabilities remain intact, while multi-channel routing reduces single-point instability.'
        },
        protocol: {
          label: 'Unified access',
          titleLine1: 'One key.',
          titleLine2: 'Many protocols.',
          subtitle: 'Switch the Base URL, not your development habits.',
          description: 'Keep OpenAI- and Anthropic-compatible SDKs and CLIs while reaching leading models from one account.'
        },
        billing: {
          label: 'Traceable billing',
          titleLine1: 'Every call.',
          titleLine2: 'Every detail.',
          subtitle: 'Review price, tokens, status, and cost request by request.',
          description: 'Check model pricing before a call and itemized usage after it, then pay only for what was actually used.'
        }
      }
    },
    hero: {
      badge: 'Stable · Genuine · Fair',
      posterStatement: 'Full models.\nNo dilution.',
      posterSubstatement: 'One gateway to leading AI models, with clear pricing and traceable usage.',
      metaModels: 'Claude · OpenAI · Gemini · Grok',
      metaControl: 'Clear billing · Traceable usage',
      description:
        'PokeAPI is an AI aggregation gateway for developers. It preserves mainstream provider protocols and model capabilities, uses multi-channel routing to reduce single-point failures, and bills by actual usage.',
      commitment: 'Long-term commitment: PokeAPI is here to stay.',
      ctaPrimary: 'Get started',
      ctaDocs: 'Read the docs',
      baseUrlOpenai: 'OpenAI Compatible',
      baseUrlAnthropic: 'Anthropic Compatible',
      websiteNode: 'Website Node',
      copy: 'Copy',
      copied: 'Copied'
    },
    value: {
      kicker: 'PRINCIPLES',
      title: 'Stable. Genuine.\nFair.',
      subtitle: 'No inflated promises. Just clear models, pricing, and usage.',
      items: {
        unified: {
          title: 'Genuine Models',
          desc: 'Model names, protocols, and billing rules stay explicit, without vague bundles hiding what you call.'
        },
        observability: {
          title: 'Clear Billing',
          desc: 'Usage-based billing with visible prices, usage, and cost boundaries.'
        },
        elastic: {
          title: 'Stable Routing',
          desc: 'Multi-channel routing and failover reduce the impact of upstream instability.'
        },
        developer: {
          title: 'Simple Integration',
          desc: 'Works with common SDKs and CLIs; switch the Base URL and key to connect.'
        }
      }
    },
    workflow: {
      kicker: 'SETUP',
      title: 'Three steps.\nSame workflow.',
      subtitle: 'Keep your existing SDKs and CLIs; replace the endpoint and key.',
      steps: {
        register: {
          title: 'Create an Account and Key',
          desc: 'Create an API key in the console after registration.'
        },
        configure: {
          title: 'Switch the Base URL',
          desc: 'Choose the OpenAI- or Anthropic-compatible endpoint in the docs, then replace the Base URL and key.'
        },
        observe: {
          title: 'Send Requests and Review Usage',
          desc: 'Review the model, tokens, cost, and status in the console after each call.'
        }
      }
    },
    ecosystem: {
      kicker: 'MODELS',
      title: 'Leading models. One gateway.',
      subtitle: 'Available models are listed in the console.',
      more: 'View the console model catalog'
    },
    pricing: {
      kicker: 'PRICING',
      title: 'Clear prices.\nPay for usage.',
      subtitle: 'Model prices, billing rules, and usage stay visible.',
      rateLabel: 'Billing Method',
      rateValue: 'Usage-based',
      officialLabel: 'Current Rates',
      officialValue: 'See console',
      badge: 'Clear Billing',
      note: 'Review rates before topping up and usage details after each call.',
      cta: 'Open Console Pricing'
    },
    docsPanel: {
      kicker: 'DOCUMENTATION',
      title: 'Documentation.\nNo hidden rules.',
      description: 'Review protocols, Base URLs, models, billing, and error handling.',
      button: 'Open Documentation',
      unavailable: 'Documentation URL is not configured'
    },
    privacy: {
      kicker: 'PRIVACY',
      title: 'Only necessary\ndata.',
      description:
        'We do not collect data for advertising, profiling, or resale. To provide accounts, routing, billing, and abuse prevention, we process only the data those functions require. The exact scope is stated in our privacy policy.',
      minimum: 'Only the data required for accounts, routing, billing, and security is processed.',
      noSale: 'We do not sell user data.',
      noTraining: 'PokeAPI does not use user data to train models.',
      noContent: 'Prompts, response bodies, and other API content are not retained.'
    },
    footer: {
      allRightsReserved: 'All rights reserved.',
      backToTop: 'Back to top'
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
    cacheCreationTokens: 'Cache Write',
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
      username: 'Username (optional)',
      password: 'Password (optional)',
      database: 'Database',
      usernamePlaceholder: 'Leave empty for default user',
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
