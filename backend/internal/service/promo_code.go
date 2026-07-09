package service

import (
	"math"
	"time"
)

// PromoCode 注册优惠码
type PromoCode struct {
	ID          int64
	Code        string
	BonusAmount float64
	// RechargeBonusMultiplier 通过该优惠码注册的用户充值时的到账加成倍率。
	// >1 表示加成（如 1.2 = 多到账 20%），=1 表示无加成，<=0 视为无效并回退为 1。
	RechargeBonusMultiplier float64
	MaxUses                 int
	UsedCount               int
	Status                  string
	ExpiresAt               *time.Time
	Notes                   string
	CreatedAt               time.Time
	UpdatedAt               time.Time

	// 关联
	UsageRecords []PromoCodeUsage
}

// PromoCodeUsage 优惠码使用记录
type PromoCodeUsage struct {
	ID          int64
	PromoCodeID int64
	UserID      int64
	BonusAmount float64
	UsedAt      time.Time

	// 关联
	PromoCode *PromoCode
	User      *User
}

// CanUse 检查优惠码是否可用
func (p *PromoCode) CanUse() bool {
	if p.Status != PromoCodeStatusActive {
		return false
	}
	if p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt) {
		return false
	}
	if p.MaxUses > 0 && p.UsedCount >= p.MaxUses {
		return false
	}
	return true
}

// IsExpired 检查是否已过期
func (p *PromoCode) IsExpired() bool {
	return p.ExpiresAt != nil && time.Now().After(*p.ExpiresAt)
}

// DefaultRechargeBonusMultiplier 默认充值到账加成倍率（1.0 = 无加成）
const DefaultRechargeBonusMultiplier = 1.0

// normalizeRechargeBonusMultiplier 归一化充值到账加成倍率。
// 语义为"到账加成"：充值到账余额 = 实付金额 × 全局倍率 × 优惠倍率。
// 非法值（NaN/Inf/<1）一律归一为 1.0（无加成），确保优惠只会让用户多得、绝不少得。
func normalizeRechargeBonusMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier < DefaultRechargeBonusMultiplier {
		return DefaultRechargeBonusMultiplier
	}
	return multiplier
}

// EffectiveRechargeMultiplier 返回该优惠码生效的充值加成倍率（已归一化）。
func (p *PromoCode) EffectiveRechargeMultiplier() float64 {
	if p == nil {
		return DefaultRechargeBonusMultiplier
	}
	return normalizeRechargeBonusMultiplier(p.RechargeBonusMultiplier)
}

// CreatePromoCodeInput 创建优惠码输入
type CreatePromoCodeInput struct {
	Code                    string
	BonusAmount             float64
	RechargeBonusMultiplier float64
	MaxUses                 int
	ExpiresAt               *time.Time
	Notes                   string
}

// UpdatePromoCodeInput 更新优惠码输入
type UpdatePromoCodeInput struct {
	Code                    *string
	BonusAmount             *float64
	RechargeBonusMultiplier *float64
	MaxUses                 *int
	Status                  *string
	ExpiresAt               *time.Time
	Notes                   *string
}
