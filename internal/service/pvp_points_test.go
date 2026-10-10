package service

import "testing"

// TestPointDelta_winLossFloor pins the retail-reverse-engineered AP curve:
// gap = (oppPoint - myPoint) / 12, win = 192 + gap clamped to [+12, +300],
// loss = gap - 144 clamped to [-288, -144].
func TestPointDelta_winLossFloor(t *testing.T) {
	if d := pointDelta(1000, 1000, true); d != 192 {
		t.Fatalf("even win want 192 got %d", d)
	}
	if d := pointDelta(1000, 2000, true); d != 275 {
		t.Fatalf("win vs +1000 stronger opp want 275 got %d", d)
	}
	if d := pointDelta(1000, 5000, true); d != 300 {
		t.Fatalf("win bonus must cap at 300, got %d", d)
	}
	if d := pointDelta(1000, 500, true); d != 151 {
		t.Fatalf("win vs weaker opp shrinks, want 151 got %d", d)
	}
	if d := pointDelta(3000, 100, true); d != 12 {
		t.Fatalf("win vs much weaker opp must floor at 12, got %d", d)
	}
	if d := pointDelta(1000, 1000, false); d != -144 {
		t.Fatalf("even loss want -144 got %d", d)
	}
	if d := pointDelta(1000, 3000, false); d != -144 {
		t.Fatalf("loss vs stronger opp never lighter than -144, got %d", d)
	}
	if d := pointDelta(1000, 500, false); d != -185 {
		t.Fatalf("loss vs weaker opp want -185 got %d", d)
	}
	if d := pointDelta(10000, 100, false); d != -288 {
		t.Fatalf("loss penalty must cap at -288, got %d", d)
	}
	if v := applyPointDelta(5, -10); v != 0 {
		t.Fatalf("points must floor at 0, got %d", v)
	}
}
