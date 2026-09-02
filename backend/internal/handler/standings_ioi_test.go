package handler

import (
	"testing"
	"time"

	"github.com/ysnb/oj/internal/model"
)

// ioiFixture builds a 90-minute IOI contest starting at base with no freeze
// and three problems A/B/C.
func ioiFixture() (*model.Contest, []model.ContestProblem, time.Time) {
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	contest := &model.Contest{
		ID: 1, Mode: model.ContestModeIOI,
		StartTime: start, EndTime: start.Add(90 * time.Minute),
		FreezeEnabled: false,
	}
	links := []model.ContestProblem{
		{ContestID: 1, ProblemID: 11, Label: "A"},
		{ContestID: 1, ProblemID: 12, Label: "B"},
		{ContestID: 1, ProblemID: 13, Label: "C"},
	}
	return contest, links, start
}

func ioiSub(uid, pid uint, score int, status string, at time.Time) model.Submission {
	return model.Submission{UserID: uid, ProblemID: pid, ContestID: nil,
		Status: status, Score: score, CreatedAt: at}
}

func TestComputeIOIStandingsBestOfAndRanking(t *testing.T) {
	contest, links, start := ioiFixture()
	at := start.Add(10 * time.Minute)
	subs := []model.Submission{
		// user 1: A partial 60 then full 100 (best-of 100), B tried 0
		ioiSub(1, 11, 40, model.SubWA, at),
		ioiSub(1, 11, 100, model.SubAC, at.Add(5*time.Minute)),
		ioiSub(1, 12, 30, model.SubTLE, at.Add(6*time.Minute)),
		// user 2: A partial 40, B partial 90
		ioiSub(2, 11, 40, model.SubWA, at),
		ioiSub(2, 12, 60, model.SubWA, at),
		ioiSub(2, 12, 90, model.SubWA, at.Add(time.Minute)),
		// user 3: nothing submitted — must not appear
	}
	rows := computeStandings(contest, links, subs, nil, contest.EndTime.Add(time.Minute), 0)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// user1 total = 100(A best) + 30(B best) = 130; user2 = 40+90 = 130 → tie
	// broken by user id ascending: user 1 first.
	if rows[0].UserID != 1 || rows[1].UserID != 2 {
		t.Fatalf("rank order wrong: %v then %v", rows[0].UserID, rows[1].UserID)
	}
	if rows[0].PenaltyMS != 130 || rows[1].PenaltyMS != 130 {
		t.Fatalf("IOI totals wrong: %d/%d (PenaltyMS carries total score)", rows[0].PenaltyMS, rows[1].PenaltyMS)
	}
	if rows[0].Rank != 1 || rows[1].Rank != 2 {
		t.Fatalf("ranks wrong: %d/%d", rows[0].Rank, rows[1].Rank)
	}
	if c := rows[0].Cells["A"]; c.SolvedMS != 100 || !c.Solved {
		t.Fatalf("user1 A cell must be best-of 100 solved, got %+v", c)
	}
	if c := rows[0].Cells["B"]; c.SolvedMS != 30 || c.Solved {
		t.Fatalf("user1 B cell must be 30 unsolved, got %+v", c)
	}
	if c := rows[1].Cells["B"]; c.SolvedMS != 90 {
		t.Fatalf("user2 B cell must be best-of 90, got %+v", c)
	}
}

func TestComputeIOIStandingsCancelledAndCheatedExcluded(t *testing.T) {
	contest, links, start := ioiFixture()
	at := start.Add(10 * time.Minute)
	cancelled := ioiSub(1, 11, 100, model.SubAC, at)
	cancelled.Cancelled = true
	subs := []model.Submission{
		cancelled,
		ioiSub(2, 11, 50, model.SubWA, at),
	}
	flags := map[uint]model.ContestUserFlag{
		3: {UserID: 3, Cheated: true},
	}
	rows := computeStandings(contest, links, subs, flags, contest.EndTime.Add(time.Minute), 0)
	// user1's only submission was jury-cancelled → no row
	if len(rows) != 2 {
		t.Fatalf("want 2 rows (user2 + cheated user3), got %d", len(rows))
	}
	var u2, u3 *struct{ id, rank, score int64 }
	for i := range rows {
		id := int64(rows[i].UserID)
		if id == 2 {
			u2 = &struct{ id, rank, score int64 }{id, int64(rows[i].Rank), rows[i].PenaltyMS}
		}
		if id == 3 {
			u3 = &struct{ id, rank, score int64 }{id, int64(rows[i].Rank), rows[i].PenaltyMS}
		}
	}
	if u2 == nil || u2.score != 50 || u2.rank != 1 {
		t.Fatalf("user2 row wrong: %+v", u2)
	}
	if u3 == nil || !rows[1].Cheated || u3.score != 0 {
		t.Fatalf("cheated user must keep an empty flagged row: %+v", rows[1])
	}
}

func TestComputeIOIStandingsFreezeMasksLaterScores(t *testing.T) {
	contest, links, start := ioiFixture()
	freeze := start.Add(60 * time.Minute)
	contest.FreezeEnabled = true
	contest.FreezeTime = &freeze
	subs := []model.Submission{
		ioiSub(1, 11, 40, model.SubWA, start.Add(30*time.Minute)),  // pre-freeze
		ioiSub(1, 11, 90, model.SubWA, start.Add(70*time.Minute)),  // frozen
		ioiSub(2, 11, 85, model.SubAC, start.Add(20*time.Minute)),  // pre-freeze
	}
	rows := computeStandings(contest, links, subs, nil, start.Add(80*time.Minute), 0)
	byUser := map[uint]standingRow{}
	for i := range rows {
		byUser[rows[i].UserID] = rows[i]
	}
	if c := byUser[1].Cells["A"]; c.SolvedMS != 40 || c.Pending != 1 {
		t.Fatalf("user1 A must show pre-freeze 40 with 1 pending, got %+v", c)
	}
	if c := byUser[2].Cells["A"]; c.SolvedMS != 85 {
		t.Fatalf("user2 A must show 85, got %+v", c)
	}
	// user2 (85) outranks user1 (40)
	if byUser[2].Rank != 1 || byUser[1].Rank != 2 {
		t.Fatalf("freeze must not leak higher post-freeze scores: %+v", rows)
	}
}

func TestComputeIOIStandingsModeDispatch(t *testing.T) {
	// an ACM-mode contest with the same inputs must NOT use IOI math
	contest, links, start := ioiFixture()
	contest.Mode = model.ContestModeACM
	at := start.Add(10 * time.Minute)
	subs := []model.Submission{
		ioiSub(1, 11, 60, model.SubWA, at),
		ioiSub(2, 11, 100, model.SubAC, at),
	}
	rows := computeStandings(contest, links, subs, nil, contest.EndTime.Add(time.Minute), 0)
	// ACM: user2 solved (rank 1), user1 failed (rank 2); PenaltyMS is time-based
	if rows[0].UserID != 2 || rows[0].Solved != 1 || rows[1].Solved != 0 {
		t.Fatalf("ACM standings must ignore scores: %+v", rows)
	}
}