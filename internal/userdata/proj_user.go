package userdata

import (
	"sort"
	"sync"
	//"time"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

func init() {
	register("IUser", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":              user.UserId,
			"playerId":            user.PlayerId,
			"osType":              user.OsType,
			"platformType":        user.PlatformType,
			"userRestrictionType": user.UserRestrictionType,
			"registerDatetime":    user.RegisterDatetime,
			"gameStartDatetime":   user.GameStartDatetime,
			"latestVersion":       user.LatestVersion,
		})
		return s
	})
	register("IUserSetting", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                user.UserId,
			"isNotifyPurchaseAlert": user.Setting.IsNotifyPurchaseAlert,
			"latestVersion":         user.Setting.LatestVersion,
		})
		return s
	})
	register("IUserStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                user.UserId,
			"level":                 user.Status.Level,
			"exp":                   user.Status.Exp,
			"staminaMilliValue":     user.Status.StaminaMilliValue,
			"staminaUpdateDatetime": user.Status.StaminaUpdateDatetime,
			"latestVersion":         user.Status.LatestVersion,
		})
		return s
	})
	register("IUserGem", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":        user.UserId,
			"paidGem":       user.Gem.PaidGem,
			"freeGem":       user.Gem.FreeGem,
			"latestVersion": gametime.NowMillis(),
		})
		return s
	})
	register("IUserProfile", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                          user.UserId,
			"name":                            user.Profile.Name,
			"nameUpdateDatetime":              user.Profile.NameUpdateDatetime,
			"message":                         user.Profile.Message,
			"messageUpdateDatetime":           user.Profile.MessageUpdateDatetime,
			"favoriteCostumeId":               user.Profile.FavoriteCostumeId,
			"favoriteCostumeIdUpdateDatetime": user.Profile.FavoriteCostumeIdUpdateDatetime,
			"latestVersion":                   user.Profile.LatestVersion,
		})
		return s
	})
	register("IUserLogin", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                    user.UserId,
			"totalLoginCount":           user.Login.TotalLoginCount,
			"continualLoginCount":       user.Login.ContinualLoginCount,
			"maxContinualLoginCount":    user.Login.MaxContinualLoginCount,
			"lastLoginDatetime":         user.Login.LastLoginDatetime,
			"lastComebackLoginDatetime": user.Login.LastComebackLoginDatetime,
			"latestVersion":             user.Login.LatestVersion,
		})
		return s
	})
	register("IUserLoginBonus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                      user.UserId,
			"loginBonusId":                user.LoginBonus.LoginBonusId,
			"currentPageNumber":           user.LoginBonus.CurrentPageNumber,
			"currentStampNumber":          user.LoginBonus.CurrentStampNumber,
			"latestRewardReceiveDatetime": user.LoginBonus.LatestRewardReceiveDatetime,
			"latestVersion":               user.LoginBonus.LatestVersion,
		})
		return s
	})
	register("IUserTutorialProgress", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedTutorialRecords(user)...)
		return s
	})
	register("IUserMission", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedMissionRecords(user)...)
		return s
	})
	register("IUserNaviCutIn", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedNaviCutInRecords(user)...)
		return s
	})
	register("IUserMovie", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedMovieRecords(user)...)
		return s
	})
	register("IUserContentsStory", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedContentsStoryRecords(user)...)
		return s
	})
	register("IUserOmikuji", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedOmikujiRecords(user)...)
		return s
	})
	register("IUserDokan", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedDokanRecords(user)...)
		return s
	})
	register("IUserPortalCageStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                user.UserId,
			"isCurrentProgress":     user.PortalCageStatus.IsCurrentProgress,
			"dropItemStartDatetime": user.PortalCageStatus.DropItemStartDatetime,
			"currentDropItemCount":  user.PortalCageStatus.CurrentDropItemCount,
			"latestVersion":         user.PortalCageStatus.LatestVersion,
		})
		return s
	})
	register("IUserEventQuestGuerrillaFreeOpen", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":           user.UserId,
			"startDatetime":    user.GuerrillaFreeOpen.StartDatetime,
			"openMinutes":      user.GuerrillaFreeOpen.OpenMinutes,
			"dailyOpenedCount": user.GuerrillaFreeOpen.DailyOpenedCount,
			"latestVersion":    user.GuerrillaFreeOpen.LatestVersion,
		})
		return s
	})

	register("IUserShopItem", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedShopItemRecords(user)...)
		return s
	})
	register("IUserShopReplaceable", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                     user.UserId,
			"lineupUpdateCount":          user.ShopReplaceable.LineupUpdateCount,
			"latestLineupUpdateDatetime": user.ShopReplaceable.LatestLineupUpdateDatetime,
			"latestVersion":              user.ShopReplaceable.LatestVersion,
		})
		return s
	})
	register("IUserShopReplaceableLineup", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedShopReplaceableLineupRecords(user)...)
		return s
	})

	register("IUserFacebook", func(user store.UserState) string {
		return ProjectFacebook(user.UserId, user.FacebookId)
	})
	registerStatic("IUserApple")
}

func ProjectFacebook(userId int64, facebookId int64) string {
	if facebookId == 0 {
		return "[]"
	}
	s, _ := utils.EncodeJSONMaps(map[string]any{
		"userId":        userId,
		"facebookId":    facebookId,
		"latestVersion": gametime.NowMillis(),
	})
	return s
}

