// lunar-base-grant: a single-purpose CLI placed by lunar-base into
// lunar-tear/server/cmd/lunar-base-grant/ at setup time. It wraps
// lunar-tear's UpdateUser + GrantPossession / GrantCostume so the
// lunar-base web app can mutate the save file through the same code paths
// the game server uses.
//
// Source-of-truth lives in lunar-base/tools/grant/src/. setup.bat copies
// these files into lunar-tear/server/cmd/lunar-base-grant/ before building,
// because Go's `internal/` package rule requires the importer to live inside
// lunar-tear/server/.
//
// Protocol: read one JSON request from stdin, write one JSON response to
// stdout, exit 0 on success and 1 on failure.
//
// Actions:
//   grant_possession         - apply a single GrantPossession to one user
//   grant_batch              - apply many GrantPossession calls inside one
//                              UpdateUser transaction (Item Editor MAX ALL etc.)
//   grant_costume_batch      - load master data, build a PossessionGranter,
//                              then apply many GrantCostume calls inside one
//                              UpdateUser transaction (Costume Editor)
//   grant_weapon_batch       - same plumbing as grant_costume_batch, but calls
//                              GrantWeapon per id (Weapon Editor)
//   grant_companion_batch    - same plumbing, calls GrantCompanion per id
//                              (Upgrade Manager: Add All Missing Companions)
//   grant_thought_batch      - insert a ThoughtState per id, self-skipping
//                              already-owned thoughts (Upgrade Manager:
//                              Add All Missing Debris)
//   exalt_characters         - set CharacterRebirths[id] to a target rebirth
//                              count (Upgrade Manager: Exalt All)
//   release_panels           - load CharacterBoardCatalog, release the given
//                              panel ids and apply their effects (Upgrade
//                              Manager: Fill Mythic Slab Pages)
//   upgrade_all_companions   - set every owned companion to max level (50).
//   upgrade_all_weapons      - load WeaponCatalog and, for every owned weapon,
//                              evolve to chain end, ascend to LB cap, refine
//                              if eligible, enhance to level cap, set all
//                              skills + abilities to max level (cost-bypass).
//   upgrade_all_costumes     - load CostumeCatalog and, for every owned
//                              costume, awaken to 5 (granting Debris and
//                              status-up rows), ascend to LB cap, enhance to
//                              level cap, set active skill to max, unlock 3
//                              karma slots for SSR (cost-bypass; karma rolls
//                              are still left to the player).
//   fill_karma_slots         - for every already-unlocked karma slot, pick
//                              the user's preferred (effect_type, target_id)
//                              from the slot's odds pool. Falls back to
//                              rarest if the costume's pool doesn't carry
//                              the chosen effect. Always overwrites.
//   set_costume_karma_batch  - per-costume karma write. Each spec carries
//                              {costume_id, karma: {slot: odds_number}};
//                              the shim looks up the user_costume by id,
//                              writes the OddsNumber, skips if same.
//   grant_memoir_batch       - insert N new memoirs (Parts rows) with chosen
//                              level, primary main-stat, and sub-stat rows.
//                              Pre-flights against the 999-row inventory cap.
//   upgrade_all_memoirs      - set every owned memoir's Level to 15 (no RNG
//                              enhance loop, no sub-stat fill).
//   set_memoir_subs_batch    - overwrite sub-status rows for given
//                              user_parts_uuids with caller-chosen slot
//                              configurations.
//   mark_contents_stories_played - write user.ContentsStories[id]=now for
//                              each id, marking cutscenes as viewed.
//                              Used to clear the Dark Memory cutscene
//                              queue after mass-grants.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/store/sqlite"
)

type grantSpec struct {
	PossessionType int32 `json:"possession_type"`
	PossessionID   int32 `json:"possession_id"`
	Count          int32 `json:"count"`
}

type weaponSpec struct {
	WeaponID          int32   `json:"weapon_id"`
	ExtraStoryUnlocks []int32 `json:"extra_story_unlocks,omitempty"`
}

type exaltSpec struct {
	CharacterID  int32 `json:"character_id"`
	RebirthCount int32 `json:"rebirth_count"`
}

// karmaPref is a single (effect_type, target_id) entry. The shim walks
// each slot's preference list in order; first match in the costume's
// odds pool wins. If none match, the shim falls back to the rarest entry
// in the pool (highest RarityType, ties broken by lowest OddsNumber).
type karmaPref struct {
	EffectType int32 `json:"effect_type"`
	TargetID   int32 `json:"target_id"`
}

