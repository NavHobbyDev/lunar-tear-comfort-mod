package campaign

type RateBonus struct {
	override    int32
	bonusPermil int32
}

func (b RateBonus) Apply(basePermil int32) int32 {
	base := basePermil
	if b.override > 0 {
		base = b.override
	}
	return clampPermil(int32(int64(base) + int64(b.bonusPermil)))
}

type ExpBonus struct {
	bonusPermil int32
}

func (b ExpBonus) Apply(base int32) int32 {
	return int32(int64(base) * int64(1000+b.bonusPermil) / 1000)
}

type StaminaMul struct {
	permil int32
}

func (m StaminaMul) Apply(base int32) int32 {
	if m.permil == 1000 {
		return base
	}
	return int32(int64(base) * int64(m.permil) / 1000)
}

type DropRateMul struct {
	bonusPermil int32
}

func (m DropRateMul) Apply(base int32) int32 {
	return int32((int64(base)*int64(1000+m.bonusPermil) + 999) / 1000)
}

type BonusDrop struct {
	PossessionType int32
	PossessionId   int32
	Count          int32
	RarityType     int32
}

func clampPermil(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 1000 {
		return 1000
	}
	return v
}
