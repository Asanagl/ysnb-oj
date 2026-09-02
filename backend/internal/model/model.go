// Package model defines all GORM entities. JSON-ish fields are stored as
// plain text so the same schema works on both PostgreSQL (prod) and SQLite
// (dev); the trade-off is documented in docs/architecture.md.
package model

import "time"

// Roles, statuses and visibility enums live here so every package shares
// one spelling of the vocabulary.
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleSetter     = "setter"
	RoleUser       = "user"

	VisibilityHidden  = "hidden"  // only setters/admin see it
	VisibilityMembers = "members" // logged-in users see it
	VisibilityPublic  = "public"  // visible to anyone with an account (no anonymous bank in V1)

	JudgeModeDefault     = "default"     // diff against answer
	JudgeModeSPJ         = "spj"         // run custom checker
	JudgeModeInteractive = "interactive" // interactor talks to user program

	SubPending   = "PENDING"
	SubCompiling = "COMPILING"
	SubJudging   = "JUDGING"
	SubAC        = "AC"
	SubWA        = "WA"
	SubTLE       = "TLE"
	SubMLE       = "MLE"
	SubRE        = "RE"
	SubCE        = "CE"
	SubSE        = "SE" // system error on judge side

	ContestModeACM = "acm"
	ContestModeIOI = "ioi" // partial credit: per-case scores, best-of per problem, ranked by total score

	DaemonOnline  = "online"
	DaemonOffline = "offline"
)

type User struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Username     string `gorm:"uniqueIndex;size:64" json:"username"`
	PasswordHash string `json:"-"`
	Nickname     string `gorm:"size:64" json:"nickname"`
	Role         string `gorm:"size:16;index" json:"role"`
	StudentNo    string `gorm:"size:32;index" json:"student_no"`
	// Banned blocks login and submission (JWTs are short-lived; the ban is
	// enforced on the live request path, not just at token issue time).
	Banned      bool       `json:"banned"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// InvitationCode gates registration; admins mint them, invitees spend them.
type InvitationCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"uniqueIndex;size:32" json:"code"`
	MaxUses   int        `json:"max_uses"`
	UsedCount int        `json:"used_count"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedBy uint       `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

type Sample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
	Note   string `json:"note,omitempty"`
}

