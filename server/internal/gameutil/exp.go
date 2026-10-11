package gameutil

// LevelAndCap returns the level for the given cumulative EXP based on
// the thresholds slice, and clamps EXP so it never exceeds the last
// threshold (max-level cap).
func LevelAndCap(exp int32, thresholds []int32) (level, capped int32) {
	level = 1
	for lvl := 1; lvl < len(thresholds); lvl++ {
		if exp >= thresholds[lvl] {
			level = int32(lvl)
		} else {
			break
		}
	}
	if len(thresholds) > 0 && exp > thresholds[len(thresholds)-1] {
		exp = thresholds[len(thresholds)-1]
	}
	return level, exp
}

// ApplyExpWithMaxLevel runs LevelAndCap and then clamps the resulting
// level to the per-instance maxLevel (e.g. limit break + awaken for
// weapons, limit break + rebirth for costumes). A maxLevel <= 0 means
// "no per-instance cap" and the result is identical to LevelAndCap.
func ApplyExpWithMaxLevel(exp int32, thresholds []int32, maxLevel int32) (level, capped int32) {
	level, capped = LevelAndCap(exp, thresholds)
	if maxLevel > 0 && level > maxLevel && int(maxLevel) < len(thresholds) {
		level = maxLevel
		capped = thresholds[maxLevel]
	}
	return
}
