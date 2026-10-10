// Code generated from official mission/weapon name dumps; DO NOT EDIT.

package masterdata

// weaponMissionOptionGroups maps a MissionClearConditionOptionGroupId used by
// weapon-specific missions (condition types 6, 8 and 43: "enhance skills",
// "ascend" and "reach level" for a concrete weapon) to every WeaponId of the
// targeted weapon family. Evolution steps share the same weapon name, so all
// of them count toward the mission. The official tables carry no group ->
// weapon relation; the mapping was recovered from the mission text ids.
var weaponMissionOptionGroups = map[int32][]int32{
	200: { 250011, 250012 }, // Desolate Loss
	202: { 250011, 250012 }, // Desolate Loss
	205: { 340031, 340032 }, // Blackbird Dagger
	206: { 340031, 340032 }, // Blackbird Dagger
	207: { 340031, 340032 }, // Blackbird Dagger
	209: { 250141, 250142 }, // Dark Revolver
	210: { 250141, 250142 }, // Dark Revolver
	211: { 250141, 250142 }, // Dark Revolver
	220: { 220061, 220062 }, // Spear of Freezing Mist
	221: { 220061, 220062 }, // Spear of Freezing Mist
	222: { 220061, 220062 }, // Spear of Freezing Mist
	225: { 310131, 310132 }, // Blackbird Gauntlets
	226: { 220071, 220072 }, // Dark Spear
	227: { 310131, 310132 }, // Blackbird Gauntlets
	228: { 220071, 220072 }, // Dark Spear
	229: { 310131, 310132 }, // Blackbird Gauntlets
	230: { 220071, 220072 }, // Dark Spear
	240: { 250151, 250152 }, // Serene Handgun
	241: { 250151, 250152 }, // Serene Handgun
	242: { 250151, 250152 }, // Serene Handgun
	244: { 330061, 330062 }, // Blackbird Greatsword
	245: { 210021, 210022 }, // Dark Dagger
	246: { 330061, 330062 }, // Blackbird Greatsword
	247: { 210021, 210022 }, // Dark Dagger
	248: { 330061, 330062 }, // Blackbird Greatsword
	249: { 210021, 210022 }, // Dark Dagger
	257: { 210181, 210182 }, // Hexer's Staff
	258: { 210181, 210182 }, // Hexer's Staff
	259: { 210181, 210182 }, // Hexer's Staff
	267: { 230151, 230152 }, // Baton of Tender Rain
	268: { 230151, 230152 }, // Baton of Tender Rain
	269: { 230151, 230152 }, // Baton of Tender Rain
	271: { 320071, 320072 }, // Blackbird Lance
	272: { 230121, 230122 }, // Dark Staff
	273: { 320071, 320072 }, // Blackbird Lance
	274: { 230121, 230122 }, // Dark Staff
	275: { 320071, 320072 }, // Blackbird Lance
	276: { 230121, 230122 }, // Dark Staff
	284: { 250171, 250172, 9038045, 9038205 }, // Javelin of Vice
	285: { 250171, 250172, 9038045, 9038205 }, // Javelin of Vice
	286: { 250171, 250172, 9038045, 9038205 }, // Javelin of Vice
	294: { 210191, 210192 }, // Trial Shearing Pike: WR0a
	295: { 210191, 210192 }, // Trial Shearing Pike: WR0a
	296: { 210191, 210192 }, // Trial Shearing Pike: WR0a
	298: { 340271, 340272 }, // Blackbird Crosier
	299: { 250181, 250182 }, // Dark Fists
	300: { 340271, 340272 }, // Blackbird Crosier
	301: { 250181, 250182 }, // Dark Fists
	302: { 340271, 340272 }, // Blackbird Crosier
	303: { 250181, 250182 }, // Dark Fists
	305: { 320161, 320162 }, // Blackbird Gun
	306: { 230171, 230172 }, // Dark Longsword
	307: { 320161, 320162 }, // Blackbird Gun
	308: { 230171, 230172 }, // Dark Longsword
	309: { 320161, 320162 }, // Blackbird Gun
	310: { 230171, 230172 }, // Dark Longsword
	318: { 350181, 350182 }, // Wicked Bottles
	319: { 350181, 350182 }, // Wicked Bottles
	320: { 350181, 350182 }, // Wicked Bottles
	338: { 220161, 220162 }, // Hakage
	339: { 220161, 220162 }, // Hakage
	340: { 220161, 220162 }, // Hakage
	348: { 240201, 240202 }, // Umbra 1945
	349: { 240201, 240202 }, // Umbra 1945
	350: { 240201, 240202 }, // Umbra 1945
	352: { 350211, 350212 }, // Blacktoe Dagger
	353: { 350211, 350212 }, // Blacktoe Dagger
	354: { 350211, 350212 }, // Blacktoe Dagger
	362: { 240221, 240222 }, // Muguet Lumière
	364: { 240221, 240222 }, // Muguet Lumière
	366: { 310281, 310282 }, // Blacktoe Greatsword
	367: { 310281, 310282 }, // Blacktoe Greatsword
	368: { 310281, 310282 }, // Blacktoe Greatsword
	375: { 350221, 350222 }, // Flurry of Indulgence
	376: { 350221, 350222 }, // Flurry of Indulgence
	379: { 320221, 320222 }, // Blacktoe Crosier
	380: { 320221, 320222 }, // Blacktoe Crosier
	381: { 320221, 320222 }, // Blacktoe Crosier
	388: { 350261, 350262 }, // Blackened Plumage
	389: { 350261, 350262 }, // Blackened Plumage
	397: { 340371, 340372 }, // Blacktoe Handgun
	398: { 340371, 340372 }, // Blacktoe Handgun
	399: { 340371, 340372 }, // Blacktoe Handgun
	406: { 220181, 220182 }, // Rare-Iron Blade
	407: { 220181, 220182 }, // Rare-Iron Blade
	410: { 330301, 330302 }, // Blacktoe Gauntlets
	411: { 330301, 330302 }, // Blacktoe Gauntlets
	412: { 330301, 330302 }, // Blacktoe Gauntlets
	428: { 310321, 310322 }, // Blade of Begging
	429: { 310321, 310322 }, // Blade of Begging
	438: { 350291, 350292 }, // Blacktoe Halberd
	439: { 350291, 350292 }, // Blacktoe Halberd
	440: { 350291, 350292 }, // Blacktoe Halberd
	447: { 240241, 240242 }, // Nightcrow's Pistol
	448: { 240241, 240242 }, // Nightcrow's Pistol
	457: { 220191, 220192 }, // Haze
	458: { 220191, 220192 }, // Haze
	465: { 310361, 310362 }, // Darkweave Gauntlets
	466: { 310361, 310362 }, // Darkweave Gauntlets
	467: { 310361, 310362 }, // Darkweave Gauntlets
	470: { 330371, 330372 }, // Darkweave Baton
	471: { 330371, 330372 }, // Darkweave Baton
	472: { 330371, 330372 }, // Darkweave Baton
	477: { 350331, 350332 }, // Misos
	478: { 350331, 350332 }, // Misos
	489: { 220211, 220212 }, // Deceit's Fang
	490: { 220211, 220212 }, // Deceit's Fang
	493: { 310391, 310392 }, // Darkweave Pike
	494: { 310391, 310392 }, // Darkweave Pike
	495: { 310391, 310392 }, // Darkweave Pike
	505: { 350351, 350352 }, // Darkweave Pistol
	506: { 350351, 350352 }, // Darkweave Pistol
	507: { 350351, 350352 }, // Darkweave Pistol
	515: { 250221, 250222 }, // Hero's Dying Wish
	516: { 250221, 250222 }, // Hero's Dying Wish
	529: { 340521, 340522 }, // Blackened Will
	530: { 340521, 340522 }, // Blackened Will
	533: { 320361, 320362 }, // Darkweave Dagger
	534: { 320361, 320362 }, // Darkweave Dagger
	535: { 320361, 320362 }, // Darkweave Dagger
	541: { 320371, 320372 }, // Katana
	543: { 320371, 320372 }, // Katana
	555: { 250231, 250232 }, // Fairy-Tale Staff
	556: { 250231, 250232 }, // Fairy-Tale Staff
	559: { 340561, 340562 }, // Darkweave Zweihander
	560: { 340561, 340562 }, // Darkweave Zweihander
	561: { 340561, 340562 }, // Darkweave Zweihander
	568: { 220231, 220232 }, // Ocarina
	569: { 220231, 220232 }, // Ocarina
	572: { 310491, 310492 }, // Gun of Incineration
	573: { 310491, 310492 }, // Gun of Incineration
	574: { 310491, 310492 }, // Gun of Incineration
}

