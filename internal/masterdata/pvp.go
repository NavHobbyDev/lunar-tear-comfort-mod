package masterdata

import (
	"log"
	"sort"

	"lunar-tear/server/internal/utils"
)

// PvpGrade is one rung of the arena ladder within a grade group: reaching
// NecessaryPvpPoint promotes the player to this grade, which selects the
// per-match and weekly reward groups.
type PvpGrade struct {
	GradeId               int32
	NecessaryPvpPoint     int32
	WeeklyRewardGroupId   int32
	OneMatchRewardGroupId int32
}

// pvpOneMatchChoice is one weighted outcome of a per-match reward roll.
type pvpOneMatchChoice struct {
	OneMatchRewardId int32
	Weight           int32
}

// PvpSeason is a competitive season window. Live data has 38 valid past
// seasons (all ending far in the future) plus a few invalid/placeholder ones.
// The server always pays rewards from the newest valid season's tables
// (LatestSeason), ignoring the official date windows.
type PvpSeason struct {
	SeasonId     int32
	StartMillis  int64
	EndMillis    int64
	Invalid      bool
	GradeGroupId int32
	// WeeklyRankRewardRankGroupId selects the rank-range ladder used by the
	// client's "Weekly Ranking Rewards" tab (0 on old seasons).
	WeeklyRankRewardRankGroupId int32
	// SeasonRankRewardRankGroupId selects the rank-range ladder used by the
	// client's "Season Ranking Rewards" tab.
	SeasonRankRewardRankGroupId int32
}

// rankTier is one rung of a rank-range reward ladder: everyone ranked at or
// above (numerically <=) RankLowerLimit down to the next tier gets GroupId.
type rankTier struct {
	RankLowerLimit int32
	GroupId        int32
}

// PvpCatalog holds everything FinishBattle and the weekly reward flow need:
// the ladder (grades per group), the per-match and weekly reward tables, the
// reward item definitions, and the season list.
type PvpCatalog struct {
	// gradesByGroup maps PvpGradeGroupId -> grades sorted ascending by
	// NecessaryPvpPoint, so a linear scan resolves the grade for any rating.
	gradesByGroup map[int32][]PvpGrade
	// oneMatchGroupChoices maps PvpGradeOneMatchRewardGroupId -> weighted
	// list of PvpGradeOneMatchRewardId outcomes.
	oneMatchGroupChoices map[int32][]pvpOneMatchChoice
	// oneMatchRewards maps PvpGradeOneMatchRewardId -> its reward items
	// (already resolved from m_pvp_reward, ordered by SortOrder).
	oneMatchRewards map[int32][]RewardItem
	// weeklyRewards maps PvpGradeWeeklyRewardGroupId -> its reward items.
	weeklyRewards map[int32][]RewardItem
	// weeklyRankRankGroups maps PvpWeeklyRankRewardRankGroupId -> rank tiers
	// ("Weekly Ranking Rewards" tab), sorted ascending by RankLowerLimit.
	weeklyRankRankGroups map[int32][]rankTier
	// weeklyRankRewards maps PvpWeeklyRankRewardGroupId -> its reward items.
	weeklyRankRewards map[int32][]RewardItem
	// seasonRankRankGroups maps PvpSeasonRankRewardRankGroupId -> rank tiers
	// ("Season Ranking Rewards" tab), sorted ascending by RankLowerLimit.
	seasonRankRankGroups map[int32][]rankTier
	// seasonRankRewards maps PvpSeasonRankRewardGroupId -> its reward items.
	seasonRankRewards map[int32][]RewardItem

	seasons      []PvpSeason
	validSeasons []PvpSeason // !Invalid, sorted ascending by SeasonId
}

// GradeForPoint returns the highest grade in the group whose NecessaryPvpPoint
// is not above point. Falls back to the lowest grade if none match.
func (c *PvpCatalog) GradeForPoint(gradeGroupId, point int32) (PvpGrade, bool) {
	grades := c.gradesByGroup[gradeGroupId]
	if len(grades) == 0 {
		return PvpGrade{}, false
	}
	chosen := grades[0]
	for _, g := range grades {
		if point >= g.NecessaryPvpPoint {
			chosen = g
		} else {
			break
		}
	}
	return chosen, true
}

