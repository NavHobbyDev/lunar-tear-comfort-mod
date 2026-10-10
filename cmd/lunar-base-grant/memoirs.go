// Memoir (parts) actions for the lunar-base-grant shim:
//   - grant_memoir_batch       insert N new parts at a chosen level + main
//                              stat + sub-stats, in one UpdateUser txn
//   - upgrade_all_memoirs      set every owned part's Level to 15
//   - set_memoir_subs_batch    overwrite sub-stat rows for given user_parts_uuids
//
// These bypass PartsService.Enhance entirely. The real flow rolls a per-level
// success rate, picks sub-stats by random lottery, and grows the value at the
// unlock level only. We treat the request as authoritative state: the user
// has chosen the primary main-stat id and sub-stat (kind, calc, value)
// directly, and we set them without RNG. That mirrors how Upgrade All
// Weapons / Costumes already works.
//
// Encoding reference (per the user's enhance log):
//   StatusKindType: 1=Agility 2=Attack 3=CritDmg 4=CritRate 6=HP 7=Defense
//   StatusCalculationType: 1=flat, 2=percent (percent values stored ×10:
//       3% = 30, 12.5% = 125, 25% = 250, 36% = 360)
//   PartsStatusSubLotteryId: stored for completeness; the displayed value
//       is computed from kind/calc/StatusChangeValue at the client.

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"lunar-tear/server/internal/store"
)

const (
	memoirMaxLevel    = int32(15)
	memoirInventoryCap = 999
)

// runGrantMemoirBatch inserts new memoirs (Parts rows) at the requested
// level + main-stat + sub-stat configuration. PartsGroupNotes are added
// for any group that the user has never owned a memoir from before, so the
// memoir compendium picks them up. Pre-flights against the 999-row cap.
func runGrantMemoirBatch(req *request) (int, error) {
	if len(req.Memoirs) == 0 {
		return 0, errors.New("memoirs list is empty")
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		current := len(u.Parts)
		if current+len(req.Memoirs) > memoirInventoryCap {
			err = fmt.Errorf(
				"memoir inventory cap: %d already owned + %d requested would exceed %d",
				current, len(req.Memoirs), memoirInventoryCap,
			)
			return
		}
		for _, m := range req.Memoirs {
			level := m.Level
			if level <= 0 {
				level = 1
			}
			if level > memoirMaxLevel {
				level = memoirMaxLevel
			}
			key := uuid.New().String()
			u.Parts[key] = store.PartsState{
				UserPartsUuid:       key,
				PartsId:             m.PartsID,
				Level:               level,
				PartsStatusMainId:   m.PartsStatusMainID,
				IsProtected:         false,
				AcquisitionDatetime: now,
				LatestVersion:       now,
			}
			if _, exists := u.PartsGroupNotes[m.PartsGroupID]; !exists {
				u.PartsGroupNotes[m.PartsGroupID] = store.PartsGroupNoteState{
					PartsGroupId:             m.PartsGroupID,
					FirstAcquisitionDatetime: now,
					LatestVersion:            now,
				}
			}
			for _, s := range m.Subs {
				if s.Slot < 1 || s.Slot > 4 {
					continue
				}
				u.PartsStatusSubs[store.PartsStatusSubKey{
					UserPartsUuid: key,
					StatusIndex:   s.Slot,
				}] = store.PartsStatusSubState{
					UserPartsUuid:           key,
					StatusIndex:             s.Slot,
					PartsStatusSubLotteryId: s.LotteryID,
					Level:                   level,
					StatusKindType:          s.KindType,
					StatusCalculationType:   s.CalcType,
					StatusChangeValue:       s.Value,
					LatestVersion:           now,
				}
			}
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("grant memoir batch: %w", err)
	}
	return applied, nil
}

// runUpgradeAllMemoirs sets every owned memoir's Level to 15. Sub-status
// rows are left untouched — empty slots stay empty (this matches the
// user's request: the action is "upgrade everything to lv15", not "give
// every memoir 4 perfect-roll subs").
func runUpgradeAllMemoirs(req *request) (int, error) {
	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for partUuid, part := range u.Parts {
			if part.Level >= memoirMaxLevel {
				continue
			}
			part.Level = memoirMaxLevel
			part.LatestVersion = now
			u.Parts[partUuid] = part
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("upgrade all memoirs: %w", err)
	}
	return applied, nil
}

// runSetMemoirSubsBatch overwrites sub-status rows for the given memoir
// uuids. Each spec carries the full slot 1-4 desired state; we replace
// (not merge) — any slot present in the spec is written, slots not in
// the spec are left untouched. Unknown uuids are skipped.
func runSetMemoirSubsBatch(req *request) (int, error) {
	if len(req.MemoirSlots) == 0 {
		return 0, errors.New("memoir_slots list is empty")
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, spec := range req.MemoirSlots {
			part, ok := u.Parts[spec.UserPartsUUID]
			if !ok {
				continue
			}
			for _, s := range spec.Subs {
				if s.Slot < 1 || s.Slot > 4 {
					continue
				}
				u.PartsStatusSubs[store.PartsStatusSubKey{
					UserPartsUuid: spec.UserPartsUUID,
					StatusIndex:   s.Slot,
				}] = store.PartsStatusSubState{
					UserPartsUuid:           spec.UserPartsUUID,
					StatusIndex:             s.Slot,
					PartsStatusSubLotteryId: s.LotteryID,
					Level:                   part.Level,
					StatusKindType:          s.KindType,
					StatusCalculationType:   s.CalcType,
					StatusChangeValue:       s.Value,
					LatestVersion:           now,
				}
			}
			part.LatestVersion = now
			u.Parts[spec.UserPartsUUID] = part
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("set memoir subs: %w", err)
	}
	return applied, nil
}
