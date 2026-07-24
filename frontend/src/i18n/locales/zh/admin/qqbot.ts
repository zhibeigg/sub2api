export default {
  qqbot: {
    title: 'QQBot 管理',
    description: '统一管理腾讯 QQ 机器人运行状态、凭据、消息策略、账户绑定与诊断信息。',
    tabs: { overview: '概览', transport: '接入方式', config: '接入配置', messages: '消息与欢迎', bindings: '绑定记录', diagnostics: '诊断' },
    status: { enabled: '已启用', disabled: '已停用' },
    runtime: { disabled: '已停用', starting: '启动中', running: '运行中', reloading: '重载中', degraded: '降级', unknown: '未知' },
    secrets: { configured: '已配置', missing: '未配置', keepPlaceholder: '留空保留现有密钥', keepHint: '服务器只返回配置状态；留空不会清除或覆盖现有密钥。' },
    overview: {
      title: '运行概览', description: '查看机器人活动、队列、配置版本和账户绑定趋势。', transportMode: '当前接入方式', desiredState: '期望状态', runtimeState: '实际状态', workers: 'Worker', completionRate: '绑定完成率', configVersion: '配置版本 v{version}', activeVersion: '活动版本 v{version}', pendingJobs: '{count} 个 pending', todayRequests: '今日 {count} 次请求', queueTitle: '事件队列', backlog: '积压', pending: '待确认', deadLetters: '死信', queueCapacity: '队列容量', activityTitle: '最近活动', lastWebhook: '最近 Webhook', lastEvent: '最近事件', lastSend: '最近发送', lastError: '最近错误', bindingTitle: '绑定统计', totalRequests: '累计 {count} 次请求'
    },
    transport: {
      title: '接入方式', description: '选择 QQBot 使用的唯一消息接入方式。切换后仅显示该方式的接入配置。', selected: '当前选择', switching: '正在切换接入方式…', inherited: '当前选择继承自升级前的默认配置。请确认并保存需要使用的接入方式。', modes: { botgo: '腾讯官方 BotGo', onebot: 'SnowLuma OneBot' }, botgoDescription: '通过腾讯官方机器人平台接收频道与群事件，并使用 AppID、AppSecret 和 Webhook Secret 配置。', onebotDescription: '通过 SnowLuma 的本机反向 WebSocket 接收 OneBot v11 事件，并使用 Self ID 与 Access Token 配置。'
    },
    config: {
      title: '机器人配置', description: '凭据仅在提交新值时替换，读取页面不会暴露明文或密文。', appId: 'AppID', appSecret: 'AppSecret', webhookSecret: 'Webhook Secret', sandbox: 'Sandbox 环境', sandboxHint: '使用腾讯机器人沙箱接口与测试范围。', publicBaseUrl: '公共基础 URL', publicBaseUrlHint: '用于生成 Webhook、域名校验、邮件绑定与 /check 图片链接；/check 必须使用腾讯可访问的 HTTPS 地址。', runtimeTitle: '运行参数', workerCount: 'Worker 数量', queueCapacity: '队列容量', apiTimeout: 'API 超时（毫秒）', probeTitle: '腾讯连接测试', probeHint: '启用新配置或轮换凭据前必须通过连接测试。'
    },
    onebot: {
      title: 'SnowLuma / OneBot v11', description: '通过本机反向 WebSocket 接入普通 QQ 群消息与进群事件；当前接入方式为 OneBot 时才会运行。', connected: '反向 WS 已连接', disconnected: '反向 WS 未连接', selfId: '机器人 QQ / Self ID', selfIdHint: '必须与 SnowLuma 当前登录账号一致，仅保存数字 ID。', accessToken: 'Bearer Access Token', tokenHint: '至少 32 个字符；加密保存且 API 只返回是否已配置。', reverseWsUrl: 'SnowLuma 反向 WebSocket URL', reverseWsHint: '请在 SnowLuma 中使用该本机地址，配置相同 Token，并保持 3010/3011 仅监听 loopback。', actionTimeout: 'Action 超时（毫秒）', enableRuntime: '启用 OneBot 事件处理', runtimeState: '运行状态', workers: 'Worker', pendingActions: '待响应 Action', lastConnection: '最近连接活动', requestApprovalTitle: '申请自动审批', requestApprovalHint: '默认关闭。仅处理 SnowLuma 投递的 OneBot 申请事件，不会自动接受机器人入群邀请。', autoApproveFriends: '自动通过好友申请', autoApproveFriendsHint: '开启后会批准所有发送给该机器人 QQ 的好友申请。', autoApproveGroups: '自动通过白名单群的加群申请', autoApproveGroupsHint: '仅批准目标群位于“允许的群 ID”白名单内的用户加群申请；不会处理邀请。'
    },
    messages: {
      title: '消息与欢迎', description: '配置帮助回复、账户绑定、首绑奖励以及群和频道的欢迎范围。白名单群可直接发送 /bind 邮箱；所有群内回复都会留在原群。', bindingEnabled: '允许账户绑定', bindingEnabledHint: '关闭后，待处理链接无法完成绑定；白名单群内可直接发送 /bind 加邮箱。', welcomeEnabled: '新成员欢迎', welcomeEnabledHint: '在支持的群或频道成员加入事件中发送欢迎。', friendOpening: '好友添加开场消息', friendOpeningHint: '用户实际添加机器人为好友后，OneBot 会在好友私聊中发送一次开场帮助。此规则不可关闭；普通私聊、群临时会话和群消息不会触发私聊。', channelCheckEnabled: '允许 /check 渠道状态图', channelCheckEnabledHint: '开启后，/check 可生成渠道状态图；同时要求公网 HTTPS 根域和服务端显式配置共享 TOTP 加密密钥。', firstBindBonus: '首次绑定奖励', linkTtl: '绑定链接有效期（分钟）', commandCooldown: '指令冷却（秒）', commandCooldownHint: '按用户、指令和机器人传输链路隔离；/help、/bind、/check 在冷却内会提示准确剩余时间。', welcomeMessage: '入群欢迎文案', welcomeMessageHint: "支持 {'{'}site{'}'}、{'{'}user{'}'}、{'{'}bind_command{'}'} 纯文本占位；关闭绑定或 /check 后，对应指令行不会发送。最多 4000 字符，当前 {count} 字符。", helpMessage: '帮助文案', helpMessageHint: '最多 4000 字符，当前 {count} 字符。', allowlistTitle: '群与频道范围', allowlistHint: '群或频道白名单为空时将拒绝对应来源（fail-closed），并非不限制；每项按腾讯提供的原始 ID 精确匹配。', allowedGroups: '允许的群 ID', allowedGuilds: '允许的频道 ID', onePerLine: '每行一个 ID', welcomeChannels: '频道欢迎子频道映射', welcomeChannelsHint: '每行使用“频道 ID = 子频道 ID”。'
    },
    bindings: {
      title: '绑定记录', description: '共 {count} 条记录；仅显示脱敏邮箱和 OpenID 指纹。', filterStatus: '状态', filterScene: '场景', filterSearch: '搜索脱敏邮箱、QQ 或指纹', filterFrom: '起始日期', filterTo: '结束日期', empty: '没有符合筛选条件的绑定记录。', status: '状态', account: '账户', scene: '场景', qq: '声明 QQ', bonus: '奖励', createdAt: '创建时间', completedAt: '完成时间', completed: '已完成', pending: '待处理', expired: '已过期', failed: '失败', revoked: '已解绑', unbind: '解绑', pageInfo: '第 {page} / {pages} 页', detailTitle: '绑定详情', source: '来源 ID', channel: '子频道 ID', failureCode: '失败代码', delivery: '邮件 / 通知状态', unbindTitle: '解除 QQBot 绑定', unbindWarning: '解绑只移除身份关联，已发放的首绑奖励不会撤回。', unbindReason: '解绑原因', unbindReasonError: '请输入 3 至 300 个字符的原因。', unbindConfirm: '确认解绑'
    },
    diagnostics: {
      title: '诊断', description: '核对腾讯平台入口、配置代际、连接测试和最近稳定错误。', webhookUrl: 'Webhook URL', validationUrl: '域名校验 URL', configVersion: '保存版本', desiredVersion: '期望版本', activeVersion: '活动版本', runtimeState: '运行状态', probeResult: '最近连接测试', probeOk: '连接测试通过', probeFailed: '连接测试失败', noProbe: '本次页面会话尚未执行连接测试。', lastError: '最近稳定错误', errorCode: '错误代码', errorTime: '发生时间', errorMessage: '错误信息'
    },
    actions: { enabled: '启用 QQBot', unsaved: '有未保存更改', synced: '已与服务器同步', probe: '测试连接', probing: '测试中…', unbinding: '解绑中…' },
    notices: { saved: 'QQBot 配置已保存', transportUpdated: 'QQBot 接入方式已更新', probeSucceeded: '腾讯连接测试通过', oneBotSaved: 'OneBot 配置已保存', oneBotProbeSucceeded: 'OneBot 反向 WebSocket 探测通过', unbound: '绑定已解除' },
    bindingStatus: { pending: '待处理', completed: '已完成', expired: '已过期', revoked: '已解绑', failed: '失败' },
    validation: { appId: '请输入 1 至 64 位数字 AppID。', oneBotSelfId: '请输入 5 至 20 位数字 Self ID。', oneBotToken: 'OneBot Token 至少需要 32 个字符。', publicUrl: '请输入有效的 HTTP(S) 公共 URL。', workers: 'Worker 数量必须是 1 至 64 的整数。', queue: '队列容量必须是 100 至 100,000 的整数。', oneBotQueue: 'OneBot 队列容量必须是 16 至 100,000 的整数。', timeout: 'API 超时必须是 500 至 30,000 毫秒。', bonus: '首次绑定奖励不能为负数。', ttl: '链接有效期必须是 5 至 1440 分钟的整数。', welcome: '入群欢迎文案不能超过 4000 字符。', help: '帮助文案不能超过 4000 字符。', mapping: '频道欢迎映射格式无效，请使用“频道 ID = 子频道 ID”。' },
    errors: { loadConfig: '加载 QQBot 配置失败。', updateTransport: '更新 QQBot 接入方式失败。', loadRuntime: '加载 QQBot 运行状态失败。', loadOneBotConfig: '加载 OneBot 配置失败。', loadOneBotRuntime: '加载 OneBot 运行状态失败。', loadStats: '加载 QQBot 统计失败。', loadBindings: '加载绑定记录失败。', saveConfig: '保存 QQBot 配置失败。', saveOneBotConfig: '保存 OneBot 配置失败。', probe: '连接测试失败。', oneBotProbe: 'OneBot 探测失败。', unbind: '解绑失败。', credentialsRequired: '启用或测试前必须填写 AppID，并配置 AppSecret 与 Webhook Secret。', probeRequired: '启用新配置或更改凭据后，必须先通过连接测试。', oneBotCredentialsRequired: '请填写正确的 Self ID，并配置至少 32 字符的 OneBot Token。', oneBotSaveBeforeProbe: '请先以停用状态保存 OneBot 配置，等待 SnowLuma 建立反向 WebSocket 后再探测。', oneBotProbeRequired: '启用 OneBot Runtime 前必须先通过当前配置的反向 WebSocket 探测。', QQBOT_PROBE_REQUIRED: '当前凭据尚未通过连接测试，请先执行探测。', QQBOT_CONFIG_CONFLICT: '配置已被其他管理员更新，请刷新后重试。', QQBOT_TRANSPORT_NOT_SELECTED: '请先在“接入方式”中选择对应的传输链路。' }
  }
}
