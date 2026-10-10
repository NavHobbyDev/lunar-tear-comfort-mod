package store

import (
	"log"

	"lunar-tear/server/internal/model"
)

const StaminaRecoveryDivisor int64 = 180

func SettleStamina(user *UserState, maxStaminaMillis int32, nowMillis int64) {
	stored := int64(user.Status.StaminaMilliValue)
	maxMilli := int64(maxStaminaMillis)
	if stored >= maxMilli {
		return
	}
	elapsed := nowMillis - user.Status.StaminaUpdateDatetime
	if elapsed <= 0 {
		return
	}
	regen := elapsed / StaminaRecoveryDivisor
	settled := min(stored+regen, maxMilli)
	user.Status.StaminaMilliValue = int32(settled)
	user.Status.StaminaUpdateDatetime = nowMillis
}

func ConsumeStamina(user *UserState, costUnits int32, maxStaminaMillis int32, nowMillis int64) {
	SettleStamina(user, maxStaminaMillis, nowMillis)
	user.Status.StaminaMilliValue = max(user.Status.StaminaMilliValue-costUnits*1000, 0)
	user.Status.StaminaUpdateDatetime = nowMillis
	log.Printf("[ConsumeStamina] cost=%d -> remaining=%d", costUnits, user.Status.StaminaMilliValue)
}

func RecoverStamina(user *UserState, millis int32, maxStaminaMillis int32, nowMillis int64) {
	SettleStamina(user, maxStaminaMillis, nowMillis)
	user.Status.StaminaMilliValue += millis
	user.Status.StaminaUpdateDatetime = nowMillis
	log.Printf("[RecoverStamina] +%d -> total=%d", millis, user.Status.StaminaMilliValue)
}

func ReplenishStamina(user *UserState, maxStaminaMillis int32, nowMillis int64) {
	user.Status.StaminaMilliValue = maxStaminaMillis
	user.Status.StaminaUpdateDatetime = nowMillis
	log.Printf("[ReplenishStamina] set to %d", maxStaminaMillis)
}

func ResolveStaminaEffectMillis(effectValueType, effectValue, maxStaminaMillis int32) int32 {
	switch effectValueType {
	case model.EffectValueFixed:
		return effectValue * 1000
	case model.EffectValuePermil:
		return effectValue * maxStaminaMillis / 1000
	default:
		return 0
	}
}
