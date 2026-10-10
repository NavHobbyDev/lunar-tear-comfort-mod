package userdata

import (
	"sort"

	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

func init() {
	register("IUserBigHuntProgressStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                    user.UserId,
			"currentBigHuntBossQuestId": user.BigHuntProgress.CurrentBigHuntBossQuestId,
			"currentBigHuntQuestId":     user.BigHuntProgress.CurrentBigHuntQuestId,
			"currentQuestSceneId":       user.BigHuntProgress.CurrentQuestSceneId,
			"isDryRun":                  user.BigHuntProgress.IsDryRun,
			"latestVersion":             user.BigHuntProgress.LatestVersion,
		})
		return s
	})

	register("IUserBigHuntMaxScore", func(user store.UserState) string {
		if len(user.BigHuntMaxScores) == 0 {
			return "[]"
		}
		ids := make([]int, 0, len(user.BigHuntMaxScores))
		for id := range user.BigHuntMaxScores {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		records := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			ms := user.BigHuntMaxScores[int32(id)]
			records = append(records, map[string]any{
				"userId":                 user.UserId,
				"bigHuntBossId":          int32(id),
				"maxScore":               ms.MaxScore,
				"maxScoreUpdateDatetime": ms.MaxScoreUpdateDatetime,
				"latestVersion":          ms.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})

	register("IUserBigHuntStatus", func(user store.UserState) string {
		if len(user.BigHuntStatuses) == 0 {
			return "[]"
		}
		ids := make([]int, 0, len(user.BigHuntStatuses))
		for id := range user.BigHuntStatuses {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		records := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			st := user.BigHuntStatuses[int32(id)]
			records = append(records, map[string]any{
				"userId":                            user.UserId,
				"bigHuntBossQuestId":                int32(id),
				"dailyChallengeCount":               st.DailyChallengeCount,
				"latestChallengeDatetime":           st.LatestChallengeDatetime,
				"lastDailyRewardReceivedDayVersion": st.LastDailyRewardReceivedDayVersion,
				"latestVersion":                     st.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})

	register("IUserBigHuntScheduleMaxScore", func(user store.UserState) string {
		if len(user.BigHuntScheduleMaxScores) == 0 {
			return "[]"
		}
		type sortableKey struct {
			ScheduleId int32
			BossId     int32
		}
		keys := make([]sortableKey, 0, len(user.BigHuntScheduleMaxScores))
		for k := range user.BigHuntScheduleMaxScores {
			keys = append(keys, sortableKey{k.BigHuntScheduleId, k.BigHuntBossId})
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].ScheduleId != keys[j].ScheduleId {
				return keys[i].ScheduleId < keys[j].ScheduleId
			}
			return keys[i].BossId < keys[j].BossId
		})
		records := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			ms := user.BigHuntScheduleMaxScores[store.BigHuntScheduleScoreKey{BigHuntScheduleId: k.ScheduleId, BigHuntBossId: k.BossId}]
			records = append(records, map[string]any{
				"userId":                 user.UserId,
				"bigHuntScheduleId":      k.ScheduleId,
				"bigHuntBossId":          k.BossId,
				"maxScore":               ms.MaxScore,
				"maxScoreUpdateDatetime": ms.MaxScoreUpdateDatetime,
				"latestVersion":          ms.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})

	register("IUserBigHuntWeeklyMaxScore", func(user store.UserState) string {
		if len(user.BigHuntWeeklyMaxScores) == 0 {
			return "[]"
		}
		type sortableKey struct {
			WeeklyVersion int64
			AttributeType int32
		}
		keys := make([]sortableKey, 0, len(user.BigHuntWeeklyMaxScores))
		for k := range user.BigHuntWeeklyMaxScores {
			keys = append(keys, sortableKey{k.BigHuntWeeklyVersion, k.AttributeType})
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].WeeklyVersion != keys[j].WeeklyVersion {
				return keys[i].WeeklyVersion < keys[j].WeeklyVersion
			}
			return keys[i].AttributeType < keys[j].AttributeType
		})
		records := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			ms := user.BigHuntWeeklyMaxScores[store.BigHuntWeeklyScoreKey{BigHuntWeeklyVersion: k.WeeklyVersion, AttributeType: k.AttributeType}]
			records = append(records, map[string]any{
				"userId":               user.UserId,
				"bigHuntWeeklyVersion": k.WeeklyVersion,
				"attributeType":        k.AttributeType,
				"maxScore":             ms.MaxScore,
				"latestVersion":        ms.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})

	register("IUserBigHuntWeeklyStatus", func(user store.UserState) string {
		if len(user.BigHuntWeeklyStatuses) == 0 {
			return "[]"
		}
		versions := make([]int64, 0, len(user.BigHuntWeeklyStatuses))
		for v := range user.BigHuntWeeklyStatuses {
			versions = append(versions, v)
		}
		sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
		records := make([]map[string]any, 0, len(versions))
		for _, v := range versions {
			ws := user.BigHuntWeeklyStatuses[v]
			records = append(records, map[string]any{
				"userId":                 user.UserId,
				"bigHuntWeeklyVersion":   v,
				"isReceivedWeeklyReward": ws.IsReceivedWeeklyReward,
				"latestVersion":          ws.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
}