type Problem struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Title       string `gorm:"size:200;index" json:"title"`
	StatementMD string `gorm:"type:text" json:"statement_md"`
	InputDesc   string `gorm:"type:text" json:"input_desc"`
	OutputDesc  string `gorm:"type:text" json:"output_desc"`
	Hint        string `gorm:"type:text" json:"hint"`
	Source      string `gorm:"size:200" json:"source"`
	Tags        string `gorm:"type:text" json:"tags"`    // JSON array of strings
	Samples     string `gorm:"type:text" json:"samples"` // JSON []Sample
	TimeLimitMS int    `json:"time_limit_ms"`
	MemLimitMB  int    `json:"mem_limit_mb"`
	Visibility  string `gorm:"size:16;index" json:"visibility"`
	JudgeMode   string `gorm:"size:16" json:"judge_mode"`
	// ContestID marks an exclusive contest problem: it never appears in the
	// public problem bank and is only reachable through its contest
	// (independent-copy model — each contest owns its own problems).
	ContestID *uint `gorm:"index" json:"contest_id"`
	// ReviewStatus tracks user-created problems: draft (editable), pending
	// (awaiting admin approval), approved (public), rejected. Admin/setter
	// creations skip review entirely (approved on create).
	ReviewStatus  string    `gorm:"size:16;index;default:approved" json:"review_status"`
	CheckerSource string    `gorm:"type:text" json:"-"` // compiled per-problem, never echoed to clients
	InteractorSrc string    `gorm:"type:text" json:"-"`
	CreatedBy     uint      `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProblemSolution is one 题解 floor under a problem: Markdown body, optional
// reply-to parent, and an official pin by the problem author or admins.
type ProblemSolution struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProblemID  uint      `gorm:"index" json:"problem_id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	ParentID   *uint     `gorm:"index" json:"parent_id"` // nil = top-level floor
	Title      string    `gorm:"size:200" json:"title"`
	BodyMD     string    `gorm:"type:text" json:"body_md"`
	IsOfficial bool      `gorm:"index" json:"is_official"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TestCase is metadata only; the actual .in/.out files live under
// DataDir/testdata/<problemID>/<caseIndex>/{input,answer}.
type TestCase struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// ProblemID groups the case; case_index avoids PG's reserved word `index`.
	ProblemID  uint      `gorm:"index" json:"problem_id"`
	CaseIndex  int       `gorm:"column:case_index" json:"index"`
	InputSHA   string    `gorm:"size:64" json:"input_sha"`
	AnswerSHA  string    `gorm:"size:64" json:"answer_sha"`
	InputSize  int64     `json:"input_size"`
	AnswerSize int64     `json:"answer_size"`
	// Score is the IOI partial credit for passing this case (0 in ACM mode;
	// sum over cases should equal the problem's full score, e.g. 100).
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// CaseResult is one test case outcome inside a judged submission.
type CaseResult struct {
	Index   int    `json:"index"`
	Status  string `json:"status"`
	TimeMS  int64  `json:"time_ms"`
	MemKB   int64  `json:"mem_kb"`
	Message string `json:"message,omitempty"` // checker verdict text, small
	Score   int    `json:"score,omitempty"`   // per-case credit earned (IOI)
}

type Submission struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UserID         uint       `gorm:"index" json:"user_id"`
	ProblemID      uint       `gorm:"index" json:"problem_id"`
	ContestID      *uint      `gorm:"index" json:"contest_id"` // nil during training
	Language       string     `gorm:"size:32" json:"language"`
	CodePath       string     `json:"-"` // path under DataDir; not exposed
	CodeSize       int        `json:"code_size"`
	Status         string     `gorm:"size:16;index" json:"status"`
	// Score is the IOI partial credit for this submission (sum of per-case
	// scores); always 0 in ACM mode, where verdict is the only currency.
	Score          int        `json:"score"`
	TimeMS         int64      `json:"time_ms"`
	MemoryKB       int64      `json:"memory_kb"`
	CompileMessage string     `gorm:"type:text" json:"compile_message,omitempty"`
	Cases          string     `gorm:"type:text" json:"-"` // JSON []CaseResult
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
	JudgedAt       *time.Time `json:"judged_at"`
	// LeaseUntil bounds how long a daemon may hold this submission; the
	// requeue scanner resets crashed daemons' leases back to PENDING.
	LeaseUntil *time.Time `gorm:"index" json:"-"`
	// IsPractice marks a post-contest 补题 submission: it never changes the
	// frozen contest standings.
	IsPractice bool `gorm:"index" json:"is_practice"`
	// Cancelled marks a jury-nullified submission: it stays visible with a
	// badge but is excluded from standings math (attempts and solves).
	Cancelled bool `gorm:"index" json:"cancelled"`
}

// ContestUserFlag holds per-contest jury marks for a user: 打星 (starred,
// shown on standings without occupying a rank) and 作弊 (cheated, row kept
// but flagged and excluded from scoring). Both are reversible.
type ContestUserFlag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ContestID uint      `gorm:"uniqueIndex:idx_contest_user" json:"contest_id"`
	UserID    uint      `gorm:"uniqueIndex:idx_contest_user" json:"user_id"`
	Starred   bool      `json:"starred"`
	Cheated   bool      `json:"cheated"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContestNotice is a jury announcement shown on the contest page.
type ContestNotice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ContestID uint      `gorm:"index" json:"contest_id"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Contest struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Title       string     `gorm:"size:200" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Mode        string     `gorm:"size:16" json:"mode"`
	Visibility  string     `gorm:"size:16" json:"visibility"` // public | hidden
	StartTime   time.Time  `json:"start_time"`
	EndTime     time.Time  `json:"end_time"`
	FreezeTime  *time.Time `json:"freeze_time"` // scoreboard frozen from here
	// Freeze configuration chosen at creation: NoFreeze disables the whole
	// concept (freeze_time and manual freeze become inert); NoManualFreeze
	// keeps the automatic schedule but forbids the jury's mid-contest lever.
	FreezeEnabled  bool       `gorm:"default:true" json:"freeze_enabled"`
	NoManualFreeze bool       `json:"no_manual_freeze"`
	ManualFrozen   bool       `json:"manual_frozen"`
	ManualFrozenAt *time.Time `json:"manual_frozen_at"`
	// 滚榜: number of bottom-ranked rows whose frozen verdicts are revealed.
	RevealCount int `json:"reveal_count"`
	// RequireRegistration (nil for legacy contests = not required): when set,
	// submissions inside the contest need a prior registration entry.
	RequireRegistration *bool `json:"require_registration"`
	// 组队赛 (ICPC 三人一队): when TeamMode is on, contestants register as a
	// whole team (captain registers with the team id) and the standings
	// aggregate per team. TeamCapacity caps members per registered team
	// (0 = unset; default 3 at creation).
	TeamMode     bool      `json:"team_mode"`
	TeamCapacity int       `json:"team_capacity"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// ContestRegistration is a contestant's 报名 entry: team type (official or
// starred) chosen at registration; type changes afterwards need the jury.
// 组队赛 (Contest.TeamMode): the captain registers once with TeamID set and
// TeamName = team name; members are snapshotted into one row per member with
// the same TeamID so the standings can aggregate per team.
type ContestRegistration struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ContestID uint   `gorm:"uniqueIndex:idx_reg_contest_user" json:"contest_id"`
	UserID    uint   `gorm:"uniqueIndex:idx_reg_contest_user" json:"user_id"`
	TeamName  string `gorm:"size:100" json:"team_name"`
	TeamType  string `gorm:"size:16" json:"team_type"` // official | starred
	// TeamID links a 组队赛 registration to a Team (0 = individual entry).
	TeamID    uint      `gorm:"index;default:0" json:"team_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ContestProblem struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	ContestID  uint   `gorm:"index" json:"contest_id"`
	ProblemID  uint   `gorm:"index" json:"problem_id"`
	Label      string `gorm:"size:4" json:"label"` // A, B, C ...
	OrderIndex int    `json:"order_index"`
}

