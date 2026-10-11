package model

const (
	// GiftInventoryCap is the maximum number of unclaimed mails the gift
	// box (in-game mail) can hold, mirroring the weapon and memoir
	// inventory caps. When a new mail arrives at a full mailbox, the
	// oldest mails are dropped to make room.
	GiftInventoryCap int32 = 999
)
