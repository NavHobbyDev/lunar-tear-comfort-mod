package campaign

func (c *Catalog) PartsRateBonus(t PartsTarget, f Filter) RateBonus {
	var out RateBonus
	for _, r := range c.enhance {
		if !r.isActive(f) {
			continue
		}
		if !matchesParts(r.targets, t) {
			continue
		}
		out = applyEnhanceEffect(out, r)
	}
	return out
}

func (c *Catalog) CostumeExpBonus(t CostumeTarget, f Filter) ExpBonus {
	var sum int32
	for _, r := range c.enhance {
		if !r.isActive(f) || r.effectType != EnhanceEffectAdditionalPerm {
			continue
		}
		if matchesCostume(r.targets, t) {
			sum += r.effectValue
		}
	}
	return ExpBonus{bonusPermil: sum}
}

func (c *Catalog) WeaponExpBonus(t WeaponTarget, f Filter) ExpBonus {
	var sum int32
	for _, r := range c.enhance {
		if !r.isActive(f) || r.effectType != EnhanceEffectAdditionalPerm {
			continue
		}
		if matchesWeapon(r.targets, t) {
			sum += r.effectValue
		}
	}
	return ExpBonus{bonusPermil: sum}
}

func applyEnhanceEffect(b RateBonus, r enhanceRow) RateBonus {
	switch r.effectType {
	case EnhanceEffectProbability:
		b.override = r.effectValue
	case EnhanceEffectAdditionalPerm:
		b.bonusPermil += r.effectValue
	}
	return b
}

func matchesParts(targets []enhanceMatch, t PartsTarget) bool {
	for _, m := range targets {
		switch m.t {
		case EnhanceTargetPartsAll:
			return true
		case EnhanceTargetPartsSeriesId:
			if m.v == t.PartsGroupId {
				return true
			}
		case EnhanceTargetPartsId:
			if m.v == t.PartsId {
				return true
			}
		}
	}
	return false
}

func matchesCostume(targets []enhanceMatch, t CostumeTarget) bool {
	for _, m := range targets {
		switch m.t {
		case EnhanceTargetCostumeAll:
			return true
		case EnhanceTargetCostumeCharacterId:
			if m.v == t.CharacterId {
				return true
			}
		case EnhanceTargetCostumeSkillfulWeapon:
			if m.v == t.SkillfulWeaponType {
				return true
			}
		case EnhanceTargetCostumeId:
			if m.v == t.CostumeId {
				return true
			}
		}
	}
	return false
}

func matchesWeapon(targets []enhanceMatch, t WeaponTarget) bool {
	for _, m := range targets {
		switch m.t {
		case EnhanceTargetWeaponAll:
			return true
		case EnhanceTargetWeaponTypeId:
			if m.v == t.WeaponType {
				return true
			}
		case EnhanceTargetWeaponAttributeTypeId:
			if m.v == t.AttributeType {
				return true
			}
		case EnhanceTargetWeaponId:
			if m.v == t.WeaponId {
				return true
			}
		}
	}
	return false
}
