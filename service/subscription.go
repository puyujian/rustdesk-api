package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SubscriptionService struct{}

// ========== 套餐管理 ==========

// GetPlanById 根据ID获取套餐
func (ss *SubscriptionService) GetPlanById(id uint) *model.SubscriptionPlan {
	plan := &model.SubscriptionPlan{}
	DB.Where("id = ?", id).First(plan)
	return plan
}

// GetPlanByCode 根据编码获取套餐
func (ss *SubscriptionService) GetPlanByCode(code string) *model.SubscriptionPlan {
	plan := &model.SubscriptionPlan{}
	DB.Where("code = ?", code).First(plan)
	return plan
}

// ListActivePlans 获取启用的套餐列表
func (ss *SubscriptionService) ListActivePlans() []*model.SubscriptionPlan {
	var plans []*model.SubscriptionPlan
	DB.Where("status = ?", model.COMMON_STATUS_ENABLE).Order("sort_order ASC, id ASC").Find(&plans)
	return plans
}

// ListPlans 获取套餐列表(分页)
func (ss *SubscriptionService) ListPlans(page, pageSize uint, where func(tx *gorm.DB)) *model.SubscriptionPlanList {
	res := &model.SubscriptionPlanList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.SubscriptionPlan{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Order("sort_order ASC, id ASC").Find(&res.Plans)
	return res
}

// CreatePlan 创建套餐
func (ss *SubscriptionService) CreatePlan(plan *model.SubscriptionPlan) error {
	return DB.Create(plan).Error
}

// UpdatePlan 更新套餐
func (ss *SubscriptionService) UpdatePlan(plan *model.SubscriptionPlan) error {
	return DB.Save(plan).Error
}

// DeletePlan 删除套餐(软删除:禁用)
func (ss *SubscriptionService) DeletePlan(id uint) error {
	return DB.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Update("status", model.COMMON_STATUS_DISABLED).Error
}

// ========== 订单管理 ==========

// GenerateOutTradeNo 生成业务订单号
func (ss *SubscriptionService) GenerateOutTradeNo(userId uint) string {
	// 格式: RD + 日期 + 用户ID + 随机数
	return fmt.Sprintf("RD%s%d%s", time.Now().Format("20060102150405"), userId, utils.RandomString(6))
}

// CreateOrder 创建订单并返回支付URL
func (ss *SubscriptionService) CreateOrder(userId, planId uint) (outTradeNo, payURL string, err error) {
	// 1. 检查套餐
	plan := ss.GetPlanById(planId)
	if plan.Id == 0 {
		return "", "", errors.New("PlanNotFound")
	}
	if plan.Status != model.COMMON_STATUS_ENABLE {
		return "", "", errors.New("PlanDisabled")
	}

	// 免费套餐：直接创建已支付订单并激活订阅
	if plan.Price == 0 {
		outTradeNo = ss.GenerateOutTradeNo(userId)
		amountYuan := model.FenToYuan(plan.Price)
		now := time.Now().Unix()

		err = DB.Transaction(func(tx *gorm.DB) error {
			order := &model.Order{
				UserId:     userId,
				PlanId:     planId,
				OutTradeNo: outTradeNo,
				Subject:    plan.Name,
				Amount:     plan.Price,
				AmountYuan: amountYuan,
				Status:     model.OrderStatusPaid,
				PaidAt:     now,
			}
			if err := tx.Create(order).Error; err != nil {
				Logger.Error("Create free order failed: ", err)
				return err
			}
			return ss.activateOrExtendSubscription(tx, order.UserId, order.PlanId, order.Id, now)
		})
		if err != nil {
			return "", "", err
		}
		return outTradeNo, "", nil
	}

	// 复用同一套餐的最新待支付订单，避免重复创建
	existing := &model.Order{}
	if err := DB.Where("user_id = ? AND plan_id = ? AND status = ?", userId, planId, model.OrderStatusPending).
		Order("id DESC").
		First(existing).Error; err == nil && existing.Id != 0 {
		payURL = AllService.PaymentService.BuildPayURL(existing.OutTradeNo)
		return existing.OutTradeNo, payURL, nil
	}

	// 2. 生成订单号
	outTradeNo = ss.GenerateOutTradeNo(userId)
	amountYuan := model.FenToYuan(plan.Price)

	// 3. 创建订单
	order := &model.Order{
		UserId:     userId,
		PlanId:     planId,
		OutTradeNo: outTradeNo,
		Subject:    plan.Name,
		Amount:     plan.Price,
		AmountYuan: amountYuan,
		Status:     model.OrderStatusPending,
	}
	if err := DB.Create(order).Error; err != nil {
		Logger.Error("Create order failed: ", err)
		return "", "", err
	}

	// 4. 构建支付URL
	payURL = AllService.PaymentService.BuildPayURL(outTradeNo)

	return outTradeNo, payURL, nil
}

// GetOrderByOutTradeNo 根据业务订单号获取订单
func (ss *SubscriptionService) GetOrderByOutTradeNo(outTradeNo string) *model.Order {
	order := &model.Order{}
	DB.Where("out_trade_no = ?", outTradeNo).First(order)
	return order
}

// GetOrderById 根据ID获取订单
func (ss *SubscriptionService) GetOrderById(id uint) *model.Order {
	order := &model.Order{}
	DB.Where("id = ?", id).First(order)
	return order
}

// ListOrders 获取订单列表(分页)
func (ss *SubscriptionService) ListOrders(page, pageSize uint, where func(tx *gorm.DB)) *model.OrderList {
	res := &model.OrderList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Order{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Preload("User").Preload("Plan").Order("id DESC").Find(&res.Orders)
	return res
}

// ListUserOrders 获取用户订单列表
func (ss *SubscriptionService) ListUserOrders(userId uint, page, pageSize uint) *model.OrderList {
	return ss.ListOrders(page, pageSize, func(tx *gorm.DB) {
		tx.Where("user_id = ?", userId)
	})
}

// HandleNotify 处理支付回调
func (ss *SubscriptionService) HandleNotify(params map[string]string) error {
	outTradeNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	money := params["money"]
	pid := params["pid"]

	// 1. 验签
	if !AllService.PaymentService.Verify(params) {
		// 仅记录关键字段,避免泄露敏感信息
		Logger.Warn("Payment notify sign verify failed, out_trade_no: ", outTradeNo, " trade_no: ", tradeNo, " pid: ", pid)
		return errors.New("SignVerifyFailed")
	}

	// 2. 参数校验
	if outTradeNo == "" || tradeNo == "" || money == "" {
		Logger.Warn("Payment notify missing params, out_trade_no: ", outTradeNo, " trade_no: ", tradeNo, " money: ", money)
		return errors.New("ParamsError")
	}

	// 3. 校验pid是否匹配
	cfg := AllService.PaymentService.GetConfig()
	if pid != "" && pid != cfg.Pid {
		Logger.Warn("Payment notify pid mismatch, out_trade_no: ", outTradeNo, " expected: ", cfg.Pid, " got: ", pid)
		return errors.New("PidMismatch")
	}

	// 4. 检查交易状态
	tradeStatus := params["trade_status"]
	if tradeStatus != "TRADE_SUCCESS" {
		Logger.Info("Payment notify trade_status is not TRADE_SUCCESS: ", tradeStatus)
		return nil // 非成功状态,忽略
	}

	// 5. 使用事务处理
	return DB.Transaction(func(tx *gorm.DB) error {
		// 5.1 查询订单(加行锁)
		order := &model.Order{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("out_trade_no = ?", outTradeNo).First(order).Error; err != nil {
			Logger.Error("Payment notify order not found: ", outTradeNo)
			return errors.New("OrderNotFound")
		}

		// 5.2 幂等检查
		if order.Status == model.OrderStatusPaid || order.Status == model.OrderStatusRefunded {
			Logger.Info("Payment notify order already processed: ", outTradeNo)
			return nil // 已处理,直接返回成功
		}
		if order.Status == model.OrderStatusClosed {
			Logger.Warn("Payment notify ignored for closed order: ", outTradeNo)
			return nil // 已关闭订单,忽略
		}

		// 5.3 校验金额(使用分为单位比较,更精确)
		moneyFen, err := ss.ParseMoneyToFen(money)
		if err != nil {
			Logger.Error("Payment notify parse money failed: ", err)
			return errors.New("InvalidMoney")
		}
		if moneyFen != order.Amount {
			Logger.Error("Payment notify amount mismatch, expected: ", order.Amount, " got: ", moneyFen)
			return errors.New("AmountMismatch")
		}

		// 5.4 更新订单状态(保存回调原始数据为JSON)
		now := time.Now().Unix()
		payloadBytes, _ := json.Marshal(params)
		if err := tx.Model(order).Updates(map[string]interface{}{
			"trade_no":       tradeNo,
			"status":         model.OrderStatusPaid,
			"paid_at":        now,
			"notify_payload": string(payloadBytes),
		}).Error; err != nil {
			Logger.Error("Payment notify update order failed: ", err)
			return err
		}

		// 3.5 激活/续期订阅
		if err := ss.activateOrExtendSubscription(tx, order.UserId, order.PlanId, order.Id, now); err != nil {
			Logger.Error("Payment notify activate subscription failed: ", err)
			return err
		}

		Logger.Info("Payment notify success, order: ", outTradeNo, " user: ", order.UserId)
		return nil
	})
}

// activateOrExtendSubscription 激活或续期订阅(事务内调用)
func (ss *SubscriptionService) activateOrExtendSubscription(tx *gorm.DB, userId, planId, orderId uint, now int64) error {
	// 1. 获取套餐
	plan := &model.SubscriptionPlan{}
	if err := tx.Where("id = ?", planId).First(plan).Error; err != nil {
		return err
	}

	// 2. 查询现有订阅(加行锁)
	sub := &model.UserSubscription{}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userId).First(sub).Error

	// 3. 计算新的过期时间
	var startAt, expireAt int64
	if err == gorm.ErrRecordNotFound {
		// 新订阅
		startAt = now
		expireAt = ss.calcExpireTime(now, plan.PeriodUnit, plan.PeriodCount)
	} else if err != nil {
		return err
	} else {
		// 续期: 如果当前订阅未过期,从过期时间续期;否则从现在开始
		if sub.ExpireAt > now && sub.Status == model.SubscriptionStatusActive {
			startAt = sub.StartAt
			expireAt = ss.calcExpireTime(sub.ExpireAt, plan.PeriodUnit, plan.PeriodCount)
		} else {
			startAt = now
			expireAt = ss.calcExpireTime(now, plan.PeriodUnit, plan.PeriodCount)
		}
	}

	// 4. 更新或创建订阅
	if sub.Id == 0 {
		// 创建新订阅
		sub = &model.UserSubscription{
			UserId:      userId,
			PlanId:      planId,
			LastOrderId: orderId,
			StartAt:     startAt,
			ExpireAt:    expireAt,
			Status:      model.SubscriptionStatusActive,
		}
		return tx.Create(sub).Error
	} else {
		// 更新订阅
		return tx.Model(sub).Updates(map[string]interface{}{
			"plan_id":       planId,
			"last_order_id": orderId,
			"start_at":      startAt,
			"expire_at":     expireAt,
			"status":        model.SubscriptionStatusActive,
		}).Error
	}
}

// calcExpireTime 计算过期时间
func (ss *SubscriptionService) calcExpireTime(baseTime int64, periodUnit string, periodCount int) int64 {
	t := time.Unix(baseTime, 0)
	switch periodUnit {
	case model.PeriodUnitDay:
		t = t.AddDate(0, 0, periodCount)
	case model.PeriodUnitMonth:
		t = t.AddDate(0, periodCount, 0)
	case model.PeriodUnitYear:
		t = t.AddDate(periodCount, 0, 0)
	default:
		t = t.AddDate(0, periodCount, 0) // 默认按月
	}
	return t.Unix()
}

// ========== 订阅查询 ==========

// GetUserSubscription 获取用户订阅
func (ss *SubscriptionService) GetUserSubscription(userId uint) *model.UserSubscription {
	sub := &model.UserSubscription{}
	DB.Where("user_id = ?", userId).Preload("Plan").First(sub)
	return sub
}

// GetSubscriptionById 获取订阅详情(管理员)
func (ss *SubscriptionService) GetSubscriptionById(id uint) *model.UserSubscription {
	sub := &model.UserSubscription{}
	DB.Where("id = ?", id).Preload("User").Preload("Plan").Preload("LastOrder").First(sub)
	return sub
}

// IsSubscriptionActive 检查用户订阅是否有效
func (ss *SubscriptionService) IsSubscriptionActive(userId uint) bool {
	sub := ss.GetUserSubscription(userId)
	if sub.Id == 0 {
		return false
	}
	now := time.Now().Unix()
	return sub.Status == model.SubscriptionStatusActive && sub.ExpireAt > now
}

// ListSubscriptions 获取订阅列表(分页)
func (ss *SubscriptionService) ListSubscriptions(page, pageSize uint, where func(tx *gorm.DB)) *model.UserSubscriptionList {
	res := &model.UserSubscriptionList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.UserSubscription{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Preload("User").Preload("Plan").Order("id DESC").Find(&res.Subscriptions)
	return res
}

// ========== 退款处理 ==========

// RefundOrder 退款订单
func (ss *SubscriptionService) RefundOrder(orderId uint, reason string) error {
	order := ss.GetOrderById(orderId)
	if order.Id == 0 {
		return errors.New("OrderNotFound")
	}
	if order.Status != model.OrderStatusPaid {
		return errors.New("OrderNotPaid")
	}
	if order.TradeNo == "" {
		return errors.New("TradeNoEmpty")
	}

	// 调用支付网关退款
	_, err := AllService.PaymentService.Refund(order.TradeNo, order.AmountYuan)
	if err != nil {
		Logger.Error("Refund order failed: ", err)
		return err
	}

	// 更新订单状态
	now := time.Now().Unix()
	if err := DB.Model(order).Updates(map[string]interface{}{
		"status":      model.OrderStatusRefunded,
		"refunded_at": now,
	}).Error; err != nil {
		return err
	}

	// 取消用户订阅(简单处理:直接标记取消)
	DB.Model(&model.UserSubscription{}).Where("user_id = ?", order.UserId).Updates(map[string]interface{}{
		"status": model.SubscriptionStatusCanceled,
	})

	Logger.Info("Refund order success, order: ", order.OutTradeNo, " reason: ", reason)
	return nil
}

// ========== 管理员操作 ==========

// GrantSubscription 管理员赠送订阅时长
func (ss *SubscriptionService) GrantSubscription(userId, planId uint, days int) error {
	plan := ss.GetPlanById(planId)
	if plan.Id == 0 {
		return errors.New("PlanNotFound")
	}

	now := time.Now().Unix()
	expireAt := time.Unix(now, 0).AddDate(0, 0, days).Unix()

	sub := ss.GetUserSubscription(userId)
	if sub.Id == 0 {
		// 创建新订阅
		sub = &model.UserSubscription{
			UserId:   userId,
			PlanId:   planId,
			StartAt:  now,
			ExpireAt: expireAt,
			Status:   model.SubscriptionStatusActive,
		}
		return DB.Create(sub).Error
	} else {
		// 续期
		if sub.ExpireAt > now && sub.Status == model.SubscriptionStatusActive {
			expireAt = time.Unix(sub.ExpireAt, 0).AddDate(0, 0, days).Unix()
		}
		return DB.Model(sub).Updates(map[string]interface{}{
			"plan_id":   planId,
			"expire_at": expireAt,
			"status":    model.SubscriptionStatusActive,
		}).Error
	}
}

// CancelSubscription 管理员取消订阅
func (ss *SubscriptionService) CancelSubscription(userId uint) error {
	return DB.Model(&model.UserSubscription{}).Where("user_id = ?", userId).Updates(map[string]interface{}{
		"status": model.SubscriptionStatusCanceled,
	}).Error
}

// ========== 辅助函数 ==========

// ParseMoneyToFen 解析金额字符串为分(使用字符串严格解析,避免浮点精度问题)
func (ss *SubscriptionService) ParseMoneyToFen(money string) (int64, error) {
	return model.YuanToFen(money)
}
