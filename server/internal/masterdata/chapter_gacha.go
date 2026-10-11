package masterdata

import (
	"fmt"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

const trueDarkMemoryQuestChapterId int32 = 901

type GachaBoxItemEntry struct {
	PossessionType int32
	PossessionId   int32
	Count          int32
	MaxCount       int32
	Weight         int32
	IsFeatured     bool
}

// === "His Body, Rust" (gachaId=200001) ===
var chapter1Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301010, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301009, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330001, Count: 4, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330009, Count: 4, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "What Was Lost" (gachaId=200002) ===
var chapter2Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301002, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301001, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330005, Count: 4, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330013, Count: 4, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "Determined Hearts" / "Two Lonely Marionettes" (gachaId=200003) ===
var chapter3Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301012, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301011, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330002, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330010, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330017, Count: 4, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "This Violent, Fleeting World" (gachaId=200004) ===
var chapter4Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301004, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301003, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330006, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "The Distant Peak" (gachaId=200005) ===
var chapter5Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301006, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301005, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330014, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330018, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "Redemption: Black" (gachaId=200006) ===
var chapter6Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301008, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330003, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330011, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301007, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "priSOner" (gachaId=200007) ===
var chapter7Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301010, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330007, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330015, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301009, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330021, Count: 4, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "LIberator" (gachaId=200008) ===
var chapter8Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 301002, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330019, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330023, Count: 4, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301001, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330022, Count: 4, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "A Cold and Hollow Silence" (gachaId=200009) ===
var chapter9Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330004, Count: 4, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330012, Count: 4, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301012, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301011, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "The Reluctant Hero" (gachaId=200010) ===
var chapter10Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330008, Count: 4, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330016, Count: 4, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301004, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301003, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "Echoes in the Heart" (gachaId=200011) ===
var chapter11Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330020, Count: 4, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330024, Count: 4, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301006, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301005, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
}

// === "Redemption: White" (gachaId=200012) ===
var chapter12Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330004, Count: 8, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330008, Count: 8, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330012, Count: 8, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330016, Count: 8, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330020, Count: 8, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330024, Count: 8, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301008, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330003, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330007, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330011, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330015, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330019, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330023, Count: 8, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301007, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330002, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330006, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330010, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330014, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330018, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330022, Count: 8, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330001, Count: 8, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330005, Count: 8, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330009, Count: 8, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330013, Count: 8, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330017, Count: 8, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330021, Count: 8, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "The Dawn Binding Magick" (gachaId=200013) ===
var chapter13Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330004, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330012, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301002, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301010, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330003, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330011, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301001, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301009, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330002, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330010, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330001, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330009, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "Noontide Curse of the Rift" (gachaId=200014) ===
var chapter14Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330008, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330016, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301004, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301012, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330007, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330015, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301003, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301011, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330006, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330014, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330005, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330013, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "Twilight One Vow" (gachaId=200015) ===
var chapter15Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330020, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330024, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301006, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301008, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330019, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330023, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301005, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301007, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330018, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330022, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330017, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330021, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "The Gloaming Proof of Zero" (gachaId=200016) ===
var chapter16Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === "Eventide One Thousand Moons" (gachaId=200017) ===
var chapter17Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === "Daybreak One Sun" (gachaId=200018) ===
var chapter18Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === "The Gloaming Binding Magick" (gachaId=200019) ===
var chapter19Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330004, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330012, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301002, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301010, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330003, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330011, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301001, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301009, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330002, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330010, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330001, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330009, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "Eventide Curse of the Rift" (gachaId=200020) ===
var chapter20Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330008, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330016, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301004, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301012, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330007, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330015, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301003, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301011, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330006, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330014, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330005, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330013, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "Daybreak One Vow" (gachaId=200021) ===
var chapter21Items = []GachaBoxItemEntry{
	{PossessionType: 5, PossessionId: 330020, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330024, Count: 12, MaxCount: 0, Weight: 4000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301006, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301008, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330019, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330023, Count: 12, MaxCount: 0, Weight: 6000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301005, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 301007, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330018, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330022, Count: 12, MaxCount: 0, Weight: 8000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330017, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
	{PossessionType: 5, PossessionId: 330021, Count: 12, MaxCount: 0, Weight: 10000, IsFeatured: false},
}

// === "The Dawn Proof of Zero" (gachaId=200022) ===
var chapter22Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === "Noontide One Thousand Moons" (gachaId=200023) ===
var chapter23Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === "Twilight One Sun" (gachaId=200024) ===
var chapter24Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === CHAPTER 25 (gachaId=200025) ===
var chapter25Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === CHAPTER 26 (gachaId=200026) ===
var chapter26Items = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 252, Count: 1, MaxCount: 0, Weight: 1, IsFeatured: true},
	{PossessionType: 6, PossessionId: 242, Count: 1, MaxCount: 0, Weight: 10, IsFeatured: true},
	{PossessionType: 6, PossessionId: 9001, Count: 1, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 1, Count: 100, MaxCount: 0, Weight: 10000, IsFeatured: true},
}

