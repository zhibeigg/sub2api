export default {
  qqbot: {
    title: 'QQBot 管理',
    description: '统一管理腾讯 QQ 机器人运行状态、凭据、消息策略、账户绑定与诊断信息。',
    tabs: { overview: '概览', config: '机器人配置', messages: '消息与欢迎', bindings: '绑定记录', diagnostics: '诊断' },
    status: { enabled: '已启用', disabled: '已停用' },
    runtime: { disabled: '已停用', starting: '启动中', running: '运行中', reloading: '重载中', degraded: '降级', unknown: '未知' },
    secrets: { configured: '已配置', missing: '未配置', keepPlaceholder: '留空保留现有密钥', keepHint: '服务器只返回配置状态；留空不会清除或覆盖现有密钥。' },
    overview: {
      title: '运行概览', description: '查看机器人活动、队列、配置版本和账户绑定趋势。', desiredState: '期望状态', runtimeState: '实际状态', workers: 'Worker', completionRate: '绑定完成率', configVersion: '配置版本 v{version}', activeVersion: '活动版本 v{version}', pendingJobs: '{count} 个 pending', todayRequests: '今日 {count} 次请求', queueTitle: '事件队列', backlog: '积压', pending: '待确认', deadLetters: '死信', queueCapacity: '队列容量', activityTitle: '最近活动', lastWebhook: '最近 Webhook', lastEvent: '最近事件', lastSend: '最近发送', lastError: '最近错误', bindingTitle: '绑定统计', totalRequests: '累计 {count} 次请求'
    },
    config: {
      title: '机器人配置', description: '凭据仅在提交新值时替换，读取页面不会暴露明文或密文。', appId: 'AppID', appSecret: 'AppSecret', webhookSecret: 'Webhook Secret', sandbox: 'Sandbox 环境', sandboxHint: '使用腾讯机器人沙箱接口与测试范围。', publicBaseUrl: '公共基础 URL', publicBaseUrlHint: '用于生成 Webhook、域名校验与邮件绑定链接。', runtimeTitle: '运行参数', workerCount: 'Worker 数量', queueCapacity: '队列容量', apiTimeout: 'API 超时（毫秒）', probeTitle: '腾讯连接测试', probeHint: '启用新配置或轮换凭据前必须通过连接测试。'
    },
    messages: {
      title: '消息与欢迎', description: '配置帮助回复、账户绑定、首绑奖励以及群和频道的欢迎范围。', bindingEnabled: '允许账户绑定', bindingEnabledHint: '关闭后，待处理链接无法完成绑定。', welcomeEnabled: '新成员欢迎', welcomeEnabledHint: '在支持的频道成员加入事件中发送欢迎。', firstInteraction: '首次互动欢迎', firstInteractionHint: '用户首次提及或私聊机器人时发送欢迎。', firstBindBonus: '首次绑定奖励', linkTtl: '绑定链接有效期（分钟）', helpMessage: '帮助文案', helpMessageHint: '最多 4000 字符，当前 {count} 字符。', allowlistTitle: '群与频道范围', allowlistHint: '留空表示不限制；每项按腾讯提供的原始 ID 精确匹配。', allowedGroups: '允许的群 ID', allowedGuilds: '允许的频道 ID', onePerLine: '每行一个 ID', welcomeChannels: '频道欢迎子频道映射', welcomeChannelsHint: '每行使用“频道 ID = 子频道 ID”。'
    },
    bindings: {
      title: '绑定记录', description: '共 {count} 条记录；仅显示脱敏邮箱和 OpenID 指纹。', filterStatus: '状态', filterScene: '场景', filterSearch: '搜索脱敏邮箱、QQ 或指纹', filterFrom: '起始日期', filterTo: '结束日期', empty: '没有符合筛选条件的绑定记录。', status: '状态', account: '账户', scene: '场景', qq: '声明 QQ', bonus: '奖励', createdAt: '创建时间', completedAt: '完成时间', completed: '已完成', pending: '待处理', expired: '已过期', failed: '失败', revoked: '已解绑', unbind: '解绑', pageInfo: '第 {page} / {pages} 页', detailTitle: '绑定详情', source: '来源 ID', channel: '子频道 ID', failureCode: '失败代码', delivery: '邮件 / 通知状态', unbindTitle: '解除 QQBot 绑定', unbindWarning: '解绑只移除身份关联，已发放的首绑奖励不会撤回。', unbindReason: '解绑原因', unbindReasonError: '请输入 3 至 300 个字符的原因。', unbindConfirm: '确认解绑'
    },
    diagnostics: {
      title: '诊断', description: '核对腾讯平台入口、配置代际、连接测试和最近稳定错误。', webhookUrl: 'Webhook URL', validationUrl: '域名校验 URL', configVersion: '保存版本', desiredVersion: '期望版本', activeVersion: '活动版本', runtimeState: '运行状态', probeResult: '最近连接测试', probeOk: '连接测试通过', probeFailed: '连接测试失败', noProbe: '本次页面会话尚未执行连接测试。', lastError: '最近稳定错误', errorCode: '错误代码', errorTime: '发生时间', errorMessage: '错误信息'
    },
    actions: { enabled: '启用 QQBot', unsaved: '有未保存更改', synced: '已与服务器同步', probe: '测试连接', probing: '测试中…', unbinding: '解绑中…' },
    notices: { saved: 'QQBot 配置已保存', probeSucceeded: '腾讯连接测试通过', unbound: '绑定已解除' },
    bindingStatus: { pending: '待处理', completed: '已完成', expired: '已过期', revoked: '已解绑', failed: '失败' },
    validation: { appId: '请输入 1 至 64 位数字 AppID。', publicUrl: '请输入有效的 HTTP(S) 公共 URL。', workers: 'Worker 数量必须是 1 至 64 的整数。', queue: '队列容量必须是 100 至 100,000 的整数。', timeout: 'API 超时必须是 500 至 30,000 毫秒。', bonus: '首次绑定奖励不能为负数。', ttl: '链接有效期必须是 5 至 1440 分钟的整数。', help: '帮助文案不能超过 4000 字符。', mapping: '频道欢迎映射格式无效，请使用“频道 ID = 子频道 ID”。' },
    errors: { loadConfig: '加载 QQBot 配置失败。', loadRuntime: '加载 QQBot 运行状态失败。', loadStats: '加载 QQBot 统计失败。', loadBindings: '加载绑定记录失败。', saveConfig: '保存 QQBot 配置失败。', probe: '连接测试失败。', unbind: '解绑失败。', credentialsRequired: '启用或测试前必须填写 AppID，并配置 AppSecret 与 Webhook Secret。', probeRequired: '启用新配置或更改凭据后，必须先通过连接测试。', QQBOT_PROBE_REQUIRED: '当前凭据尚未通过连接测试，请先执行探测。', QQBOT_CONFIG_CONFLICT: '配置已被其他管理员更新，请刷新后重试。' }
  }
}
