package masterdata

import (
	"testing"

	"lunar-tear/server/internal/model"
)

// Extra weapons registered via AddExtraGachaWeapons are force-included into
// overridden banners even though they are absent from WeaponById (quest
// granted / restricted), but only when their weapon type matches the
// banner's characters.
func TestOverrideBannerIncludesExtraWeapons(t *testing.T) {
	const (
		faithWeapon   = int32(320021) // one-handed sword (type 1) — matches
		gunWeapon     = int32(500031) // gun (type 6) — must be filtered out
		regularWeapon = int32(110051) // one-handed sword (type 1)
	)
	pool := &GachaCatalog{
		CostumeById: map[int32]GachaPoolItem{
			22009: {PossessionType: int32(model.PossessionTypeCostume), PossessionId: 22009, RarityType: model.RaritySRare, CharacterId: 1025},
		},
		WeaponById: map[int32]GachaPoolItem{
			regularWeapon: {PossessionType: int32(model.PossessionTypeWeapon), PossessionId: regularWeapon, RarityType: model.RarityRare},
		},
		SkillfulWeaponTypeByCostume: map[int32]int32{22009: 1},
		WeaponTypeById:              map[int32]int32{regularWeapon: 1},
		WeaponMasterById: map[int32]EntityMWeapon{
			faithWeapon: {WeaponId: faithWeapon, WeaponType: 1, RarityType: model.RaritySSRare, IsRestrictDiscard: false},
			gunWeapon:   {WeaponId: gunWeapon, WeaponType: 6, RarityType: model.RaritySSRare, IsRestrictDiscard: true},
		},
		BannerPools:     map[int32]*BannerPool{},
		FeaturedByGacha: map[int32]FeaturedSet{},
	}
	pool.AddExtraGachaWeapons([]int32{45}, []int32{faithWeapon, gunWeapon})
	pool.OverrideBannerForCharacters(45, []int32{1025}, nil, nil)

	banner := pool.BannerPools[45]
	if banner == nil {
		t.Fatal("banner 45 pool not built")
	}

	found := map[int32]bool{}
	for _, items := range banner.WeaponsByRarity {
		for _, item := range items {
			found[item.PossessionId] = true
		}
	}
	if !found[regularWeapon] {
		t.Error("regular weapon missing from overridden banner")
	}
	if !found[faithWeapon] {
		t.Error("extra weapon with matching type was not force-included")
	}
	if found[gunWeapon] {
		t.Error("extra weapon with non-matching type must be filtered out")
	}

	featured := pool.FeaturedByGacha[45]
	featuredFaith := false
	for _, w := range featured.Weapons {
		if w.PossessionId == faithWeapon {
			featuredFaith = true
		}
	}
	if !featuredFaith {
		t.Error("force-included weapon missing from banner featured set")
	}

	// Re-registering the same weapon must not duplicate it.
	pool.AddExtraGachaWeapons([]int32{45}, []int32{faithWeapon})
	pool.OverrideBannerForCharacters(45, []int32{1025}, nil, nil)
	count := 0
	for _, items := range pool.BannerPools[45].WeaponsByRarity {
		for _, item := range items {
			if item.PossessionId == faithWeapon {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("extra weapon duplicated in pool: %d copies", count)
	}
}