// === TRUE DARK MEMORY / CHAPTER EX (gachaId=200000) ===
var chapterExItems = []GachaBoxItemEntry{
	{PossessionType: 6, PossessionId: 194, Count: 100, MaxCount: 0, Weight: 5, IsFeatured: true},
	{PossessionType: 6, PossessionId: 8, Count: 1000, MaxCount: 0, Weight: 25, IsFeatured: true},
	{PossessionType: 6, PossessionId: 8, Count: 500, MaxCount: 0, Weight: 50, IsFeatured: true},
	{PossessionType: 6, PossessionId: 8, Count: 200, MaxCount: 0, Weight: 100, IsFeatured: true},
	{PossessionType: 6, PossessionId: 8, Count: 100, MaxCount: 0, Weight: 250, IsFeatured: false},
	{PossessionType: 6, PossessionId: 8, Count: 50, MaxCount: 0, Weight: 500, IsFeatured: false},
	{PossessionType: 6, PossessionId: 8, Count: 20, MaxCount: 0, Weight: 1000, IsFeatured: false},
	{PossessionType: 6, PossessionId: 8, Count: 10, MaxCount: 0, Weight: 2500, IsFeatured: false},
	{PossessionType: 6, PossessionId: 8, Count: 5, MaxCount: 0, Weight: 5000, IsFeatured: false},
}

var chapterGachaItemConfigs = map[int32][]GachaBoxItemEntry{
	200000: chapterExItems,
	200001: chapter1Items,
	200002: chapter2Items,
	200003: chapter3Items,
	200004: chapter4Items,
	200005: chapter5Items,
	200006: chapter6Items,
	200007: chapter7Items,
	200008: chapter8Items,
	200009: chapter9Items,
	200010: chapter10Items,
	200011: chapter11Items,
	200012: chapter12Items,
	200013: chapter13Items,
	200014: chapter14Items,
	200015: chapter15Items,
	200016: chapter16Items,
	200017: chapter17Items,
	200018: chapter18Items,
	200019: chapter19Items,
	200020: chapter20Items,
	200021: chapter21Items,
	200022: chapter22Items,
	200023: chapter23Items,
	200024: chapter24Items,
	200025: chapter25Items,
	200026: chapter26Items,
}

type chapterGachaSpec struct {
	mainQuestChapterId int32
	ticketId           int32
}

var chapterGachaSpecs = []chapterGachaSpec{
	{2, 1008},
	{3, 1009},
	{4, 1010},
	{5, 1011},
	{6, 1012},
	{7, 1013},
	{9, 1014},
	{10, 1015},
	{11, 1016},
	{12, 1017},
	{13, 1018},
	{14, 1019},
	{17, 1020},
	{18, 1021},
	{19, 1022},
	{20, 1023},
	{21, 1024},
	{22, 1025},
	{25, 1026},
	{26, 1027},
	{27, 1028},
	{28, 1029},
	{29, 1030},
	{30, 1031},
}