// costumeKarmaSpec carries a per-costume target karma state for the
// `set_costume_karma_batch` action. The Python side resolves dropdown
// (effect_type, target_id) values to OddsNumber via the master-data
// catalog before invoking the shim, so the shim only has to find the
// user_costume by id and write the value.
type costumeKarmaSpec struct {
	CostumeID int32           `json:"costume_id"`
	Karma     map[string]int32 `json:"karma"` // slot ("1"/"2"/"3") -> odds_number
}

// memoirSubSpec is one sub-status slot config for a memoir. Slot 1-4.
// PartsStatusSubLotteryId stays for game-data fidelity but the displayed
// value is computed from KindType/CalcType/Value at the client.
type memoirSubSpec struct {
	Slot      int32 `json:"slot"`
	LotteryID int32 `json:"lottery_id"`
	KindType  int32 `json:"kind_type"`
	CalcType  int32 `json:"calc_type"`
	Value     int32 `json:"value"`
}

// memoirGrantSpec describes one memoir to add to inventory. PartsID picks
// the master-data row (R40 lottery=1 by convention); PartsStatusMainID is
// the chosen primary stat (one of the 36 EntityMPartsStatusMain ids);
// Level defaults to 15 for the build-set flow; Subs is slots 1-4.
type memoirGrantSpec struct {
	PartsID           int32           `json:"parts_id"`
	PartsGroupID      int32           `json:"parts_group_id"`
	PartsStatusMainID int32           `json:"parts_status_main_id"`
	Level             int32           `json:"level"`
	Subs              []memoirSubSpec `json:"subs"`
}

// memoirSlotsSpec targets an existing memoir by uuid for sub-stat rewrite.
type memoirSlotsSpec struct {
	UserPartsUUID string          `json:"user_parts_uuid"`
	Subs          []memoirSubSpec `json:"subs"`
}

type request struct {
	Action         string       `json:"action"`
	DBPath         string       `json:"db_path"`
	MasterDataPath string       `json:"master_data_path"`
	UserID         int64        `json:"user_id"`
	PossessionType int32        `json:"possession_type"`
	PossessionID   int32        `json:"possession_id"`
	Count          int32        `json:"count"`
	Grants         []grantSpec  `json:"grants"`
	CostumeIDs     []int32      `json:"costume_ids"`
	Weapons        []weaponSpec `json:"weapons"`
	CompanionIDs   []int32      `json:"companion_ids"`
	ThoughtIDs     []int32      `json:"thought_ids"`
	Exaltations    []exaltSpec  `json:"exaltations"`
	PanelIDs       []int32      `json:"panel_ids"`
	// KarmaPreferences keys are slot numbers as strings ("1", "2", "3")
	// because Go's JSON tag-mapped struct fields would force a fixed key
	// shape; using map[string][]karmaPref lets the Python side just emit
	// {"1": [...], "2": [...], "3": [...]}.
	KarmaPreferences map[string][]karmaPref `json:"karma_preferences"`
	CostumeKarma     []costumeKarmaSpec     `json:"costume_karma"`
	Memoirs          []memoirGrantSpec      `json:"memoirs"`
	MemoirSlots      []memoirSlotsSpec      `json:"memoir_slots"`
	ContentsStoryIDs []int32                `json:"contents_story_ids"`
	QuestIDs         []int32                `json:"quest_ids"`
}

type response struct {
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
	Applied int         `json:"applied,omitempty"`
	Quests  []questInfo `json:"quests,omitempty"`
}

// queryQuests is set by read-only list actions (list_quests) and emitted by main.
var queryQuests []questInfo

func openDB(path string) (interface {
	Close() error
}, *sqlite.SQLiteStore, error) {
	db, err := database.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}
	return db, sqlite.New(db, nil), nil
}

func runStackable(req *request, grants []grantSpec) (int, error) {
	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, g := range grants {
			store.GrantPossession(u, model.PossessionType(g.PossessionType), g.PossessionID, g.Count)
		}
	})
	if err != nil {
		return 0, fmt.Errorf("grant: %w", err)
	}
	return len(grants), nil
}

