package service

import (
	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/store"
)

func dayBucket() int64 { return gametime.StartOfDayMillis() }

// lastLoginOrNow returns a non-nil display timestamp. The client renders these list rows
// and a nil lastLoginDatetime aborts list population client-side, so bots (millis 0) get now.
func lastLoginOrNow(millis int64) int64 {
	if millis <= 0 {
		return gametime.NowMillis()
	}
	return millis
}

func userProto(c PlayerCard) *pb.User {
	return &pb.User{
		PlayerId:          c.PlayerId,
		UserName:          c.Name,
		LastLoginDatetime: safeTimestamp(lastLoginOrNow(c.LastLoginDatetime)),
		MaxDeckPower:      c.MaxDeckPower,
		FavoriteCostumeId: c.FavoriteCostumeId,
		Level:             c.Level,
	}
}

func friendUserProto(c PlayerCard, e store.FriendEdge) *pb.FriendUser {
	return &pb.FriendUser{
		PlayerId:          c.PlayerId,
		UserName:          c.Name,
		LastLoginDatetime: safeTimestamp(lastLoginOrNow(c.LastLoginDatetime)),
		MaxDeckPower:      c.MaxDeckPower,
		FavoriteCostumeId: c.FavoriteCostumeId,
		Level:             c.Level,
		CheerReceived:     e.CheerReceivedPending,
		CheerSent:         e.CheerSentToday,
		StaminaReceived:   e.StaminaReceivedToday,
	}
}

func botCardFromId(pools botPools, playerId int64) PlayerCard {
	// Roster bots resolve to the exact card the ladder seeding wrote (same
	// points and power), so FinishBattle scores against the real opponent.
	if point, ok := rosterPointForId(playerId); ok {
		return rosterBot(pools, playerId, point)
	}
	// An id with no table row: synthesize the deck first and take its lead
	// costume as the avatar, so even an ephemeral bot shows one consistent face.
	lead := deckLeadCostume(synthBotDeck(pools, playerId))
	if lead == 0 {
		lead = pick(newRand(playerId), pools.costumeIds, 1)
	}
	return PlayerCard{
		PlayerId:          playerId,
		Name:              botName(playerId),
		Level:             50,
		MaxDeckPower:      powerForPoint(1000),
		FavoriteCostumeId: lead,
		IsBot:             true,
	}
}

func maybeResetCheerDay(user *store.UserState) {
	today := dayBucket()
	for pid, e := range user.Friends {
		if e.LastResetDay == today {
			continue
		}
		e.CheerSentToday = false
		e.StaminaReceivedToday = false
		if IsBotId(pid) {
			e.CheerReceivedPending = true // bots always cheer you
		}
		e.LastResetDay = today
		user.Friends[pid] = e
	}
}
