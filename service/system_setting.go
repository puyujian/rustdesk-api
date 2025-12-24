package service

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

type SystemSettingService struct {
	cache     map[string]*cacheItem
	cacheLock sync.RWMutex
}

type cacheItem struct {
	value     string
	expiredAt time.Time
}

const cacheTTL = 5 * time.Minute

// Get 获取设置值
func (s *SystemSettingService) Get(key string) string {
	// 先查缓存
	s.cacheLock.RLock()
	if s.cache != nil {
		if item, ok := s.cache[key]; ok && time.Now().Before(item.expiredAt) {
			s.cacheLock.RUnlock()
			return item.value
		}
	}
	s.cacheLock.RUnlock()

	// 查数据库
	var setting model.SystemSetting
	if err := DB.Where("key = ?", key).First(&setting).Error; err != nil {
		return ""
	}

	// 写入缓存
	s.cacheLock.Lock()
	if s.cache == nil {
		s.cache = make(map[string]*cacheItem)
	}
	s.cache[key] = &cacheItem{
		value:     setting.Value,
		expiredAt: time.Now().Add(cacheTTL),
	}
	s.cacheLock.Unlock()

	return setting.Value
}

// Set 设置值
func (s *SystemSettingService) Set(key, value string) error {
	var setting model.SystemSetting
	err := DB.Where("key = ?", key).First(&setting).Error
	if err != nil {
		// 不存在则创建
		setting = model.SystemSetting{
			Key:   key,
			Value: value,
		}
		err = DB.Create(&setting).Error
	} else {
		// 存在则更新
		err = DB.Model(&setting).Update("value", value).Error
	}

	if err != nil {
		return err
	}

	// 更新缓存
	s.cacheLock.Lock()
	if s.cache == nil {
		s.cache = make(map[string]*cacheItem)
	}
	s.cache[key] = &cacheItem{
		value:     value,
		expiredAt: time.Now().Add(cacheTTL),
	}
	s.cacheLock.Unlock()

	return nil
}

// Delete 删除设置
func (s *SystemSettingService) Delete(key string) error {
	// 删除缓存
	s.cacheLock.Lock()
	if s.cache != nil {
		delete(s.cache, key)
	}
	s.cacheLock.Unlock()

	return DB.Where("key = ?", key).Delete(&model.SystemSetting{}).Error
}

// ClearCache 清除缓存
func (s *SystemSettingService) ClearCache(key string) {
	s.cacheLock.Lock()
	if key == "" {
		s.cache = make(map[string]*cacheItem)
	} else if s.cache != nil {
		delete(s.cache, key)
	}
	s.cacheLock.Unlock()
}

// GetPaymentConfig 获取支付配置
func (s *SystemSettingService) GetPaymentConfig() *model.PaymentConfig {
	value := s.Get(model.SettingKeyPaymentConfig)
	if value == "" {
		// 返回默认配置（从配置文件读取作为fallback）
		return &model.PaymentConfig{
			Enable:    Config.Payment.EasyPay.Enable,
			BaseURL:   Config.Payment.EasyPay.BaseURL,
			Pid:       Config.Payment.EasyPay.Pid,
			Key:       Config.Payment.EasyPay.Key,
			NotifyURL: Config.Payment.EasyPay.NotifyURL,
			ReturnURL: Config.Payment.EasyPay.ReturnURL,
			Timeout:   int(Config.Payment.EasyPay.Timeout.Seconds()),
		}
	}

	var cfg model.PaymentConfig
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		Logger.Error("Parse payment config failed: ", err)
		return &model.PaymentConfig{}
	}
	return &cfg
}

// SetPaymentConfig 保存支付配置
func (s *SystemSettingService) SetPaymentConfig(cfg *model.PaymentConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.Set(model.SettingKeyPaymentConfig, string(data))
}
