package service

import (
	"encoding/json"
	"strings"
)

const (
	ContentModerationActionCyberAbuseBlock   = "cyber_abuse_block"
	ContentModerationErrorCodeCyberAbuse     = "cyber_abuse_blocked"
	ContentModerationPolicySourceCyberAbuse  = "local_cyber_guard"
	ContentModerationPolicySourceCyberPolicy = "upstream_cyber_policy"
	defaultCyberAbuseBlockMessage            = "该请求涉嫌恶意网络活动或违反上游使用政策，已终止执行。请仅提交合法、已授权且防御性的安全需求；重复触发可能导致 API Key 或账号暂停。"
	cyberAbuseAuditExcerpt                   = "[redacted] high-confidence cyber abuse pattern matched"
)

type CyberAbuseGuardConfig struct {
	Enabled                    bool   `json:"enabled"`
	PreflightEnabled           bool   `json:"preflight_enabled"`
	ProviderFeedbackEnabled    bool   `json:"provider_feedback_enabled"`
	CountTowardsAutoBan        bool   `json:"count_towards_auto_ban"`
	BlockMessage               string `json:"block_message"`
	preflightConfigured        bool
	providerFeedbackConfigured bool
}

func (cfg *CyberAbuseGuardConfig) UnmarshalJSON(data []byte) error {
	type rawCyberAbuseGuardConfig struct {
		Enabled                 bool   `json:"enabled"`
		PreflightEnabled        *bool  `json:"preflight_enabled"`
		ProviderFeedbackEnabled *bool  `json:"provider_feedback_enabled"`
		CountTowardsAutoBan     bool   `json:"count_towards_auto_ban"`
		BlockMessage            string `json:"block_message"`
	}
	var raw rawCyberAbuseGuardConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	cfg.Enabled = raw.Enabled
	cfg.CountTowardsAutoBan = raw.CountTowardsAutoBan
	cfg.BlockMessage = raw.BlockMessage
	cfg.preflightConfigured = raw.PreflightEnabled != nil
	if raw.PreflightEnabled != nil {
		cfg.PreflightEnabled = *raw.PreflightEnabled
	}
	cfg.providerFeedbackConfigured = raw.ProviderFeedbackEnabled != nil
	if raw.ProviderFeedbackEnabled != nil {
		cfg.ProviderFeedbackEnabled = *raw.ProviderFeedbackEnabled
	}
	return nil
}

func (cfg *CyberAbuseGuardConfig) normalize() {
	if cfg == nil {
		return
	}
	if cfg.Enabled && !cfg.preflightConfigured {
		cfg.PreflightEnabled = true
	}
	if !cfg.providerFeedbackConfigured {
		cfg.ProviderFeedbackEnabled = true
	}
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = defaultCyberAbuseBlockMessage
	}
	cfg.BlockMessage = strings.TrimSpace(cfg.BlockMessage)
}

type UpdateCyberAbuseGuardConfigInput struct {
	Enabled                 *bool   `json:"enabled"`
	PreflightEnabled        *bool   `json:"preflight_enabled"`
	ProviderFeedbackEnabled *bool   `json:"provider_feedback_enabled"`
	CountTowardsAutoBan     *bool   `json:"count_towards_auto_ban"`
	BlockMessage            *string `json:"block_message"`
}

type TestCyberAbuseInput struct {
	Text string `json:"text"`
}

type TestCyberAbuseResult struct {
	Matched    bool    `json:"matched"`
	Category   string  `json:"category"`
	RuleID     string  `json:"rule_id"`
	Confidence float64 `json:"confidence"`
	ErrorCode  string  `json:"error_code"`
	Message    string  `json:"message"`
}

type CyberAbuseDetection struct {
	Matched    bool    `json:"matched"`
	Category   string  `json:"category"`
	RuleID     string  `json:"rule_id"`
	Confidence float64 `json:"confidence"`
}

type cyberAbuseRule struct {
	id                string
	category          string
	confidence        float64
	strongPhrases     []string
	intentTerms       []string
	capabilityTerms   []string
	ignoreSafeContext bool
}

