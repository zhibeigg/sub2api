package domain

// Status constants
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
	StatusUnused   = "unused"
	StatusUsed     = "used"
	StatusExpired  = "expired"
)

// Role constants
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Platform constants
const (
	PlatformAnthropic   = "anthropic"
	PlatformOpenAI      = "openai"
	PlatformGemini      = "gemini"
	PlatformAntigravity = "antigravity"
	PlatformGrok        = "grok"
	PlatformAdobe       = "adobe"
	PlatformCursor      = "cursor"
	PlatformOpenCode    = "opencode"
	PlatformKiro        = "kiro" // AWS Kiro / CodeWhisperer (provides Claude models)
	PlatformComposite   = "composite"

	PlatformOpenCodeDisplayName = "OpenCode Go"
	DefaultOpenCodeBaseURL      = "https://opencode.ai/zen/go"
)

// Account type constants
const (
	AccountTypeOAuth          = "oauth"           // OAuth类型账号（full scope: profile + inference）
	AccountTypeSetupToken     = "setup-token"     // Setup Token类型账号（inference only scope）
	AccountTypeAPIKey         = "apikey"          // API Key类型账号
	AccountTypeUpstream       = "upstream"        // 上游透传类型账号（通过 Base URL + API Key 连接上游）
	AccountTypeBedrock        = "bedrock"         // AWS Bedrock 类型账号（通过 SigV4 签名或 API Key 连接 Bedrock，由 credentials.auth_mode 区分）
	AccountTypeServiceAccount = "service_account" // Google Service Account 类型账号（用于 Vertex AI）
)

// Redeem type constants
const (
	RedeemTypeBalance      = "balance"
	RedeemTypeConcurrency  = "concurrency"
	RedeemTypeSubscription = "subscription"
	RedeemTypeInvitation   = "invitation"
)

// PromoCode status constants
const (
	PromoCodeStatusActive   = "active"
	PromoCodeStatusDisabled = "disabled"
)

// Admin adjustment type constants
const (
	AdjustmentTypeAdminBalance     = "admin_balance"     // 管理员调整余额
	AdjustmentTypeAdminConcurrency = "admin_concurrency" // 管理员调整并发数
)

// Group subscription type constants
const (
	SubscriptionTypeStandard     = "standard"     // 标准计费模式（按余额扣费）
	SubscriptionTypeSubscription = "subscription" // 订阅模式（按限额控制）
)

// Subscription plan type constants.
const (
	SubscriptionPlanTypeSubscription             = "subscription"
	SubscriptionPlanTypeStandardQuota            = "standard_quota"
	SubscriptionPlanTypeLegacySharedSubscription = "legacy_shared_subscription"
)

// Subscription status constants
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusSuspended = "suspended"
)

// AntigravityGemini31ProAgentModel is the upstream route for Gemini 3.1 Pro High.
const AntigravityGemini31ProAgentModel = "gemini-pro-agent"

// DefaultAntigravityModelMapping 是 Antigravity 平台的默认模型映射
// 当账号未配置 model_mapping 时使用此默认值
// 与前端 useModelWhitelist.ts 中的 antigravityDefaultMappings 保持一致
var DefaultAntigravityModelMapping = map[string]string{
	// Claude 白名单
	"claude-fable-5":             "claude-fable-5",           // 官方模型
	"claude-opus-4-8":            "claude-opus-4-8",          // 官方模型
	"claude-opus-4-7":            "claude-opus-4-7",          // 官方模型
	"claude-opus-4-6-thinking":   "claude-opus-4-6-thinking", // 官方模型
	"claude-opus-4-6":            "claude-opus-4-6-thinking", // 简称映射
	"claude-opus-4-5-thinking":   "claude-opus-4-6-thinking", // 迁移旧模型
	"claude-sonnet-4-6":          "claude-sonnet-4-6",
	"claude-sonnet-4-5":          "claude-sonnet-4-5",
	"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
	// Claude 详细版本 ID 映射
	"claude-opus-4-5-20251101":   "claude-opus-4-6-thinking", // 迁移旧模型
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
	// Claude Haiku → Sonnet（无 Haiku 支持）
	"claude-haiku-4-5":          "claude-sonnet-4-6",
	"claude-haiku-4-5-20251001": "claude-sonnet-4-6",
	// Gemini 2.5 白名单
	"gemini-2.5-flash":               "gemini-2.5-flash",
	"gemini-2.5-flash-image":         "gemini-2.5-flash-image",
	"gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
	"gemini-2.5-flash-lite":          "gemini-2.5-flash-lite",
	"gemini-2.5-flash-thinking":      "gemini-2.5-flash-thinking",
	"gemini-2.5-pro":                 "gemini-2.5-pro",
	// Gemini 3 白名单
	"gemini-3-flash":    "gemini-3-flash",
	"gemini-3-pro-high": "gemini-3-pro-high",
	"gemini-3-pro-low":  "gemini-3-pro-low",
	// Gemini 3 preview 映射
	"gemini-3-flash-preview": "gemini-3-flash",
	"gemini-3-pro-preview":   "gemini-3-pro-high",
	// Gemini 3.1 白名单
	AntigravityGemini31ProAgentModel: AntigravityGemini31ProAgentModel,
	"gemini-3.1-pro":                 AntigravityGemini31ProAgentModel,
	"gemini-3.1-pro-high":            AntigravityGemini31ProAgentModel,
	"gemini-3.1-pro-low":             "gemini-3.1-pro-low",
	// Gemini 3.1 preview 映射
	"gemini-3.1-pro-preview": AntigravityGemini31ProAgentModel,
	// Gemini 3.1 image 白名单
	"gemini-3.1-flash-image": "gemini-3.1-flash-image",
	// Gemini 3.1 image preview 映射
	"gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
	// Gemini 3 image 兼容映射（向 3.1 image 迁移）
	"gemini-3-pro-image":         "gemini-3.1-flash-image",
	"gemini-3-pro-image-preview": "gemini-3.1-flash-image",
	// 其他官方模型
	"gpt-oss-120b-medium":    "gpt-oss-120b-medium",
	"tab_flash_lite_preview": "tab_flash_lite_preview",
}

