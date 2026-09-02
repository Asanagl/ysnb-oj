package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/ysnb/oj/internal/model"
)

func mkContest(start, end, freeze *time.Time) *model.Contest {
	return &model.Contest{StartTime: *start, EndTime: *end, FreezeTime: freeze, FreezeEnabled: true}
}

func ts(mins int) *time.Time {
	base := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	t := base.Add(time.Duration(mins) * time.Minute)
	return &t
}

func uid(subs []model.Submission, i int) uint { return subs[i].UserID }

func TestComputeStandingsBasicACM(t *testing.T) {
	contest := mkContest(ts(0), ts(300), nil)
	links := []model.ContestProblem{
		{ContestID: 1, ProblemID: 1, Label: "A"},
		{ContestID: 1, ProblemID: 2, Label: "B"},
	}
	subs := []model.Submission{
		{UserID: 1, ProblemID: 1, Status: model.SubWA, CreatedAt: *ts(10)},
		{UserID: 1, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(30)},
		{UserID: 2, ProblemID: 2, Status: model.SubAC, CreatedAt: *ts(20)},
	}
	rows := computeStandings(contest, links, subs, map[uint]model.ContestUserFlag{}, *ts(60), 0)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// user 2 solved cleanly at 20min; user 1 solved A at 30min + 20min penalty
	// -> user 2 ranks first on lower penalty
	if rows[0].UserID != 2 || rows[0].Solved != 1 {
		t.Fatalf("user 2 should rank first: %+v", rows[0])
	}
	if rows[1].UserID != 1 {
		t.Fatalf("user 1 should rank second: %+v", rows[1])
	}
	// 30 min solve + 20 min penalty = 50 min
	want := 50 * 60 * 1000
	if rows[1].PenaltyMS != int64(want) {
		t.Fatalf("penalty: got %d ms, want %d", rows[1].PenaltyMS, want)
	}
	cell := rows[1].Cells["A"]
	if !cell.Solved || cell.Attempts != 1 {
		t.Fatalf("cell A wrong: %+v", cell)
	}
	_ = uid
}

func TestComputeStandingsFreezeHidesLateSolve(t *testing.T) {
	contest := mkContest(ts(0), ts(300), ts(120))
	links := []model.ContestProblem{{ContestID: 1, ProblemID: 1, Label: "A"}}
	subs := []model.Submission{
		// solve arrives after freeze -> must appear as pending, not solved
		{UserID: 1, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(200)},
		{UserID: 2, ProblemID: 1, Status: model.SubWA, CreatedAt: *ts(200)},
	}
	rows := computeStandings(contest, links, subs, map[uint]model.ContestUserFlag{}, *ts(150), 0)
	if rows[0].UserID != 2 && rows[1].UserID != 2 {
		t.Fatalf("both users should have 0 solves: %+v", rows)
	}
	for _, r := range rows {
		if r.Solved != 0 {
			t.Fatalf("frozen contest must hide solves, user %d has %d", r.UserID, r.Solved)
		}
	}
	// after the contest ends, the freeze no longer applies
	rows = computeStandings(contest, links, subs, map[uint]model.ContestUserFlag{}, *ts(400), 0)
	if rows[0].UserID != 1 || rows[0].Solved != 1 {
		t.Fatalf("post-contest standings must reveal the solve: %+v", rows[0])
	}
}

func TestComputeStandingsIgnoresCEAndUnknownProblems(t *testing.T) {
	contest := mkContest(ts(0), ts(300), nil)
	links := []model.ContestProblem{{ContestID: 1, ProblemID: 1, Label: "A"}}
	subs := []model.Submission{
		{UserID: 1, ProblemID: 1, Status: model.SubCE, CreatedAt: *ts(5)},
		{UserID: 1, ProblemID: 99, Status: model.SubAC, CreatedAt: *ts(5)}, // not in contest
	}
	rows := computeStandings(contest, links, subs, map[uint]model.ContestUserFlag{}, *ts(60), 0)
	cell := rows[0].Cells["A"]
	if cell.Attempts != 0 || cell.Solved {
		t.Fatalf("CE must not count as attempt: %+v", cell)
	}
}