var cyberAbuseRules = []cyberAbuseRule{
	{
		id: "cyber_abuse.credential_theft.explicit_payload", category: "credential_theft", confidence: 0.99,
		strongPhrases:     []string{"steal credentials", "steal passwords", "harvest credentials", "harvest passwords", "dump browser cookies", "exfiltrate browser cookies", "steal session tokens", "窃取凭据", "盗取凭据", "窃取密码", "盗取密码", "导出浏览器cookie", "窃取会话令牌"},
		intentTerms:       []string{"steal", "harvest", "dump", "exfiltrate", "窃取", "盗取", "收集并外传", "导出并发送"},
		capabilityTerms:   []string{"credential", "password", "browser cookie", "session token", "api key", "凭据", "密码", "浏览器cookie", "会话令牌", "密钥"},
		ignoreSafeContext: true,
	},
	{
		id: "cyber_abuse.malware.build_or_deploy", category: "malware", confidence: 0.99,
		strongPhrases:     []string{"write ransomware", "build ransomware", "create malware", "deploy malware", "write a keylogger", "build a keylogger", "create a trojan", "编写勒索软件", "制作勒索软件", "编写恶意软件", "部署恶意软件", "编写键盘记录器", "制作木马"},
		intentTerms:       []string{"write", "build", "create", "deploy", "install", "编写", "制作", "创建", "部署", "植入"},
		capabilityTerms:   []string{"malware", "ransomware", "keylogger", "trojan", "remote access trojan", "恶意软件", "勒索软件", "键盘记录器", "木马", "远控"},
		ignoreSafeContext: true,
	},
	{
		id: "cyber_abuse.unauthorized_access.break_in", category: "unauthorized_access", confidence: 0.98,
		strongPhrases:   []string{"break into their account", "break into the server", "gain unauthorized access", "access without permission", "bypass authentication and access", "入侵他人账户", "入侵服务器", "未经授权访问", "绕过认证并访问", "破解账号并登录"},
		intentTerms:     []string{"break into", "hack into", "bypass authentication", "take over", "入侵", "破解", "绕过认证", "接管"},
		capabilityTerms: []string{"account", "server", "database", "admin panel", "邮箱", "账户", "服务器", "数据库", "后台"},
	},
	{
		id: "cyber_abuse.evasion_exfiltration.covert_theft", category: "evasion_exfiltration", confidence: 0.99,
		strongPhrases:     []string{"exfiltrate data without detection", "exfiltrate data undetected", "bypass edr and exfiltrate", "evade detection while stealing", "disable antivirus and exfiltrate", "规避检测并外传", "绕过edr并外传", "关闭杀毒并窃取", "隐蔽窃取数据", "不被发现地外传"},
		intentTerms:       []string{"exfiltrate", "steal data", "外传", "窃取数据", "导出数据"},
		capabilityTerms:   []string{"without detection", "undetected", "bypass edr", "evade detection", "disable antivirus", "规避检测", "绕过edr", "关闭杀毒", "不被发现", "隐蔽"},
		ignoreSafeContext: true,
	},
	{
		id: "cyber_abuse.botnet_disruption.command_or_attack", category: "botnet_disruption", confidence: 0.98,
		strongPhrases:   []string{"build a botnet", "create a botnet", "control a botnet", "launch a ddos attack", "take down their service", "disrupt their network", "组建僵尸网络", "创建僵尸网络", "控制僵尸网络", "发动ddos攻击", "打垮他们的服务", "瘫痪目标网络"},
		intentTerms:     []string{"build", "create", "control", "launch", "take down", "disrupt", "组建", "创建", "控制", "发动", "打垮", "瘫痪"},
		capabilityTerms: []string{"botnet", "ddos", "denial of service", "僵尸网络", "ddos", "拒绝服务", "目标网络", "服务"},
	},
}

var cyberAbuseSafeContextTerms = []string{
	"authorized security test", "authorized penetration test", "authorized pentest", "permission to test",
	"defensive research", "defensive training", "security awareness", "incident response", "malware analysis",
	"how to prevent", "how can i prevent", "how can we prevent", "how do i prevent", "how do we prevent",
	"protect against", "defend against", "detection guidance", "prevention guidance", "mitigation guidance",
	"best practices for preventing", "ctf", "capture the flag", "lab environment", "sandbox",
	"合法授权", "授权测试", "渗透测试授权", "如何防止", "如何预防", "如何检测", "如何防御",
	"防护建议", "检测建议", "缓解建议", "防御研究", "安全培训", "应急响应", "恶意软件分析", "靶场", "竞赛题", "ctf比赛",
}

func DetectCyberAbuse(text string) CyberAbuseDetection {
	normalized := normalizeCyberAbuseText(text)
	if normalized == "" {
		return CyberAbuseDetection{}
	}
	safeContext := containsAnyCyberAbuseTerm(normalized, cyberAbuseSafeContextTerms)
	for _, rule := range cyberAbuseRules {
		if containsAnyCyberAbuseTerm(normalized, rule.strongPhrases) {
			if safeContext && !rule.ignoreSafeContext {
				continue
			}
			return CyberAbuseDetection{Matched: true, Category: rule.category, RuleID: rule.id, Confidence: rule.confidence}
		}
		if containsAnyCyberAbuseTerm(normalized, rule.intentTerms) && containsAnyCyberAbuseTerm(normalized, rule.capabilityTerms) {
			if safeContext {
				continue
			}
			return CyberAbuseDetection{Matched: true, Category: rule.category, RuleID: rule.id, Confidence: rule.confidence - 0.02}
		}
	}
	return CyberAbuseDetection{}
}

func normalizeCyberAbuseText(text string) string {
	text = strings.ToLower(normalizeContentModerationText(text))
	replacer := strings.NewReplacer("，", " ", "。", " ", "；", " ", "：", " ", "、", " ", "\t", " ", "\n", " ")
	return strings.Join(strings.Fields(replacer.Replace(text)), " ")
}

func containsAnyCyberAbuseTerm(text string, terms []string) bool {
	for _, term := range terms {
		if term = strings.TrimSpace(strings.ToLower(term)); term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}