func runCostumeBatch(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for grant_costume_batch")
	}
	if len(req.CostumeIDs) == 0 {
		return 0, errors.New("costume_ids list is empty")
	}

	if err := memorydb.Init(req.MasterDataPath); err != nil {
		return 0, fmt.Errorf("init master data: %w", err)
	}
	partsCatalog, err := masterdata.LoadPartsCatalog()
	if err != nil {
		return 0, fmt.Errorf("load parts catalog: %w", err)
	}
	catalog, err := masterdata.LoadQuestCatalog(partsCatalog)
	if err != nil {
		return 0, fmt.Errorf("load quest catalog: %w", err)
	}
	gameConfig, err := masterdata.LoadGameConfig()
	if err != nil {
		return 0, fmt.Errorf("load game config: %w", err)
	}
	granter := questflow.BuildGranter(catalog, gameConfig)

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, costumeID := range req.CostumeIDs {
			granter.GrantCostume(u, costumeID, now)
		}
	})
	if err != nil {
		return 0, fmt.Errorf("grant costume: %w", err)
	}
	return len(req.CostumeIDs), nil
}

func runWeaponBatch(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for grant_weapon_batch")
	}
	if len(req.Weapons) == 0 {
		return 0, errors.New("weapons list is empty")
	}

	if err := memorydb.Init(req.MasterDataPath); err != nil {
		return 0, fmt.Errorf("init master data: %w", err)
	}
	partsCatalog, err := masterdata.LoadPartsCatalog()
	if err != nil {
		return 0, fmt.Errorf("load parts catalog: %w", err)
	}
	catalog, err := masterdata.LoadQuestCatalog(partsCatalog)
	if err != nil {
		return 0, fmt.Errorf("load quest catalog: %w", err)
	}
	gameConfig, err := masterdata.LoadGameConfig()
	if err != nil {
		return 0, fmt.Errorf("load game config: %w", err)
	}
	granter := questflow.BuildGranter(catalog, gameConfig)

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, w := range req.Weapons {
			granter.GrantWeapon(u, w.WeaponID, now)
			// Extra story unlocks (e.g. Dark Memory R50: stories 2-4 are
			// tied to evolution milestones we skipped by granting the
			// final form). grantWeaponStoryUnlock is a no-op if already
			// unlocked, so it is safe to call regardless.
			for _, idx := range w.ExtraStoryUnlocks {
				store.GrantWeaponStoryUnlock(u, w.WeaponID, idx, now)
			}
		}
	})
	if err != nil {
		return 0, fmt.Errorf("grant weapon: %w", err)
	}
	return len(req.Weapons), nil
}

func run() (int, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 0, fmt.Errorf("read stdin: %w", err)
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return 0, fmt.Errorf("parse json: %w", err)
	}
	if req.DBPath == "" {
		return 0, errors.New("db_path required")
	}
	if req.UserID <= 0 {
		return 0, errors.New("user_id required")
	}

	switch req.Action {
	case "grant_possession":
		return runStackable(&req, []grantSpec{{
			PossessionType: req.PossessionType,
			PossessionID:   req.PossessionID,
			Count:          req.Count,
		}})
	case "grant_batch":
		if len(req.Grants) == 0 {
			return 0, errors.New("grants list is empty")
		}
		return runStackable(&req, req.Grants)
	case "grant_costume_batch":
		return runCostumeBatch(&req)
	case "grant_weapon_batch":
		return runWeaponBatch(&req)
	case "grant_companion_batch":
		return runCompanionBatch(&req)
	case "grant_thought_batch":
		return runThoughtBatch(&req)
	case "exalt_characters":
		return runExalt(&req)
	case "release_panels":
		return runReleasePanels(&req)
	case "upgrade_all_companions":
		return runUpgradeAllCompanions(&req)
	case "upgrade_all_weapons":
		return runUpgradeAllWeapons(&req)
	case "upgrade_all_costumes":
		return runUpgradeAllCostumes(&req)
	case "fill_karma_slots":
		return runFillKarmaSlots(&req)
	case "set_costume_karma_batch":
		return runSetCostumeKarmaBatch(&req)
	case "grant_memoir_batch":
		return runGrantMemoirBatch(&req)
	case "upgrade_all_memoirs":
		return runUpgradeAllMemoirs(&req)
	case "set_memoir_subs_batch":
		return runSetMemoirSubsBatch(&req)
	case "mark_contents_stories_played":
		return runMarkContentsStoriesPlayed(&req)
	case "list_quests":
		return runListQuests(&req)
	case "clear_quests":
		return runClearQuests(&req)
	case "":
		return 0, errors.New("action required")
	default:
		return 0, fmt.Errorf("unknown action %q", req.Action)
	}
}

func main() {
	enc := json.NewEncoder(os.Stdout)
	applied, err := run()
	if err != nil {
		_ = enc.Encode(response{OK: false, Error: err.Error()})
		os.Exit(1)
	}
	_ = enc.Encode(response{OK: true, Applied: applied, Quests: queryQuests})
}
