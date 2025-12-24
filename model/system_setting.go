package model

import "github.com/lejianwen/rustdesk-api/v2/model/custom_types"

// SystemSetting 系统设置（key-value存储）
type SystemSetting struct {
	Id        uint                  `json:"id" gorm:"primaryKey"`
	Key       string                `json:"key" gorm:"uniqueIndex;size:128;not null"`
	Value     string                `json:"value" gorm:"type:text"`
	CreatedAt custom_types.AutoTime `json:"created_at" gorm:"type:timestamp"`
	UpdatedAt custom_types.AutoTime `json:"updated_at" gorm:"type:timestamp"`
}

func (SystemSetting) TableName() string {
	return "system_settings"
}

// PaymentConfig 支付配置结构（用于JSON序列化）
type PaymentConfig struct {
	Enable    bool   `json:"enable"`
	BaseURL   string `json:"base_url"`
	Pid       string `json:"pid"`
	Key       string `json:"key"`
	NotifyURL string `json:"notify_url"`
	ReturnURL string `json:"return_url"`
	Timeout   int    `json:"timeout"` // 秒
}

// 支付配置 key 常量
const (
	SettingKeyPaymentConfig = "payment.epay.config"
)
