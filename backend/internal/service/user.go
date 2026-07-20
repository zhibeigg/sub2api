package service

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID                int64
	Email             string
	Username          string
	Notes             string
	AvatarURL         string
	AvatarSource      string
	AvatarMIME        string
	AvatarByteSize    int
	AvatarSHA256      string
	PasswordHash      string
	Role              string
	Balance           float64
	FrozenBalance     float64
	Concurrency       int
	Status            string
	AllowedGroups     []int64
	GroupAccessMode   string
	GroupAccessGroups []int64
	TokenVersion      int64 // Incremented on password change to invalidate existing tokens
	// TokenVersionResolved indicates TokenVersion already contains the fingerprint-derived
	// value expected in JWT claims and refresh-token state.
	TokenVersionResolved bool
	SignupSource         string
	LastLoginAt          *time.Time
	LastActiveAt         *time.Time
	LastUsedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time // 非 nil 表示用户已软删除

	// GroupRates 用户专属分组倍率配置
	// map[groupID]rateMultiplier
	GroupRates map[int64]float64

	// TOTP 双因素认证字段
	TotpSecretEncrypted *string    // AES-256-GCM 加密的 TOTP 密钥
	TotpEnabled         bool       // 是否启用 TOTP
	TotpEnabledAt       *time.Time // TOTP 启用时间

	// 余额不足通知
	BalanceNotifyEnabled       bool
	BalanceNotifyThresholdType string // "fixed" (default) | "percentage"
	BalanceNotifyThreshold     *float64
	BalanceNotifyExtraEmails   []NotifyEmailEntry
	TotalRecharged             float64
	FirstRechargeBonusUsed     bool // 是否已完成首笔余额充值；首充优惠只能成功占用一次

	// RPMLimit 用户级每分钟请求数上限（0 = 不限制）。仅在所用分组未设置 rpm_limit
	// 且该 (用户, 分组) 无 rpm_override 时作为全局兜底生效，计数键 rpm:u:{userID}:{min}。
	RPMLimit int

	// PromoCodeID 注册时绑定的优惠码 ID（可空）。用于第三方支付首笔余额充值到账加成，
	// 以及按优惠链接筛选用量统计。nil 表示未通过优惠链接注册。
	PromoCodeID *int64

	// UserGroupRPMOverride 来自 auth cache snapshot 的 (user, group) RPM 覆盖值。
	// nil = 该 API Key 对应的 (user, group) 无 override；非 nil 时 checkRPM 直接使用，
	// 避免每请求查 DB。字段不持久化到数据库。
	UserGroupRPMOverride *int

	APIKeys       []APIKey
	Subscriptions []UserSubscription
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

const (
	GroupAccessModeInherit    = "inherit"
	GroupAccessModeRestricted = "restricted"
)

// AllowsStandardGroupByRestriction checks only the optional user-level
// standard-group allowlist. Base public/exclusive/subscription authorization is
// evaluated separately so the two policies can be composed without changing
// legacy grants.
func (u *User) AllowsStandardGroupByRestriction(groupID int64) bool {
	if u == nil || groupID <= 0 {
		return false
	}
	if u.GroupAccessMode != GroupAccessModeRestricted {
		return true
	}
	return containsGroupID(u.GroupAccessGroups, groupID)
}

// CanBindGroup checks whether a user can bind to a standard group.
// - Public groups still use the legacy implicit grant.
// - Exclusive groups still require the legacy AllowedGroups grant.
// - Restricted users additionally require the group in GroupAccessGroups.
func (u *User) CanBindGroup(groupID int64, isExclusive bool) bool {
	if !u.AllowsStandardGroupByRestriction(groupID) {
		return false
	}
	if !isExclusive {
		return true
	}
	return containsGroupID(u.AllowedGroups, groupID)
}

func containsGroupID(groupIDs []int64, groupID int64) bool {
	for _, id := range groupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}