func buildChapterGachaEntries() []store.GachaCatalogEntry {
	entries := make([]store.GachaCatalogEntry, 0, len(chapterGachaSpecs)+1)
	for i, spec := range chapterGachaSpecs {
		chapterNumber := int32(i + 1)
		gachaId := chapterGachaIdBase + chapterNumber
		entries = append(entries, store.GachaCatalogEntry{
			GachaId:                   gachaId,
			GachaLabelType:            model.GachaLabelChapter,
			GachaModeType:             model.GachaModeBox,
			GachaAutoResetType:        model.GachaAutoResetMonthly,
			GachaAutoResetPeriod:      1,
			IsUserGachaUnlock:         true,
			RelatedMainQuestChapterId: spec.mainQuestChapterId,
			GachaDecorationType:       model.GachaDecorationNormal,
			SortOrder:                 chapterNumber,
			BannerAssetName:           fmt.Sprintf("chapter_%d", chapterNumber),
			GroupId:                   gachaId,
			PricePhases:               buildChapterPricePhases(gachaId, spec.ticketId),
			DescriptionTextId:         gachaId,
		})
	}

	// True Dark Memory / chapter_ex
	entries = append(entries, store.GachaCatalogEntry{
		GachaId:                    chapterGachaIdBase,
		GachaLabelType:             model.GachaLabelChapter,
		GachaModeType:              model.GachaModeBox,
		GachaAutoResetType:         model.GachaAutoResetMonthly,
		GachaAutoResetPeriod:       1,
		IsUserGachaUnlock:          true,
		RelatedEventQuestChapterId: trueDarkMemoryQuestChapterId,
		GachaDecorationType:        model.GachaDecorationNormal,
		SortOrder:                  int32(len(chapterGachaSpecs) + 1),
		BannerAssetName:            "chapter_ex",
		GroupId:                    chapterGachaIdBase,
		PricePhases:                buildChapterPricePhases(chapterGachaIdBase, model.ConsumableIdDarkMemorySummonTicket),
		DescriptionTextId:          chapterGachaIdBase,
	})
	return entries
}

func chapterGachaPrerequisiteMainQuestChapterId(
	entries []store.GachaCatalogEntry,
	entryIndex int,
	quests *QuestCatalog,
) int32 {
	if entryIndex < 0 || entryIndex >= len(entries) {
		return 0
	}
	chapterId := entries[entryIndex].RelatedMainQuestChapterId
	if chapterId == 0 {
		return 0
	}

	routeId := quests.MainQuestRouteIdByChapterId[chapterId]
	seasonId := quests.SeasonIdByRouteId[routeId]

	var fallbackChapterId int32
	var previousSeasonChapterId int32

	for i := entryIndex - 1; i >= 0; i-- {
		previous := entries[i]
		if previous.GachaLabelType != model.GachaLabelChapter || previous.RelatedMainQuestChapterId == 0 {
			continue
		}
		if fallbackChapterId == 0 {
			fallbackChapterId = previous.RelatedMainQuestChapterId
		}
		if routeId == 0 {
			return fallbackChapterId
		}
		previousRouteId := quests.MainQuestRouteIdByChapterId[previous.RelatedMainQuestChapterId]
		if previousRouteId == routeId {
			return previous.RelatedMainQuestChapterId
		}
		previousSeasonId := quests.SeasonIdByRouteId[previousRouteId]
		if previousSeasonChapterId == 0 && previousSeasonId > 0 && previousSeasonId < seasonId {
			previousSeasonChapterId = previous.RelatedMainQuestChapterId
		}
	}
	if previousSeasonChapterId != 0 {
		return previousSeasonChapterId
	}
	return fallbackChapterId
}

func EnrichGachaUnlockConditions(entries []store.GachaCatalogEntry, quests *QuestCatalog) {
	lastQuestByChapter := make(map[int32]int32)
	for _, questId := range quests.OrderedQuestIds {
		if chapterId := quests.MainQuestChapterIdByQuestId[questId]; chapterId != 0 {
			lastQuestByChapter[chapterId] = questId
		}
	}

	for i := range entries {
		mainQuestChapterId := entries[i].RelatedMainQuestChapterId

		if entries[i].GachaLabelType == model.GachaLabelChapter {
			mainQuestChapterId = chapterGachaPrerequisiteMainQuestChapterId(entries, i, quests)
		}

		if mainQuestChapterId != 0 {
			if questId := lastQuestByChapter[mainQuestChapterId]; questId != 0 {
				entries[i].UnlockConditions = []store.GachaUnlockConditionEntry{{
					GachaUnlockConditionType: model.GachaUnlockMainQuestClear,
					ConditionValue:           questId,
				}}
				continue
			}
		}

		if entries[i].RelatedEventQuestChapterId != 0 {
			if entries[i].RelatedEventQuestChapterId == trueDarkMemoryQuestChapterId {
				entries[i].UnlockConditions = []store.GachaUnlockConditionEntry{{
					GachaUnlockConditionType: model.GachaUnlockNone,
				}}
				continue
			}
		}

		if len(entries[i].UnlockConditions) == 0 {
			entries[i].UnlockConditions = []store.GachaUnlockConditionEntry{{
				GachaUnlockConditionType: model.GachaUnlockNone,
			}}
		}
	}
}