// RollOneMatchReward picks one PvpGradeOneMatchRewardId from a one-match reward
// group by weight and returns it along with the reward items it grants. rollFn
// must return a value in [0,total). Returns (0, nil) if the group is unknown.
func (c *PvpCatalog) RollOneMatchReward(oneMatchRewardGroupId int32, rollFn func(total int32) int32) (int32, []RewardItem) {
	choices := c.oneMatchGroupChoices[oneMatchRewardGroupId]
	if len(choices) == 0 {
		return 0, nil
	}
	var total int32
	for _, ch := range choices {
		total += ch.Weight
	}
	if total <= 0 {
		// Degenerate weights: fall back to the first choice.
		id := choices[0].OneMatchRewardId
		return id, c.oneMatchRewards[id]
	}
	r := rollFn(total)
	for _, ch := range choices {
		r -= ch.Weight
		if r < 0 {
			return ch.OneMatchRewardId, c.oneMatchRewards[ch.OneMatchRewardId]
		}
	}
	last := choices[len(choices)-1].OneMatchRewardId
	return last, c.oneMatchRewards[last]
}

// WeeklyRewards returns the reward items for a weekly reward group (empty for
// the group-0 sentinel used by the lowest grade).
func (c *PvpCatalog) WeeklyRewards(weeklyRewardGroupId int32) []RewardItem {
	return c.weeklyRewards[weeklyRewardGroupId]
}

// WeeklyRankRewardsForRank returns the "Weekly Ranking Rewards" items for a
// ladder position (empty when the rank falls below every tier).
func (c *PvpCatalog) WeeklyRankRewardsForRank(rankGroupId int32, rank int) []RewardItem {
	return rankTierRewards(c.weeklyRankRankGroups, c.weeklyRankRewards, rankGroupId, rank)
}

// SeasonRankRewardsForRank returns the "Season Ranking Rewards" items for a
// ladder position (empty when the rank falls below every tier).
func (c *PvpCatalog) SeasonRankRewardsForRank(rankGroupId int32, rank int) []RewardItem {
	return rankTierRewards(c.seasonRankRankGroups, c.seasonRankRewards, rankGroupId, rank)
}

// rankTierRewards resolves the reward group for a ladder position: the tier
// with the highest RankLowerLimit not above rank. A missing/zero rankGroupId
// (old seasons reference group 0) falls back to the lowest known ladder so
// every rotated season still pays ranking rewards.
func rankTierRewards(tiersByGroup map[int32][]rankTier, rewards map[int32][]RewardItem, rankGroupId int32, rank int) []RewardItem {
	tiers, ok := tiersByGroup[rankGroupId]
	if !ok {
		var fallback int32
		for gid := range tiersByGroup {
			if fallback == 0 || gid < fallback {
				fallback = gid
			}
		}
		if fallback == 0 {
			return nil
		}
		tiers = tiersByGroup[fallback]
	}
	var groupId int32
	for _, t := range tiers {
		if int32(rank) >= t.RankLowerLimit {
			groupId = t.GroupId
		} else {
			break
		}
	}
	if groupId == 0 {
		return nil
	}
	return rewards[groupId]
}

// SeasonAt returns the valid season at the given server rotation index,
// wrapping around the valid-season list so the weekly-advancing counter
// never runs out of seasons.
func (c *PvpCatalog) SeasonAt(index int) (PvpSeason, bool) {
	n := len(c.validSeasons)
	if n == 0 {
		return PvpSeason{}, false
	}
	index %= n
	if index < 0 {
		index += n
	}
	return c.validSeasons[index], true
}

// LatestSeason returns the newest valid season by masterdata id (the game's
// "current" season, e.g. 37). All arena rewards are resolved from this
// season's tables: the private server deliberately ignores both the official
// date windows and the per-user rotation index.
func (c *PvpCatalog) LatestSeason() (PvpSeason, bool) {
	if len(c.validSeasons) == 0 {
		return PvpSeason{}, false
	}
	return c.validSeasons[len(c.validSeasons)-1], true
}

// CurrentSeason resolves the season active at nowMillis: the highest-id valid
// season whose window contains now. If none is active it returns the highest-id
// valid season that has already started, so the client always sees a real id.
func (c *PvpCatalog) CurrentSeason(nowMillis int64) (PvpSeason, bool) {
	var active, started PvpSeason
	var haveActive, haveStarted bool
	for _, s := range c.seasons {
		if s.Invalid {
			continue
		}
		if s.StartMillis <= nowMillis && nowMillis <= s.EndMillis {
			if !haveActive || s.SeasonId > active.SeasonId {
				active, haveActive = s, true
			}
		}
		if s.StartMillis <= nowMillis {
			if !haveStarted || s.SeasonId > started.SeasonId {
				started, haveStarted = s, true
			}
		}
	}
	if haveActive {
		return active, true
	}
	if haveStarted {
		return started, true
	}
	return PvpSeason{}, false
}

