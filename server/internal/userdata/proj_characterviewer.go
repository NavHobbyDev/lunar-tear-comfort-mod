package userdata

import (
	//"sort"

	"lunar-tear/server/internal/store"
	//"lunar-tear/server/internal/utils"
)

func init() {
	register("IUserCharacterViewerField", func(user store.UserState) string {
		return "[]"  // client crashes on any real data for this table
	})
}

/* func init() {
	register("IUserCharacterViewerField", func(user store.UserState) string {
		records := make([]map[string]any, 0, len(user.CharacterViewerFields))
		
		// Sort by characterViewerFieldId for consistent output
		var fieldIds []int32
		for id := range user.CharacterViewerFields {
			fieldIds = append(fieldIds, id)
		}
		sort.Slice(fieldIds, func(i, j int) bool {
			return fieldIds[i] < fieldIds[j]
		})
		
		for _, id := range fieldIds {
			field := user.CharacterViewerFields[id]
			records = append(records, map[string]any{
				"userId":                 user.UserId,
				"characterViewerFieldId": field.CharacterViewerFieldId,
				"isViewed":               field.IsViewed,
				"viewedDatetime":         field.ViewedDatetime,
				"latestVersion":          field.LatestVersion,
			})
		}
		
		if len(records) == 0 {
			return ""
		}
		
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
} */
