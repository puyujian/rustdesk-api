package service

import (
	"sync"
	"time"
)

// RelayWhitelistService 管理 relay uuid 白名单
// 用于 hbbs 写入允许的 uuid，hbbr 消费验证
type RelayWhitelistService struct {
	mu    sync.RWMutex
	items map[string]*whitelistItem
}

type whitelistItem struct {
	slots    int       // 剩余可用次数
	expireAt time.Time // 过期时间
}

// NewRelayWhitelistService 创建白名单服务实例
func NewRelayWhitelistService() *RelayWhitelistService {
	svc := &RelayWhitelistService{
		items: make(map[string]*whitelistItem),
	}
	// 启动清理协程
	go svc.cleanupLoop()
	return svc
}

// Allow 写入白名单
// uuid: relay 会话 uuid
// slots: 允许消费次数 (通常为 2，因为 relay 需要两端各连接一次)
// ttlSec: 过期时间(秒)
func (s *RelayWhitelistService) Allow(uuid string, slots int, ttlSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if slots <= 0 {
		slots = 2
	}
	if ttlSec <= 0 {
		ttlSec = 120
	}

	s.items[uuid] = &whitelistItem{
		slots:    slots,
		expireAt: time.Now().Add(time.Duration(ttlSec) * time.Second),
	}
	Logger.Debugf("RelayWhitelist: allow uuid=%s slots=%d ttl=%ds", uuid, slots, ttlSec)
}

// Consume 消费白名单
// 返回 true 表示允许，false 表示拒绝
func (s *RelayWhitelistService) Consume(uuid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, exists := s.items[uuid]
	if !exists {
		Logger.Debugf("RelayWhitelist: consume uuid=%s not found", uuid)
		return false
	}

	// 检查是否过期
	if time.Now().After(item.expireAt) {
		delete(s.items, uuid)
		Logger.Debugf("RelayWhitelist: consume uuid=%s expired", uuid)
		return false
	}

	// 检查剩余次数
	if item.slots <= 0 {
		delete(s.items, uuid)
		Logger.Debugf("RelayWhitelist: consume uuid=%s no slots left", uuid)
		return false
	}

	// 扣减次数
	item.slots--
	Logger.Debugf("RelayWhitelist: consume uuid=%s success, remaining=%d", uuid, item.slots)

	// 如果次数用完，删除条目
	if item.slots <= 0 {
		delete(s.items, uuid)
	}

	return true
}

// Check 检查 uuid 是否在白名单中（不消费）
func (s *RelayWhitelistService) Check(uuid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[uuid]
	if !exists {
		return false
	}

	if time.Now().After(item.expireAt) {
		return false
	}

	return item.slots > 0
}

// cleanupLoop 定期清理过期条目
func (s *RelayWhitelistService) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

func (s *RelayWhitelistService) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for uuid, item := range s.items {
		if now.After(item.expireAt) || item.slots <= 0 {
			delete(s.items, uuid)
		}
	}
}

// Stats 返回当前白名单统计信息
func (s *RelayWhitelistService) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"count": len(s.items),
	}
}
