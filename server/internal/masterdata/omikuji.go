package masterdata

import (
	"log"
	"lunar-tear/server/internal/utils"
)

type OmikujiCatalog struct {
	assetIds map[int32]int32
}

func (c *OmikujiCatalog) LookupAssetId(omikujiId int32) int32 {
	if id, ok := c.assetIds[omikujiId]; ok {
		return id
	}
	return 0
}

func LoadOmikujiCatalog() *OmikujiCatalog {
	entries, err := utils.ReadTable[EntityMOmikuji]("m_omikuji")
	if err != nil {
		log.Fatalf("load omikuji table: %v", err)
	}

	cat := &OmikujiCatalog{
		assetIds: make(map[int32]int32, len(entries)),
	}
	for _, e := range entries {
		cat.assetIds[e.OmikujiId] = e.OmikujiAssetId
	}
	return cat
}
