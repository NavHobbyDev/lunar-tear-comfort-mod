package masterdata

import "testing"

func TestCollectHighestReward(t *testing.T) {
	c := &BigHuntCatalog{
		ScoreRewardThresholds: map[int32][]ScoreRewardThreshold{
			1: {
				{NecessaryScore: 0, BigHuntRewardGroupId: 100},
				{NecessaryScore: 2000, BigHuntRewardGroupId: 101},
				{NecessaryScore: 4000, BigHuntRewardGroupId: 102},
			},
		},
		RewardItems: map[int32][]RewardItem{
			100: {{PossessionType: 6, PossessionId: 30, Count: 1}},
			101: {{PossessionType: 6, PossessionId: 30, Count: 1}, {PossessionType: 5, PossessionId: 1, Count: 2}},
			102: {{PossessionType: 6, PossessionId: 30, Count: 1}},
		},
	}

	// A score far above every threshold pays only the highest tier's reward,
	// never the sum of all tiers (the old 47-gems bug).
	got := c.CollectHighestReward(1, 999999)
	if len(got) != 1 || got[0].PossessionId != 30 || got[0].Count != 1 {
		t.Errorf("CollectHighestReward(top) = %+v, want the single tier-102 gem row", got)
	}

	// Exact boundary picks that tier.
	got = c.CollectHighestReward(1, 2000)
	if len(got) != 2 {
		t.Errorf("CollectHighestReward(2000) = %+v, want the two tier-101 rows", got)
	}

	// Between thresholds picks the last one crossed.
	got = c.CollectHighestReward(1, 3999)
	if len(got) != 2 {
		t.Errorf("CollectHighestReward(3999) = %+v, want tier-101 rows", got)
	}

	// Below the first threshold -> nothing.
	if got := c.CollectHighestReward(1, -1); got != nil {
		t.Errorf("CollectHighestReward(-1) = %+v, want nil", got)
	}

	// Unknown group -> nothing.
	if got := c.CollectHighestReward(999, 5000); got != nil {
		t.Errorf("CollectHighestReward(unknown group) = %+v, want nil", got)
	}
}