func TestParseUserCSV(t *testing.T) {
	csvData := "alice,S001,Alice,\nbob,S002,,bobpass\n\n,badrow\n"
	rows := parseUserCSV(strings.NewReader(csvData), 10)
	if len(rows) != 3 {
		t.Fatalf("expected 3 data rows, got %d", len(rows))
	}
	if rows[0].Password == "" {
		t.Fatal("missing password must be generated")
	}
	if rows[1].Password != "bobpass" {
		t.Fatalf("explicit password lost: %q", rows[1].Password)
	}
	if rows[2].Err == "" {
		t.Fatal("row without username must be flagged")
	}
}

// --- jury behavior tests (cancel / cheat / star) ---

func emptyFlags() map[uint]model.ContestUserFlag {
	return map[uint]model.ContestUserFlag{}
}

func TestComputeStandingsCheatedExcludedButRowKept(t *testing.T) {
	contest := mkContest(ts(0), ts(300), nil)
	links := []model.ContestProblem{{ContestID: 1, ProblemID: 1, Label: "A"}}
	subs := []model.Submission{
		{UserID: 1, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(10)},
		{UserID: 2, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(20)},
	}
	flags := map[uint]model.ContestUserFlag{2: {UserID: 2, Cheated: true}}
	rows := computeStandings(contest, links, subs, flags, *ts(60), 0)
	if len(rows) != 2 {
		t.Fatalf("cheated row must be kept, got %d rows", len(rows))
	}
	if rows[0].UserID != 1 || rows[0].Rank != 1 {
		t.Fatalf("honest user must rank first: %+v", rows[0])
	}
	last := rows[1]
	if !last.Cheated || last.Solved != 0 || last.Rank != 0 {
		t.Fatalf("cheated row must be flagged, unscored, unrated: %+v", last)
	}
}

func TestComputeStandingsCancelledSubmissionInvisible(t *testing.T) {
	contest := mkContest(ts(0), ts(300), nil)
	links := []model.ContestProblem{{ContestID: 1, ProblemID: 1, Label: "A"}}
	subs := []model.Submission{
		{UserID: 1, ProblemID: 1, Status: model.SubWA, CreatedAt: *ts(10), Cancelled: true},
		{UserID: 1, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(30)},
	}
	rows := computeStandings(contest, links, subs, emptyFlags(), *ts(60), 0)
	cell := rows[0].Cells["A"]
	// negative case: the cancelled WA must not count as an attempt
	if cell.Attempts != 0 || !cell.Solved {
		t.Fatalf("cancelled WA must be invisible: %+v", cell)
	}
	if rows[0].PenaltyMS != 30*60*1000 {
		t.Fatalf("penalty must equal bare solve time: %d", rows[0].PenaltyMS)
	}
}

func TestComputeStandingsStarredUnrated(t *testing.T) {
	contest := mkContest(ts(0), ts(300), nil)
	links := []model.ContestProblem{{ContestID: 1, ProblemID: 1, Label: "A"}}
	subs := []model.Submission{
		{UserID: 1, ProblemID: 1, Status: model.SubWA, CreatedAt: *ts(10)},
		{UserID: 1, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(30)},
		{UserID: 2, ProblemID: 1, Status: model.SubAC, CreatedAt: *ts(40)},
	}
	flags := map[uint]model.ContestUserFlag{2: {UserID: 2, Starred: true}}
	rows := computeStandings(contest, links, subs, flags, *ts(60), 0)
	if rows[0].UserID != 1 || rows[0].Rank != 1 {
		t.Fatalf("rated user ranks first: %+v", rows[0])
	}
	star := rows[1]
	if !star.Starred || star.Rank != 0 || star.Solved != 1 {
		t.Fatalf("starred user must solve normally but carry no rank: %+v", star)
	}
}
