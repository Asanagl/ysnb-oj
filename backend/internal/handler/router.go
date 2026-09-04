// Package handler wires the REST API. Handlers are thin: validation and
// orchestration only; cross-cutting judging logic lives in judgehub.
package handler

import (
	"io"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/auth"
	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/judgehub"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/queue"
	"github.com/ysnb/oj/internal/wsq"
	"github.com/ysnb/oj/pkg/judge"
)

type Server struct {
	DB       *gorm.DB
	Cfg      *config.Config
	JWT      *auth.Manager
	Queue    queue.TaskQueue
	WS       *wsq.Hub
	Langs    *judge.Registry
	JudgeHub *judgehub.Hub
	// Submits throttles code submissions per user; defined here so handlers
	// share one limiter instance across requests.
	Submits *submitLimiter
}

// Router builds the complete gin engine.
func (s *Server) Router() *gin.Engine {
	if s.Submits == nil {
		s.Submits = newSubmitLimiter()
	}
	if s.Cfg.Mode == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// Access log through slog (JSON → journald) and panic recovery with
	// structured logging; gin.DefaultWriter/Recovery output used to bypass
	// the level system entirely.
	slogWriter := newGinSlogWriter()
	ginErrWriter := ginErrorWriter()
	gin.DefaultWriter = slogWriter
	gin.DefaultErrorWriter = ginErrWriter
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{Output: slogWriter}), gin.CustomRecovery(func(c *gin.Context, rec any) {
		slog.Error("gin panic recovered", "path", c.Request.URL.Path, "panic", rec)
		c.AbortWithStatus(500)
	}))
	// Behind nginx we are single-proxy: only trust loopback, otherwise a
	// forged X-Forwarded-For rewrites c.ClientIP() and defeats the login/
	// register per-IP limiters (P0 from the security review).
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	// Global body ceiling: uploads are ≤128MB zips plus multipart overhead;
	// everything else is far smaller. Prevents unbounded-body memory DoS.
	r.Use(bodyCap(160 << 20))
	// security headers on every reply (nginx adds a second layer at the edge)
	r.Use(securityHeaders())

	api := r.Group("/api/v1")
	api.Use(s.JWT.Middleware(), s.banGate())

	api.POST("/auth/login", capJSONBody(1<<20), s.loginLimiter(), s.login)
	api.POST("/auth/register", capJSONBody(1<<20), s.registerLimiter(), s.register)
	api.GET("/auth/me", auth.RequireAuth(), s.me)
	api.GET("/languages", s.listLanguages)

	// problems: browsing needs login (invite-gated community in V1)
	problems := api.Group("/problems", auth.RequireAuth())
	problems.GET("", s.listProblems)
	problems.GET("/:id", s.getProblem)
	setter := api.Group("/problems", auth.RequireAuth(), auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin), s.userWriteLimiter(20, 60))
	setter.POST("", s.createProblem)
	setter.PUT("/:id", s.updateProblem)
	setter.DELETE("/:id", s.deleteProblem)
	setter.POST("/:id/testdata", s.uploadTestdata)
	setter.GET("/:id/testdata", s.listTestdata)
	setter.POST("/:id/testdata/case", s.uploadSingleCase)
	setter.GET("/:id/testdata/:caseId/preview/:kind", s.previewCase)
	setter.DELETE("/:id/testdata/:caseId", s.deleteSingleCase)
	setter.POST("/:id/copy", s.copyProblem)
	setter.GET("/:id/export", s.exportProblem)
	setter.POST("/import", s.importProblem)
	setter.POST("/:id/checker", func(c *gin.Context) { s.uploadJudgeSource(c, "checker") })
	setter.POST("/:id/interactor", func(c *gin.Context) { s.uploadJudgeSource(c, "interactor") })
	// IOI partial credit: per-case score table management (setter+)
	setter.PUT("/:id/case-scores", s.setCaseScores)
	setter.GET("/:id/case-scores", s.listCaseScores)

	api.GET("/submissions", auth.RequireAuth(), s.listSubmissions)
	api.GET("/submissions/:id", auth.RequireAuth(), s.getSubmission)
	api.POST("/submissions", auth.RequireAuth(), s.createSubmission)
	api.GET("/users/me/stats", auth.RequireAuth(), s.myStats)
	api.GET("/users/:id/profile", auth.RequireAuth(), s.userProfile)
	api.GET("/users/search", auth.RequireAuth(), s.searchUsers)
	api.GET("/problems/:id/solutions", auth.RequireAuth(), s.listSolutions)
	api.POST("/problems/:id/solutions", auth.RequireAuth(), s.userWriteLimiter(10, 60), s.createSolution)
	api.PUT("/problems/:id/solutions/:sid", auth.RequireAuth(), s.userWriteLimiter(10, 60), s.updateSolution)
	api.DELETE("/problems/:id/solutions/:sid", auth.RequireAuth(), s.deleteSolution)

	// user problem creation with review flow (any logged-in user)
	create := api.Group("/my/problems", auth.RequireAuth(), s.userWriteLimiter(6, 60))
	create.POST("", s.createUserProblem)
	create.GET("", s.myProblems)
	create.PUT("/:id", s.updateMyProblem)

	// training 题单 (problem lists): read for all, manage for creator/setter+
	lists := api.Group("/lists", auth.RequireAuth())
	lists.GET("", s.listLists)
	lists.GET("/:id", s.getList)
	lists.POST("", auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin), s.createList)
	lists.PUT("/:id", auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin), s.updateList)
	lists.DELETE("/:id", auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin), s.deleteList)

	// training teams (训练小组): anyone can create; captain manages members
	teams := api.Group("/teams", auth.RequireAuth(), s.userWriteLimiter(20, 60))
	teams.GET("", s.listTeams)
	teams.POST("", s.createTeam)
	teams.GET("/:id", s.getTeam)
	teams.POST("/join/:code", s.joinTeam)
	teams.POST("/:id/leave", s.leaveTeam)
	teams.POST("/:id/members/:uid", s.pullMember)
	teams.DELETE("/:id/members/:uid", s.kickMember)
	teams.POST("/:id/captain/:uid", s.transferCaptain)
	teams.POST("/:id/invite", s.regenerateInvite)
	teams.POST("/:id/notices", s.createTeamNotice)
	teams.DELETE("/:id/notices/:nid", s.deleteTeamNotice)
	teams.GET("/:id/board", s.teamBoard)
	teams.POST("/:id/lists", s.shareList)
	teams.DELETE("/:id/lists/:lid", s.unshareList)

	api.GET("/contests", auth.RequireAuth(), s.listContests)
	api.GET("/contests/:id", auth.RequireAuth(), s.getContest)
	api.GET("/contests/:id/standings", auth.RequireAuth(), s.getStandings)
	api.GET("/contests/:id/standings.csv", auth.RequireAuth(), s.exportStandingsCSV)
	contests := api.Group("/contests", auth.RequireAuth(), auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin), s.userWriteLimiter(10, 60))
	contests.POST("", s.createContest)
	contests.PUT("/:id", s.updateContest)
	contests.POST("/:id/problems", s.setContestProblems)

	// Admin console backend. Two tiers:
	//   - staff tier (setter+): problem review queue and stat pages read-only;
	//   - admin tier: users, daemons, invites, rejudge, review actions.
	staff := api.Group("/admin", auth.RequireAuth(), auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin))
	staff.GET("/stats/overview", s.statsOverview)
	staff.GET("/stats/trend", s.statsTrend)
	staff.GET("/stats/concurrency", s.statsConcurrency)
	staff.GET("/stats/load", s.statsLoad)
	staff.GET("/problems/pending", s.pendingProblems)
	staff.POST("/problems/:id/review", s.reviewProblem)

	admin := api.Group("/admin", auth.RequireAuth(), auth.RequireRole(model.RoleAdmin, model.RoleSuperAdmin))
	admin.GET("/users", s.listUsers)
	admin.POST("/users/import", s.importUsers)
	admin.PUT("/users/:id/role", s.setUserRole)
	admin.PUT("/users/:id/profile", s.updateUser)
	admin.POST("/users/:id/ban", s.setUserBan)
	admin.POST("/users/:id/reset-password", s.resetUserPassword)
	admin.POST("/invite-codes", s.createInviteCode)
	admin.GET("/invite-codes", s.listInviteCodes)
	admin.DELETE("/invite-codes/:id", s.deleteInviteCode)
	admin.GET("/daemons", s.listDaemons)
	admin.POST("/rejudge/:id", s.rejudge)
	// log viewer: read-only journald tail of the two OJ units (constant
	// argv, admin-gated — see admin_logs.go's security model comment).
	admin.GET("/logs", s.adminLogs)

	// super-admin tier: granting/revoking admin & super_admin, and bans.
	// why: role escalation and ban decisions outrank ordinary administration;
	// a plain admin must not be able to mint another admin.
	super := api.Group("/admin", auth.RequireAuth(), s.requireSuperAdmin())
	super.PUT("/users/:id/role/super", s.setSuperRole)
	super.POST("/users/:id/ban/super", s.superSetUserBan)

	// judge (creator/admin) operations: cancel grades, cheat/star marks,
	// scoped rejudge, notices, manual freeze, time adjustment.
	jury := api.Group("/contests/:id", auth.RequireAuth())
	jury.POST("/register", s.registerContest)
	jury.GET("/my-registration", s.myRegistration)
	jury.PUT("/my-registration", s.editRegistration)
	jury.DELETE("/my-registration", s.cancelRegistration)
	jury.GET("/registrations", s.listRegistrations)
	jury.PUT("/registrations/:uid", s.adminEditRegistration)
	jury.DELETE("/registrations/:uid", s.adminDeleteRegistration)
	jury.GET("/notices", s.listNotices)
	jury.POST("/notices", s.createNotice)
	jury.DELETE("/notices/:nid", s.deleteNotice)
	jury.GET("/flags", s.listFlags)
	jury.GET("/participants", s.listParticipants)
	jury.GET("/users/search", s.searchContestUsers)
	jury.PUT("/users/:uid/flags", s.setUserFlags)
	jury.POST("/submissions/:sid/cancel", s.setContestSubmissionCancel)
	jury.POST("/submissions/:sid/rejudge", s.rejudgeContestSubmission)
	jury.POST("/users/:uid/problems/:pid/cancel", s.cancelUserProblem)
	jury.POST("/problems/:pid/rejudge", s.rejudgeContestProblem)
	jury.PUT("/freeze", s.setContestFreeze)
	jury.PUT("/time", s.setContestTime)
	jury.PUT("/reveal", s.setContestReveal)

	// Public external API (bots / dashboards / CLI / cph bridge): anonymous
	// read + optional API Key. Isolated from the JWT-scoped group on
	// purpose — see public_api.go's header comment.
	public := api.Group("/public", s.publicAPIAuth())
	public.GET("/users/:id", s.publicUserSummary)
	public.GET("/problems", s.publicProblems)
	public.GET("/problems/:id/samples", s.publicProblemSamples)
	public.GET("/problems/:id/cph", s.publicProblemCph)

	// API key management lives behind the admin console (JWT + admin role).
	keyAdmin := api.Group("/admin/api-keys", auth.RequireAuth(), auth.RequireRole(model.RoleAdmin, model.RoleSuperAdmin))
	keyAdmin.POST("", s.createAPIKey)
	keyAdmin.GET("", s.listAPIKeys)
	keyAdmin.POST("/:id/revoke", s.revokeAPIKey)

	// Plugin system (M5): external problem crawlers, event hooks and the
	// external-practice sync (刷题统计报表). Management endpoints are
	// admin-tier; user-facing bind/report endpoints are self-service.
	staff.GET("/plugins", s.pluginList)
	admin.GET("/plugins/sources", s.pluginSources)
	setter2 := api.Group("/external/problems", auth.RequireAuth(), auth.RequireRole(model.RoleSetter, model.RoleAdmin, model.RoleSuperAdmin), s.userWriteLimiter(6, 60))
	setter2.POST("/preview", s.previewExternalProblem)
	setter2.POST("/import", s.importExternalProblem)
	// event hook target management (admin): webhook endpoints for bots
	admin.GET("/hooks", s.listHookTargets)
	admin.PUT("/hooks/:name", s.setHookTarget)
	admin.DELETE("/hooks/:name", s.deleteHookTarget)
	// external practice: self-service bind/sync + report; sync crawls an
	// external site, so it gets the tightest budget of the write classes.
	ext := api.Group("/external", auth.RequireAuth())
	ext.GET("/bindings", s.myExternalBindings)
	ext.PUT("/bindings", s.userWriteLimiter(4, 60), s.upsertExternalBinding)
	ext.DELETE("/bindings/:platform", s.deleteExternalBinding)
	ext.POST("/bindings/:platform/sync", s.userWriteLimiter(4, 60), s.syncExternalBinding)
	api.GET("/external/report/:id", auth.RequireAuth(), s.externalReport)

	// judge daemons authenticate with the shared daemon secret
	internal := api.Group("/internal", s.requireDaemonToken())
	internal.GET("/testdata/:problemId/:caseIndex/:kind", s.serveTestdata)

	// WebSocket upgrades authenticate via ?token=: browsers cannot set the
	// Authorization header on a WebSocket, so RequireAuth would 401 forever.
	// admin:* topics require the admin role (BUG-005).
	api.GET("/ws", s.wsAuth(), s.WS.Handler(wsTopicAuth))
	return r
}

