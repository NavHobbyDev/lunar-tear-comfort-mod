package masterdata

import (
	"log"
	"sort"

	"lunar-tear/server/internal/utils"
)

type WebviewPanelMissionCatalog struct {
	PageIds []int32 // every WebviewPanelMissionPageId, sorted ascending
}

func LoadWebviewPanelMissionCatalog() *WebviewPanelMissionCatalog {
	rows, err := utils.ReadTable[EntityMWebviewPanelMissionPage]("m_webview_panel_mission_page")
	if err != nil {
		log.Printf("load webview panel mission page table: %v", err)
		return &WebviewPanelMissionCatalog{}
	}
	ids := make([]int32, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.WebviewPanelMissionPageId)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return &WebviewPanelMissionCatalog{PageIds: ids}
}
