package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ysnb/oj/internal/model"
)

// requireDaemonToken guards /internal/* for judge daemons only.
func (s *Server) requireDaemonToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(raw) > len(prefix) && raw[len(prefix):] == s.Cfg.JWT.DaemonSecret {
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func (s *Server) listDaemons(c *gin.Context) {
	var daemons []model.JudgeDaemon
	s.DB.Order("name").Find(&daemons)
	// A stale heartbeat means the process is likely gone even if the row
	// still says online (e.g. hard power-off without graceful disconnect).
	for i := range daemons {
		d := &daemons[i]
		if d.Status == model.DaemonOnline &&
			(d.LastHeartbeat == nil || time.Since(*d.LastHeartbeat) > heartbeatStaleAfter) {
			d.Status = model.DaemonOffline
		}
	}
	queueLen, _ := s.Queue.Len(c.Request.Context())
	c.JSON(200, gin.H{"daemons": daemons, "queue_length": queueLen})
}

const heartbeatStaleAfter = 60 * time.Second

// rejudge resets a submission to PENDING and re-enqueues it.
func (s *Server) rejudge(c *gin.Context) {
	sub, ok := s.submissionByID(c)
	if !ok {
		c.JSON(404, gin.H{"error": "submission not found"})
		return
	}
	s.DB.Model(sub).Updates(map[string]any{
		"status": model.SubPending, "judged_at": nil, "lease_until": nil,
		"cases": "", "compile_message": "",
	})
	if err := s.Queue.Push(c.Request.Context(), uint64(sub.ID)); err != nil {
		c.JSON(500, gin.H{"error": "enqueue failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) listLanguages(c *gin.Context) {
	c.JSON(200, s.Langs.List())
}
