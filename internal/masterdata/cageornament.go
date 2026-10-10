package masterdata

import (
	"log"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/utils"
	"math/rand"
)

type CageOrnamentReward struct {
	PossessionType int32
	PossessionId   int32
	Count          int32
}

type LostItemRewardConfig struct {
	PossessionType int32
	PossessionId   int32
	Count          int32
	Weight         int32
}

type CageOrnamentCatalog struct {
	ornamentToRewardId map[int32]int32
	rewards            map[int32]CageOrnamentReward
	rewardPool         []CageOrnamentReward
}

type LostItemsCatalog struct {
	rewardPool []CageOrnamentReward
	ornaments  map[GimmickOrnamentRef]*EntityMGimmickOrnament
}

func (c *CageOrnamentCatalog) LookupReward(cageOrnamentId int32) (CageOrnamentReward, bool) {
	rewardId, ok := c.ornamentToRewardId[cageOrnamentId]
	if !ok || rewardId == 0 {
		return CageOrnamentReward{}, false
	}
	entry, ok := c.rewards[rewardId]
	return entry, ok
}

func (c *CageOrnamentCatalog) RandomReward() CageOrnamentReward {
	if len(c.rewardPool) == 0 {
		return CageOrnamentReward{}
	}
	return c.rewardPool[rand.Intn(len(c.rewardPool))]
}

func (l *LostItemsCatalog) RandomReward() CageOrnamentReward {
	if len(l.rewardPool) == 0 {
		return CageOrnamentReward{}
	}
	return l.selectWeightedReward()
}

func (l *LostItemsCatalog) RandomRewardWithBonus(gimmickId, ornamentIndex int32) CageOrnamentReward {
	reward := l.selectWeightedReward()
	
	ornament, ok := l.ornaments[GimmickOrnamentRef{GimmickId: gimmickId, OrnamentIndex: ornamentIndex}]
	if !ok {
		return reward
	}
	
	bonus := int32(float32(reward.Count) * 0.5 * float32(ornament.ChapterId))
	
	return CageOrnamentReward{
		PossessionType: reward.PossessionType,
		PossessionId:   reward.PossessionId,
		Count:          reward.Count + bonus,
	}
}

func (l *LostItemsCatalog) selectWeightedReward() CageOrnamentReward {
	rewards := LostItemRewardsConfig()
	if len(rewards) == 0 {
		return CageOrnamentReward{}
	}
	
	totalWeight := int32(0)
	for _, r := range rewards {
		totalWeight += r.Weight
	}
	
	if totalWeight <= 0 {
		return CageOrnamentReward{}
	}
	
	rnd := rand.Int31n(totalWeight)
	current := int32(0)
	for _, r := range rewards {
		current += r.Weight
		if rnd < current {
			return CageOrnamentReward{
				PossessionType: r.PossessionType,
				PossessionId:   r.PossessionId,
				Count:          r.Count,
			}
		}
	}
	
	return CageOrnamentReward{
		PossessionType: rewards[len(rewards)-1].PossessionType,
		PossessionId:   rewards[len(rewards)-1].PossessionId,
		Count:          rewards[len(rewards)-1].Count,
	}
}

func LostItemRewardsConfig() []LostItemRewardConfig {
	return []LostItemRewardConfig{
		{PossessionType: 12, PossessionId: 0, Count: 100, Weight: 25000},
		{PossessionType: 12, PossessionId: 0, Count: 250, Weight: 10000},
		{PossessionType: 12, PossessionId: 0, Count: 500, Weight: 500},
		{PossessionType: 12, PossessionId: 0, Count: 1000, Weight: 1},
	}
}

func LoadCageOrnamentCatalog() *CageOrnamentCatalog {
	ornaments, err := utils.ReadTable[EntityMCageOrnament]("m_cage_ornament")
	if err != nil {
		log.Fatalf("load cage ornament table: %v", err)
	}
	rewards, err := utils.ReadTable[EntityMCageOrnamentReward]("m_cage_ornament_reward")
	if err != nil {
		log.Fatalf("load cage ornament reward table: %v", err)
	}

	cat := &CageOrnamentCatalog{
		ornamentToRewardId: make(map[int32]int32, len(ornaments)),
		rewards:            make(map[int32]CageOrnamentReward, len(rewards)),
		rewardPool:         make([]CageOrnamentReward, 0, len(rewards)),
	}
	
	for _, o := range ornaments {
		cat.ornamentToRewardId[o.CageOrnamentId] = o.CageOrnamentRewardId
	}
	
	for _, r := range rewards {
		reward := CageOrnamentReward{
			PossessionType: r.PossessionType,
			PossessionId:   r.PossessionId,
			Count:          r.Count,
		}
		cat.rewards[r.CageOrnamentRewardId] = reward
		cat.rewardPool = append(cat.rewardPool, reward)
	}
	log.Printf("[CageOrnament] Loaded %d cage ornament rewards", len(cat.rewardPool))
	return cat
}

func LoadLostItemsCatalog() *LostItemsCatalog {
	lostItemRewards := LostItemRewardsConfig()
	
	gimmicks, ok := readGimmickTable[EntityMGimmick]("m_gimmick", "lost-items ornaments")
	ornaments, ok := readGimmickTable[EntityMGimmickOrnament]("m_gimmick_ornament", "lost-items ornaments")
	
	ornamentsByRef := make(map[GimmickOrnamentRef]*EntityMGimmickOrnament)
	if ok && len(gimmicks) > 0 && len(ornaments) > 0 {
		ornamentGroupByGimmick := make(map[int32]int32)
		for _, g := range gimmicks {
			if model.GimmickType(g.GimmickType) == model.GimmickTypeCageIntervalDropItem {
				ornamentGroupByGimmick[g.GimmickId] = g.GimmickOrnamentGroupId
			}
		}
		
		for i, o := range ornaments {
			for gimmickId, groupId := range ornamentGroupByGimmick {
				if groupId == o.GimmickOrnamentGroupId {
					ornamentsByRef[GimmickOrnamentRef{GimmickId: gimmickId, OrnamentIndex: o.GimmickOrnamentIndex}] = &ornaments[i]
				}
			}
		}
	}
	
	cat := &LostItemsCatalog{
		rewardPool: make([]CageOrnamentReward, 0, len(lostItemRewards)),
		ornaments:  ornamentsByRef,
	}
	
	for _, r := range lostItemRewards {
		reward := CageOrnamentReward{
			PossessionType: r.PossessionType,
			PossessionId:   r.PossessionId,
			Count:          r.Count,
		}
		cat.rewardPool = append(cat.rewardPool, reward)
	}
	log.Printf("[LostItems] Loaded %d Lost Items rewards with weights, %d ornament mappings", len(cat.rewardPool), len(ornamentsByRef))
	return cat
}


