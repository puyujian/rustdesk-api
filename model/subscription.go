package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
)

// 订单状态
const (
	OrderStatusPending  = 0 // 待支付
	OrderStatusPaid     = 1 // 已支付
	OrderStatusRefunded = 2 // 已退款
	OrderStatusClosed   = 3 // 已关闭
)

// 订阅状态
const (
	SubscriptionStatusActive   = 1 // 有效
	SubscriptionStatusExpired  = 2 // 已过期
	SubscriptionStatusCanceled = 3 // 已取消
)

// 周期单位
const (
	PeriodUnitDay   = "day"
	PeriodUnitMonth = "month"
	PeriodUnitYear  = "year"
)

// SubscriptionPlan 订阅套餐
type SubscriptionPlan struct {
	IdModel
	Code        string     `json:"code" gorm:"uniqueIndex;not null"`   // 套餐编码
	Name        string     `json:"name" gorm:"not null"`               // 套餐名称
	Description string     `json:"description" gorm:"type:text"`       // 描述
	Price       int64      `json:"price" gorm:"not null"`              // 价格(分)
	PeriodUnit  string     `json:"period_unit" gorm:"default:'month'"` // 周期单位: day/month/year
	PeriodCount int        `json:"period_count" gorm:"default:1"`      // 周期数量
	Status      StatusCode `json:"status" gorm:"default:1;index"`      // 状态: 1启用 2禁用
	SortOrder   int        `json:"sort_order" gorm:"default:0"`        // 排序
	TimeModel
}

type SubscriptionPlanList struct {
	Plans []*SubscriptionPlan `json:"list"`
	Pagination
}

// Order 支付订单
type Order struct {
	IdModel
	UserId        uint                  `json:"user_id" gorm:"index;not null"`            // 用户ID
	PlanId        uint                  `json:"plan_id" gorm:"index;not null"`            // 套餐ID
	OutTradeNo    string                `json:"out_trade_no" gorm:"uniqueIndex;not null"` // 业务订单号
	TradeNo       string                `json:"trade_no" gorm:"index"`                    // 平台订单号
	Subject       string                `json:"subject" gorm:"not null"`                  // 订单标题
	Amount        int64                 `json:"amount" gorm:"not null"`                   // 金额(分)
	AmountYuan    string                `json:"amount_yuan" gorm:"not null"`              // 金额(元字符串,用于对账)
	Status        int                   `json:"status" gorm:"default:0;index"`            // 状态: 0待支付 1已支付 2已退款 3已关闭
	PaySubmitAt   int64                 `json:"pay_submit_at" gorm:"default:0"`           // 最近一次发起支付时间(秒)
	PaidAt        int64                 `json:"paid_at" gorm:"default:0"`                 // 支付时间
	RefundedAt    int64                 `json:"refunded_at" gorm:"default:0"`             // 退款时间
	NotifyPayload string                `json:"notify_payload" gorm:"type:text"`          // 回调原始数据
	PayURL        string                `json:"pay_url,omitempty" gorm:"-"`               // 支付跳转URL(接口计算返回)
	User          *User                 `json:"user,omitempty" gorm:"foreignKey:UserId"`
	Plan          *SubscriptionPlan     `json:"plan,omitempty" gorm:"foreignKey:PlanId"`
	CreatedAt     custom_types.AutoTime `json:"created_at" gorm:"type:timestamp;index"`
	UpdatedAt     custom_types.AutoTime `json:"updated_at" gorm:"type:timestamp;"`
}

type OrderList struct {
	Orders []*Order `json:"list"`
	Pagination
}

// UserSubscription 用户订阅
type UserSubscription struct {
	IdModel
	UserId      uint                  `json:"user_id" gorm:"uniqueIndex;not null"` // 用户ID(一用户一条)
	PlanId      uint                  `json:"plan_id" gorm:"index;not null"`       // 当前套餐ID
	LastOrderId uint                  `json:"last_order_id" gorm:"index"`          // 最近订单ID
	StartAt     int64                 `json:"start_at" gorm:"not null"`            // 开始时间
	ExpireAt    int64                 `json:"expire_at" gorm:"not null;index"`     // 过期时间
	Status      int                   `json:"status" gorm:"default:1;index"`       // 状态: 1有效 2已过期 3已取消
	User        *User                 `json:"user,omitempty" gorm:"foreignKey:UserId"`
	Plan        *SubscriptionPlan     `json:"plan,omitempty" gorm:"foreignKey:PlanId"`
	LastOrder   *Order                `json:"last_order,omitempty" gorm:"foreignKey:LastOrderId"`
	CreatedAt   custom_types.AutoTime `json:"created_at" gorm:"type:timestamp;"`
	UpdatedAt   custom_types.AutoTime `json:"updated_at" gorm:"type:timestamp;"`
}

type UserSubscriptionList struct {
	Subscriptions []*UserSubscription `json:"list"`
	Pagination
}

// PriceYuan 返回元为单位的价格字符串
func (p *SubscriptionPlan) PriceYuan() string {
	return FenToYuan(p.Price)
}

// FenToYuan 分转元(避免浮点精度问题)
func FenToYuan(fen int64) string {
	sign := ""
	if fen < 0 {
		sign = "-"
		fen = -fen
	}
	return fmt.Sprintf("%s%d.%02d", sign, fen/100, fen%100)
}

// YuanToFen 元转分(字符串严格解析,避免浮点精度问题)
func YuanToFen(yuan string) (int64, error) {
	s := strings.TrimSpace(yuan)
	if s == "" {
		return 0, errors.New("invalid money")
	}
	// 去除正号
	s = strings.TrimPrefix(s, "+")
	// 不支持负数
	if strings.HasPrefix(s, "-") {
		return 0, errors.New("invalid money: negative")
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) == 0 || len(parts) > 2 {
		return 0, errors.New("invalid money format")
	}

	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}

	if intPart == "" {
		intPart = "0"
	}

	// 处理小数部分
	switch len(fracPart) {
	case 0:
		fracPart = "00"
	case 1:
		fracPart += "0"
	case 2:
		// OK
	default:
		return 0, errors.New("invalid money: too many decimal places")
	}

	// 验证是否全为数字
	if !isAllDigits(intPart) || !isAllDigits(fracPart) {
		return 0, errors.New("invalid money: non-digit characters")
	}

	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("invalid money: integer part")
	}
	cents, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil || cents < 0 || cents > 99 {
		return 0, errors.New("invalid money: decimal part")
	}

	// 溢出检查
	const maxInt64 = int64(^uint64(0) >> 1)
	if whole > (maxInt64-cents)/100 {
		return 0, errors.New("invalid money: overflow")
	}
	return whole*100 + cents, nil
}

func isAllDigits(s string) bool {
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
