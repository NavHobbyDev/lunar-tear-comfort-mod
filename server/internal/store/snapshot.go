package store

// PlayerSnapshot is a real player's public face, written on login and PvP-deck edit.
type PlayerSnapshot struct {
	PlayerId          int64
	UserName          string
	Level             int32
	MaxDeckPower      int32
	FavoriteCostumeId int32
	PvpPoint          int32
	LastLoginDatetime int64
	DefenseDeckJson   string // serialized []*pb.PvpDeckCharacter
	UpdatedAt         int64
}

// SnapshotRepository stores and queries player snapshots (the cross-user directory source).
type SnapshotRepository interface {
	UpsertSnapshot(s PlayerSnapshot) error
	GetSnapshot(playerId int64) (PlayerSnapshot, error)
	DeleteSnapshot(playerId int64) error
	// ListSnapshotsNear returns up to limit snapshots other than excludePlayerId,
	// ordered by closeness of pvp_point to nearPoint (closest first).
	ListSnapshotsNear(excludePlayerId int64, nearPoint int32, limit int) ([]PlayerSnapshot, error)
	// ListSnapshotsByPointDesc returns a ranking page ordered by pvp_point desc.
	ListSnapshotsByPointDesc(offset, limit int) ([]PlayerSnapshot, error)
	CountSnapshots() (int, error)
	RankOfPlayer(playerId int64) (int, error) // 1-based rank by pvp_point desc; 0 if absent
	// MaxPointAbovePower returns the highest pvp_point held by any snapshot
	// strictly stronger than power (excluding excludePlayerId). ok=false when
	// no stronger snapshot exists, i.e. the ladder imposes no cap.
	MaxPointAbovePower(excludePlayerId int64, power int32) (capPoint int32, ok bool, err error)
	// AllUserIds lists every account id, for backfilling snapshots on startup.
	AllUserIds() ([]int64, error)
}
