package masterdata

import (
	"fmt"
	"sort"

	"lunar-tear/server/internal/utils"
)

type ExploreCatalog struct {
	Explores    map[int32]EntityMExplore
	GradeScores map[int32][]EntityMExploreGradeScore // keyed by ExploreId, sorted desc by NecessaryScore
	GradeAssets map[int32]int32                      // gradeId -> assetGradeIconId
}

func LoadExploreCatalog() (*ExploreCatalog, error) {
	explores, err := utils.ReadTable[EntityMExplore]("m_explore")
	if err != nil {
		return nil, fmt.Errorf("load explore table: %w", err)
	}

	gradeScores, err := utils.ReadTable[EntityMExploreGradeScore]("m_explore_grade_score")
	if err != nil {
		return nil, fmt.Errorf("load explore grade score table: %w", err)
	}

	gradeAssets, err := utils.ReadTable[EntityMExploreGradeAsset]("m_explore_grade_asset")
	if err != nil {
		return nil, fmt.Errorf("load explore grade asset table: %w", err)
	}

	catalog := &ExploreCatalog{
		Explores:    make(map[int32]EntityMExplore, len(explores)),
		GradeScores: make(map[int32][]EntityMExploreGradeScore),
		GradeAssets: make(map[int32]int32, len(gradeAssets)),
	}

	for _, e := range explores {
		catalog.Explores[e.ExploreId] = e
	}

	for _, gs := range gradeScores {
		catalog.GradeScores[gs.ExploreId] = append(catalog.GradeScores[gs.ExploreId], gs)
	}
	for eid := range catalog.GradeScores {
		rows := catalog.GradeScores[eid]
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].NecessaryScore > rows[j].NecessaryScore
		})
		catalog.GradeScores[eid] = rows
	}

	for _, ga := range gradeAssets {
		catalog.GradeAssets[ga.ExploreGradeId] = ga.AssetGradeIconId
	}

	return catalog, nil
}

// GradeForScore returns the AssetGradeIconId for the given explore and score.
// Returns 0 if no matching grade is found.
func (c *ExploreCatalog) GradeForScore(exploreId, score int32) int32 {
	rows, ok := c.GradeScores[exploreId]
	if !ok {
		return 0
	}
	for _, r := range rows {
		if score >= r.NecessaryScore {
			return c.GradeAssets[r.ExploreGradeId]
		}
	}
	return 0
}