// wsTopicAuth restricts infrastructure topics to admins (super_admin
// included via isAdminRole); ordinary topics (submission:N, contest:N) are
// fine for any logged-in user.
func wsTopicAuth(role, topic string) bool {
	if strings.HasPrefix(topic, "admin:") {
		return isAdminRole(role)
	}
	return true
}

// wsAuth validates the query-string JWT before the connection upgrades.
func (s *Server) wsAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := s.JWT.Parse(c.Query("token"))
		if err != nil {
			c.AbortWithStatus(401)
			return
		}
		c.Set(auth.ClaimsKey, claims)
		c.Next()
	}
}

// ginSlogWriter adapts gin's io.Writer logging into slog at INFO (access)
// / ERROR (default error writer) level, so request logs share the JSON
// pipeline instead of writing raw lines to stdout.
type ginSlogWriter struct{ errLevel bool }

func newGinSlogWriter() *ginSlogWriter { return &ginSlogWriter{} }

func ginErrorWriter() io.Writer { return &ginSlogWriter{errLevel: true} }

func (w *ginSlogWriter) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if w.errLevel {
		slog.Error("gin", "raw", line)
	} else {
		slog.Info("http access", "raw", line)
	}
	return len(p), nil
}

var _ io.Writer = (*ginSlogWriter)(nil)
