package masterdata

import (
	"log"

	"lunar-tear/server/internal/utils"
)

// DokanCatalog holds the m_dokan rows needed to resolve which dokan (news)
// entries are currently active. The projection layer uses it to tell which
// news a player who disabled popups should be reported as having already seen
// (see userdata.sortedDokanRecords and the per-user show_news option).
type DokanCatalog struct {
	all []EntityMDokan
}

// ActiveDokanIds returns the DokanId of every dokan whose time window covers
// nowMillis. These are the entries the client would otherwise surface as
// unconfirmed news popups.
func (c *DokanCatalog) ActiveDokanIds(nowMillis int64) []int32 {
	var ids []int32
	for _, d := range c.all {
		if d.StartDatetime <= nowMillis && (d.EndDatetime == 0 || d.EndDatetime >= nowMillis) {
			ids = append(ids, d.DokanId)
		}
	}
	return ids
}

// LoadDokanCatalog reads m_dokan from master data. It is non-fatal on error:
// a missing table simply means the news option has nothing to hide.
func LoadDokanCatalog() *DokanCatalog {
	rows, err := utils.ReadTable[EntityMDokan]("m_dokan")
	if err != nil {
		log.Printf("[masterdata] load m_dokan: %v (news popups cannot be hidden)", err)
		return &DokanCatalog{}
	}
	log.Printf("[masterdata] dokan catalog loaded: %d entries", len(rows))
	return &DokanCatalog{all: rows}
}