// JudgeDaemon tracks connected judge machines for the admin monitor.
type JudgeDaemon struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `gorm:"uniqueIndex;size:64" json:"name"`
	Status        string     `gorm:"size:16" json:"status"`
	Capacity      int        `json:"capacity"`
	ActiveTasks   int        `json:"active_tasks"`
	Languages     string     `gorm:"type:text" json:"languages"` // JSON names
	LastHeartbeat *time.Time `json:"last_heartbeat"`
	// Load telemetry from heartbeats; zeros mean "not reported" (pre-upgrade
	// daemons or non-Linux dev hosts).
	Load1      float64   `json:"load1"`
	MemUsedMB  float64   `json:"mem_used_mb"`
	MemTotalMB float64   `json:"mem_total_mb"`
	CreatedAt  time.Time `json:"created_at"`
}

// ProblemList is a training 题单: an ordered list of problems any logged-in
// user can browse (visibility is site-wide public by design). Progress is
// derived from submissions, not stored — no join table for "done".
type ProblemList struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Title       string    `gorm:"size:200" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProblemListItem is one entry on a 题单. Why a separate row (not a JSON
// array on ProblemList): stable ordering, per-problem notes, and later
// per-team assignment hooks.
type ProblemListItem struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	ListID     uint   `gorm:"index:idx_list_item" json:"list_id"`
	ProblemID  uint   `gorm:"index" json:"problem_id"`
	OrderIndex int    `json:"order_index"`
	Note       string `gorm:"size:200" json:"note"` // e.g. "week1", "必做"
}

// Team is an ICPC-style 训练小组: captain + members, configurable capacity
// (default 3, the ICPC team size), join by invite code or captain pull.
// TeamMode is the display style for scores: "icpc" shows a virtual-person
// combined ranking of the whole team.
type Team struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"size:100;uniqueIndex" json:"name"`
	Bio        string    `gorm:"size:300" json:"bio"`
	Capacity   int       `gorm:"default:3" json:"capacity"` // 1..5, default 3 (ICPC)
	CaptainID  uint      `gorm:"index" json:"captain_id"`
	InviteCode string    `gorm:"size:32;uniqueIndex" json:"invite_code"`
	CreatedAt  time.Time `json:"created_at"`
}

// TeamMember joins a user into a team. unique index prevents double-join and
// joining two teams is allowed (practice groups overlap) — capacity is the
// only hard limit.
type TeamMember struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	TeamID   uint      `gorm:"uniqueIndex:idx_team_user" json:"team_id"`
	UserID   uint      `gorm:"uniqueIndex:idx_team_user" json:"user_id"`
	JoinedAt time.Time `json:"joined_at"`
}

// TeamNotice is a captain-only announcement shown on the team page.
type TeamNotice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TeamID    uint      `gorm:"index" json:"team_id"`
	UserID    uint      `json:"user_id"` // author (captain)
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamList shares a 题单 with a team: members see it on the team page.
type TeamList struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TeamID    uint      `gorm:"uniqueIndex:idx_team_list" json:"team_id"`
	ListID    uint      `gorm:"uniqueIndex:idx_team_list" json:"list_id"`
	CreatedAt time.Time `json:"created_at"`
}