func sortedTutorialRecords(user store.UserState) []map[string]any {
	ids := make([]int, 0, len(user.Tutorials))
	for id := range user.Tutorials {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		row := user.Tutorials[int32(id)]
		records = append(records, map[string]any{
			"userId":        user.UserId,
			"tutorialType":  row.TutorialType,
			"progressPhase": row.ProgressPhase,
			"choiceId":      row.ChoiceId,
			"latestVersion": row.LatestVersion,
		})
	}
	return records
}

var hiddenStoryRequirements = sync.OnceValue(masterdata.LoadHiddenStoryRequirements)

func sortedMissionRecords(user store.UserState) []map[string]any {
	missions := make(map[int32]store.UserMissionState, len(user.Missions))
	for id, m := range user.Missions {
		missions[id] = m
	}
	settings := ResolveOptions(user.Options)
	if settings.ForceHiddenStoryMissions {
		for _, missionId := range hiddenStoryRequirements().MissionIds {
			if existing, ok := missions[missionId]; ok && existing.MissionProgressStatusType >= int32(model.MissionProgressStatusTypeClear) {
				continue
			}
			missions[missionId] = store.UserMissionState{
				MissionId:                 missionId,
				StartDatetime:             user.GameStartDatetime,
				MissionProgressStatusType: int32(model.MissionProgressStatusTypeClear),
				ClearDatetime:             user.GameStartDatetime,
				LatestVersion:             user.GameStartDatetime,
			}
		}
	}

	ids := make([]int, 0, len(missions))
	for id := range missions {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		row := missions[int32(id)]
		records = append(records, map[string]any{
			"userId":                    user.UserId,
			"missionId":                 row.MissionId,
			"startDatetime":             row.StartDatetime,
			"progressValue":             row.ProgressValue,
			"missionProgressStatusType": row.MissionProgressStatusType,
			"clearDatetime":             row.ClearDatetime,
			"latestVersion":             row.LatestVersion,
		})
	}
	return records
}

func sortedNaviCutInRecords(user store.UserState) []map[string]any {
	ids := make([]int32, 0, len(user.NaviCutInPlayed))
	for id := range user.NaviCutInPlayed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	now := gametime.NowMillis()
	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		records = append(records, map[string]any{
			"userId":        user.UserId,
			"naviCutInId":   id,
			"playDatetime":  now,
			"latestVersion": now,
		})
	}
	return records
}

func sortedContentsStoryRecords(user store.UserState) []map[string]any {
	ids := make([]int32, 0, len(user.ContentsStories))
	for id := range user.ContentsStories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	now := gametime.NowMillis()
	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		records = append(records, map[string]any{
			"userId":          user.UserId,
			"contentsStoryId": id,
			"playDatetime":    user.ContentsStories[id],
			"latestVersion":   now,
		})
	}
	return records
}

func sortedMovieRecords(user store.UserState) []map[string]any {
	ids := make([]int32, 0, len(user.ViewedMovies))
	for id := range user.ViewedMovies {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	now := gametime.NowMillis()
	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		records = append(records, map[string]any{
			"userId":               user.UserId,
			"movieId":              id,
			"latestViewedDatetime": user.ViewedMovies[id],
			"latestVersion":        now,
		})
	}
	return records
}

func sortedOmikujiRecords(user store.UserState) []map[string]any {
	ids := make([]int32, 0, len(user.DrawnOmikuji))
	for id := range user.DrawnOmikuji {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	now := gametime.NowMillis()
	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		records = append(records, map[string]any{
			"userId":             user.UserId,
			"omikujiId":          id,
			"latestDrawDatetime": user.DrawnOmikuji[id],
			"latestVersion":      now,
		})
	}
	return records
}

func sortedDokanRecords(user store.UserState) []map[string]any {
	idSet := make(map[int32]bool, len(user.DokanConfirmed))
	for id := range user.DokanConfirmed {
		idSet[id] = true
	}
	// When this player disabled news popups (octo /information page), report
	// currently-active dokans as already-seen in the projection only — nothing
	// is written to the database. The real DB record is made only when the
	// player actually views a news item (the client calls
	// RegisterDokanConfirmed at that point).
	if !user.Options.ShowNews && dokanCatalog != nil {
		now := gametime.NowMillis()
		for _, id := range dokanCatalog.ActiveDokanIds(now) {
			idSet[id] = true
		}
	}
	ids := make([]int32, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	now := gametime.NowMillis()
	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		records = append(records, map[string]any{
			"userId":          user.UserId,
			"dokanId":         id,
			"displayDatetime": now,
			"latestVersion":   now,
		})
	}
	return records
}

func sortedShopItemRecords(user store.UserState) []map[string]any {
	ids := make([]int, 0, len(user.ShopItems))
	for id := range user.ShopItems {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		row := user.ShopItems[int32(id)]
		records = append(records, map[string]any{
			"userId":                           user.UserId,
			"shopItemId":                       row.ShopItemId,
			"boughtCount":                      row.BoughtCount,
			"latestBoughtCountChangedDatetime": row.LatestBoughtCountChangedDatetime,
			"latestVersion":                    row.LatestVersion,
		})
	}
	return records
}

func sortedShopReplaceableLineupRecords(user store.UserState) []map[string]any {
	slots := make([]int, 0, len(user.ShopReplaceableLineup))
	for slot := range user.ShopReplaceableLineup {
		slots = append(slots, int(slot))
	}
	sort.Ints(slots)
	records := make([]map[string]any, 0, len(slots))
	for _, slot := range slots {
		row := user.ShopReplaceableLineup[int32(slot)]
		records = append(records, map[string]any{
			"userId":        user.UserId,
			"slotNumber":    row.SlotNumber,
			"shopItemId":    row.ShopItemId,
			"latestVersion": row.LatestVersion,
		})
	}
	return records
}