func LoadPvpCatalog() *PvpCatalog {
	rewardRows, err := utils.ReadTable[EntityMPvpReward]("m_pvp_reward")
	if err != nil {
		log.Fatalf("load m_pvp_reward: %v", err)
	}
	rewardById := make(map[int32]RewardItem, len(rewardRows))
	for _, r := range rewardRows {
		rewardById[r.PvpRewardId] = RewardItem{
			PossessionType: r.PossessionType,
			PossessionId:   r.PossessionId,
			Count:          r.Count,
		}
	}

	// PvpGradeOneMatchRewardId -> resolved reward items, ordered by SortOrder.
	omrRows, err := utils.ReadTable[EntityMPvpGradeOneMatchReward]("m_pvp_grade_one_match_reward")
	if err != nil {
		log.Fatalf("load m_pvp_grade_one_match_reward: %v", err)
	}
	sort.SliceStable(omrRows, func(i, j int) bool { return omrRows[i].SortOrder < omrRows[j].SortOrder })
	oneMatchRewards := make(map[int32][]RewardItem)
	for _, r := range omrRows {
		if item, ok := rewardById[r.PvpRewardId]; ok {
			oneMatchRewards[r.PvpGradeOneMatchRewardId] = append(oneMatchRewards[r.PvpGradeOneMatchRewardId], item)
		}
	}

	// PvpGradeOneMatchRewardGroupId -> weighted list of one-match reward ids.
	omrgRows, err := utils.ReadTable[EntityMPvpGradeOneMatchRewardGroup]("m_pvp_grade_one_match_reward_group")
	if err != nil {
		log.Fatalf("load m_pvp_grade_one_match_reward_group: %v", err)
	}
	oneMatchGroupChoices := make(map[int32][]pvpOneMatchChoice)
	for _, r := range omrgRows {
		oneMatchGroupChoices[r.PvpGradeOneMatchRewardGroupId] = append(oneMatchGroupChoices[r.PvpGradeOneMatchRewardGroupId], pvpOneMatchChoice{
			OneMatchRewardId: r.PvpGradeOneMatchRewardId,
			Weight:           r.Weight,
		})
	}

	// PvpGradeWeeklyRewardGroupId -> resolved reward items, ordered by SortOrder.
	weeklyRows, err := utils.ReadTable[EntityMPvpGradeWeeklyRewardGroup]("m_pvp_grade_weekly_reward_group")
	if err != nil {
		log.Fatalf("load m_pvp_grade_weekly_reward_group: %v", err)
	}
	sort.SliceStable(weeklyRows, func(i, j int) bool { return weeklyRows[i].SortOrder < weeklyRows[j].SortOrder })
	weeklyRewards := make(map[int32][]RewardItem)
	for _, r := range weeklyRows {
		if item, ok := rewardById[r.PvpRewardId]; ok {
			weeklyRewards[r.PvpGradeWeeklyRewardGroupId] = append(weeklyRewards[r.PvpGradeWeeklyRewardGroupId], item)
		}
	}

	// PvpGradeGroupId -> grades sorted ascending by required points.
	gradeGroupRows, err := utils.ReadTable[EntityMPvpGradeGroup]("m_pvp_grade_group")
	if err != nil {
		log.Fatalf("load m_pvp_grade_group: %v", err)
	}
	gradesByGroup := make(map[int32][]PvpGrade)
	for _, r := range gradeGroupRows {
		gradesByGroup[r.PvpGradeGroupId] = append(gradesByGroup[r.PvpGradeGroupId], PvpGrade{
			GradeId:               r.PvpGradeId,
			NecessaryPvpPoint:     r.NecessaryPvpPoint,
			WeeklyRewardGroupId:   r.PvpGradeWeeklyRewardGroupId,
			OneMatchRewardGroupId: r.PvpGradeOneMatchRewardGroupId,
		})
	}
	for _, grades := range gradesByGroup {
		sort.SliceStable(grades, func(i, j int) bool { return grades[i].NecessaryPvpPoint < grades[j].NecessaryPvpPoint })
	}

	// PvpWeeklyRankRewardRankGroupId -> rank tiers for weekly ranking rewards.
	wrrgRows, err := utils.ReadTable[EntityMPvpWeeklyRankRewardRankGroup]("m_pvp_weekly_rank_reward_rank_group")
	if err != nil {
		log.Fatalf("load m_pvp_weekly_rank_reward_rank_group: %v", err)
	}
	weeklyRankRankGroups := make(map[int32][]rankTier)
	for _, r := range wrrgRows {
		weeklyRankRankGroups[r.PvpWeeklyRankRewardRankGroupId] = append(weeklyRankRankGroups[r.PvpWeeklyRankRewardRankGroupId], rankTier{
			RankLowerLimit: r.RankLowerLimit,
			GroupId:        r.PvpWeeklyRankRewardGroupId,
		})
	}

	// PvpWeeklyRankRewardGroupId -> resolved reward items, ordered by SortOrder.
	wrRows, err := utils.ReadTable[EntityMPvpWeeklyRankRewardGroup]("m_pvp_weekly_rank_reward_group")
	if err != nil {
		log.Fatalf("load m_pvp_weekly_rank_reward_group: %v", err)
	}
	sort.SliceStable(wrRows, func(i, j int) bool { return wrRows[i].SortOrder < wrRows[j].SortOrder })
	weeklyRankRewards := make(map[int32][]RewardItem)
	for _, r := range wrRows {
		if item, ok := rewardById[r.PvpRewardId]; ok {
			weeklyRankRewards[r.PvpWeeklyRankRewardGroupId] = append(weeklyRankRewards[r.PvpWeeklyRankRewardGroupId], item)
		}
	}

	// PvpSeasonRankRewardRankGroupId -> rank tiers for season ranking rewards.
	srrgRows, err := utils.ReadTable[EntityMPvpSeasonRankRewardRankGroup]("m_pvp_season_rank_reward_rank_group")
	if err != nil {
		log.Fatalf("load m_pvp_season_rank_reward_rank_group: %v", err)
	}
	seasonRankRankGroups := make(map[int32][]rankTier)
	for _, r := range srrgRows {
		seasonRankRankGroups[r.PvpSeasonRankRewardRankGroupId] = append(seasonRankRankGroups[r.PvpSeasonRankRewardRankGroupId], rankTier{
			RankLowerLimit: r.RankLowerLimit,
			GroupId:        r.PvpSeasonRankRewardGroupId,
		})
	}

	// PvpSeasonRankRewardGroupId -> resolved reward items, ordered by SortOrder.
	srRows, err := utils.ReadTable[EntityMPvpSeasonRankRewardGroup]("m_pvp_season_rank_reward_group")
	if err != nil {
		log.Fatalf("load m_pvp_season_rank_reward_group: %v", err)
	}
	sort.SliceStable(srRows, func(i, j int) bool { return srRows[i].SortOrder < srRows[j].SortOrder })
	seasonRankRewards := make(map[int32][]RewardItem)
	for _, r := range srRows {
		if item, ok := rewardById[r.PvpRewardId]; ok {
			seasonRankRewards[r.PvpSeasonRankRewardGroupId] = append(seasonRankRewards[r.PvpSeasonRankRewardGroupId], item)
		}
	}

	seasonRows, err := utils.ReadTable[EntityMPvpSeason]("m_pvp_season")
	if err != nil {
		log.Fatalf("load m_pvp_season: %v", err)
	}
	seasons := make([]PvpSeason, 0, len(seasonRows))
	for _, s := range seasonRows {
		seasons = append(seasons, PvpSeason{
			SeasonId:                    s.PvpSeasonId,
			StartMillis:                 s.SeasonStartDatetime,
			EndMillis:                   s.SeasonEndDatetime,
			Invalid:                     s.IsInvalid,
			GradeGroupId:                s.PvpGradeGroupId,
			WeeklyRankRewardRankGroupId: s.PvpWeeklyRankRewardRankGroupId,
			SeasonRankRewardRankGroupId: s.PvpSeasonRankRewardRankGroupId,
		})
	}
	validSeasons := make([]PvpSeason, 0, len(seasons))
	for _, s := range seasons {
		if !s.Invalid {
			validSeasons = append(validSeasons, s)
		}
	}
	sort.SliceStable(validSeasons, func(i, j int) bool { return validSeasons[i].SeasonId < validSeasons[j].SeasonId })
	for _, tiers := range weeklyRankRankGroups {
		sort.SliceStable(tiers, func(i, j int) bool { return tiers[i].RankLowerLimit < tiers[j].RankLowerLimit })
	}
	for _, tiers := range seasonRankRankGroups {
		sort.SliceStable(tiers, func(i, j int) bool { return tiers[i].RankLowerLimit < tiers[j].RankLowerLimit })
	}

	log.Printf("pvp catalog loaded: %d grade groups, %d one-match groups, %d weekly groups, %d rewards, %d seasons (%d valid)",
		len(gradesByGroup), len(oneMatchGroupChoices), len(weeklyRewards), len(rewardById), len(seasons), len(validSeasons))

	return &PvpCatalog{
		gradesByGroup:        gradesByGroup,
		oneMatchGroupChoices: oneMatchGroupChoices,
		oneMatchRewards:      oneMatchRewards,
		weeklyRewards:        weeklyRewards,
		weeklyRankRankGroups: weeklyRankRankGroups,
		weeklyRankRewards:    weeklyRankRewards,
		seasonRankRankGroups: seasonRankRankGroups,
		seasonRankRewards:    seasonRankRewards,
		seasons:              seasons,
		validSeasons:         validSeasons,
	}
}
