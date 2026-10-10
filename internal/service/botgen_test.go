package service

import "testing"

func TestSynthBot_deterministic(t *testing.T) {
	pools := botPools{costumeIds: []int32{10, 20, 30}, weaponIds: []int32{1, 2}, companionIds: []int32{5}}
	a := synthBot(pools, 42, 0, 1000, 5000)
	b := synthBot(pools, 42, 0, 1000, 5000)
	if a != b {
		t.Fatalf("same seed must yield identical bot:\n%+v\n%+v", a, b)
	}
	c := synthBot(pools, 42, 1, 1000, 5000)
	if a.PlayerId == c.PlayerId {
		t.Fatalf("different slot must yield a different bot id")
	}
}

func TestSynthBot_idRange(t *testing.T) {
	pools := botPools{costumeIds: []int32{10}, weaponIds: []int32{1}}
	for slot := 0; slot < 50; slot++ {
		bot := synthBot(pools, 7, slot, 99, 3000)
		if !IsBotId(bot.PlayerId) {
			t.Fatalf("bot id %d not in reserved range", bot.PlayerId)
		}
	}
}
