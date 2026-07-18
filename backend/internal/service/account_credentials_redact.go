package service

// SensitiveCredentialKeys 列出 Account.Credentials JSON map 中绝不允许返回到前端的子键。
// dto 层做响应脱敏、service 层做更新合并都引用此清单——新增凭证类型时务必同步。
var SensitiveCredentialKeys = []string{
	// OAuth
	"access_token", "refresh_token", "id_token", "agent_private_key", "dashboard_access_token", "dashboard_refresh_token",
	// API Key 类
	"api_key", "session_key", "cookie", "quota_cookie", "password", "device_token", "device_id",
	// 云服务凭据
	"aws_secret_access_key", "aws_session_token",
	"service_account_json", "service_account", "private_key",
}

var sensitiveCredentialKeySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(SensitiveCredentialKeys))
	for _, k := range SensitiveCredentialKeys {
		m[k] = struct{}{}
	}
	return m
}()

// credentialKeysPreservedOnUpdate 包含前端全对象 PUT 时必须保留的字段。
// 除敏感凭据外，Cursor Dashboard 授权流程写入的机器标识和 token 版本
// 也属于服务端管理状态；编辑弹窗持有的是授权前快照，不能因未携带这些字段而删除。
var credentialKeysPreservedOnUpdate = func() []string {
	keys := append([]string(nil), SensitiveCredentialKeys...)
	return append(keys, "cursor_machine_id", "_token_version")
}()

// IsSensitiveCredentialKey 判断指定键是否为敏感凭证子键。
func IsSensitiveCredentialKey(key string) bool {
	_, ok := sensitiveCredentialKeySet[key]
	return ok
}

// MergePreservingSensitiveCreds 把 incoming 写入 existing 之上，但敏感凭据和服务端管理字段采用
// "incoming 没提供就保留 existing"的语义。返回新的 map，不修改入参。
//
// 用途：前端编辑账号通常采用"全对象 PUT"模式；脱敏或授权前快照不会带上最新的服务端字段，
// 直接覆盖会清空已有 token、Cursor 机器标识或 token 版本。此函数保证：
//   - 普通非敏感键：完全由 incoming 决定（用户可以编辑、删除非敏感字段）。
//   - 受保护键：incoming 显式提供则覆盖，否则保留 existing。
func MergePreservingSensitiveCreds(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(incoming)+len(credentialKeysPreservedOnUpdate))
	for k, v := range incoming {
		out[k] = v
	}
	for _, key := range credentialKeysPreservedOnUpdate {
		if _, hasIncoming := incoming[key]; hasIncoming {
			continue
		}
		if existingVal, ok := existing[key]; ok {
			out[key] = existingVal
		}
	}
	return out
}
