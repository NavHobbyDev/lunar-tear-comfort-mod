package model

const (
	// PartsInventoryCap is the maximum number of memoirs (Parts) the
	// inventory can hold. The client enforces nothing here, so the server
	// never grants re-farmable drop parts past the cap: they are sold for
	// gold, and quest skips are refused while the inventory is full.
	// One-time rewards (first clear, missions) may carry rare memoirs and
	// are granted even past the cap rather than lost.
	PartsInventoryCap int32 = 999
)