// DefaultBedrockModelMapping 是 AWS Bedrock 平台的默认模型映射
// 将 Anthropic 标准模型名映射到 Bedrock 模型 ID
// 注意：此处的 "us." 前缀仅为默认值，ResolveBedrockModelID 会根据账号配置的
// aws_region 自动调整为匹配的区域前缀（如 eu.、apac.、jp. 等）
var DefaultBedrockModelMapping = map[string]string{
	// Claude Fable
	"claude-fable-5": "anthropic.claude-fable-5",
	// Claude Opus
	"claude-opus-4-8":          "us.anthropic.claude-opus-4-8-v1",
	"claude-opus-4-7":          "us.anthropic.claude-opus-4-7-v1",
	"claude-opus-4-6-thinking": "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-6":          "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-5-thinking": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-5-20251101": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-1":          "us.anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-opus-4-20250514":   "us.anthropic.claude-opus-4-20250514-v1:0",
	// Claude Sonnet
	"claude-sonnet-5":            "us.anthropic.claude-sonnet-5-v1",
	"claude-sonnet-4-6-thinking": "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-6":          "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-5":          "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-thinking": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-20250929": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-20250514":   "us.anthropic.claude-sonnet-4-20250514-v1:0",
	// Claude Haiku
	"claude-haiku-4-5":          "us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-haiku-4-5-20251001": "us.anthropic.claude-haiku-4-5-20251001-v1:0",
}

// DefaultAdobeModelMapping 是 Adobe Firefly 平台默认公开模型白名单。
// 隐藏别名由 provider/adobe/firefly catalog 解析，仅在显式账号映射时开放。
var DefaultAdobeModelMapping = map[string]string{
	"nano-banana-pro": "nano-banana-pro",
	"nano-banana-v2":  "nano-banana-v2",
	"nano-banana":     "nano-banana",
	"veo3":            "veo3",
	"veo3.1":          "veo3.1",
	"sora":            "sora",
	"sora-2-pro":      "sora-2-pro",
}

// CursorSupportedModels 是 Cursor 官方 /v1/models 返回的逻辑模型目录离线默认值。
// thinking、effort、context、fast 等执行参数必须由请求路由，不能作为独立白名单模型。
var CursorSupportedModels = []string{
	"claude-fable-5",
	"claude-haiku-4-5",
	"claude-opus-4-5",
	"claude-opus-4-6",
	"claude-opus-4-7",
	"claude-opus-4-8",
	"claude-sonnet-4",
	"claude-sonnet-4-5",
	"claude-sonnet-4-6",
	"claude-sonnet-5",
	"composer-2.5",
	"gemini-2.5-flash",
	"gemini-3-flash",
	"gemini-3.1-pro",
	"gemini-3.5-flash",
	"glm-5.2",
	"gpt-5-mini",
	"gpt-5.1",
	"gpt-5.1-codex-max",
	"gpt-5.1-codex-mini",
	"gpt-5.2",
	"gpt-5.2-codex",
	"gpt-5.3-codex",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.4-nano",
	"gpt-5.5",
	"gpt-5.6-luna",
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"grok-4.5",
	"kimi-k2.7-code",
}

// DefaultCursorModelMapping 保留 auto 兼容别名，同时默认开放显式模型身份映射。
var DefaultCursorModelMapping = func() map[string]string {
	mapping := map[string]string{
		"cursor-agent": "auto",
		"cursor-chat":  "auto",
	}
	for _, model := range CursorSupportedModels {
		mapping[model] = model
	}
	return mapping
}()

// DefaultKiroModelMapping 是 Kiro 平台的默认模型映射（模型白名单）。
// Kiro 上游（AWS CodeWhisperer）提供 Claude 系列模型，pkg/kiro 的 MapModel
// 会在转发时把客户端模型名归一化为 Kiro 模型 ID（如 claude-sonnet-4.5），
// 因此此处主要作为「该分组对外提供哪些模型」的白名单。值保持与 key 一致。
// 与前端 useModelWhitelist.ts 中的 kiroDefaultMappings 保持一致。
var DefaultKiroModelMapping = map[string]string{
	"claude-sonnet-4-5":          "claude-sonnet-4-5",
	"claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
	"claude-sonnet-4-6":          "claude-sonnet-4-6",
	"claude-sonnet-4-6-thinking": "claude-sonnet-4-6-thinking",
	"claude-opus-4-6":            "claude-opus-4-6",
	"claude-opus-4-6-thinking":   "claude-opus-4-6-thinking",
	"claude-opus-4-7":            "claude-opus-4-7",
	"claude-opus-4-8":            "claude-opus-4-8",
	"claude-haiku-4-5":           "claude-haiku-4-5",
	// 兼容旧/别名模型（由 pkg/kiro MapModel 进一步归一化）
	"claude-3-5-sonnet": "claude-sonnet-4-5",
	"claude-sonnet-4":   "claude-sonnet-4",
}
