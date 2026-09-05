// Package judgehub is the API-side gRPC endpoint judge daemons connect to.
// It owns the task queue → daemon dispatch loop, per-submission leases and
// result finalization, and mirrors daemon liveness into the admin monitor.
package judgehub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/queue"
	"github.com/ysnb/oj/internal/wsq"
	pb "github.com/ysnb/oj/pb"
)

const (
	leaseDuration   = 15 * time.Minute
	requeueInterval = 30 * time.Second
	heartbeatLogGap = 20 * time.Second // treat daemons silent longer as suspect
)

type daemonConn struct {
	info      *model.JudgeDaemon
	send      chan *pb.ApiMessage
	active    atomic.Int32
	done      chan struct{}
	closeOnce sync.Once
}

func (d *daemonConn) close() {
	d.closeOnce.Do(func() { close(d.done) })
}

type Hub struct {
	pb.UnimplementedJudgeRelayServer
	DB       *gorm.DB
	Queue    queue.TaskQueue
	WS       *wsq.Hub
	Cfg      *config.Config
	mu       sync.Mutex
	conns    map[string]*daemonConn // daemon name → live connection
	inflight map[uint64]string      // submission id → daemon name
}

func New(db *gorm.DB, q queue.TaskQueue, ws *wsq.Hub, cfg *config.Config) *Hub {
	return &Hub{DB: db, Queue: q, WS: ws, Cfg: cfg,
		conns: map[string]*daemonConn{}, inflight: map[uint64]string{}}
}

// StartRequeueScanner resets submissions whose daemon died mid-judge; it is
// the safety net under the per-connection requeue path.
func (h *Hub) StartRequeueScanner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(requeueInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.requeueExpired()
			}
		}
	}()
}

func (h *Hub) requeueExpired() {
	deadline := time.Now().Add(-leaseDuration)
	var subs []model.Submission
	if err := h.DB.Where("status = ? AND lease_until IS NOT NULL AND lease_until < ?",
		model.SubJudging, deadline).Limit(100).Find(&subs).Error; err != nil {
		return
	}
	for i := range subs {
		h.requeue(&subs[i], "lease expired")
	}
}

func (h *Hub) requeue(sub *model.Submission, reason string) {
	now := time.Now()
	h.DB.Model(sub).Updates(map[string]any{"status": model.SubPending, "lease_until": nil})
	if err := h.Queue.Push(context.Background(), uint64(sub.ID)); err != nil {
		slog.Error("judgehub requeue push failed", "submission", sub.ID, "err", err)
	}
	slog.Info("submission requeued", "submission", sub.ID, "reason", reason)
	h.WS.Publish(fmt.Sprintf("submission:%d", sub.ID), map[string]any{
		"id": sub.ID, "status": model.SubPending, "reason": reason, "at": now,
	})
}

// Connect implements the bidi stream. Auth happens on the first frame.
func (h *Hub) Connect(stream pb.JudgeRelay_ConnectServer) error {
	reg, err := h.awaitRegister(stream)
	if err != nil {
		return err
	}
	conn := h.registerConn(stream, reg)
	defer h.dropConn(reg.DaemonName, conn)

	go h.dispatchLoop(stream.Context(), conn)
	h.readLoop(stream, conn)
	return nil
}

func (h *Hub) awaitRegister(stream pb.JudgeRelay_ConnectServer) (*pb.Register, error) {
	msg, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	reg := msg.GetRegister()
	if reg == nil || reg.GetToken() != h.Cfg.JWT.DaemonSecret || reg.GetDaemonName() == "" {
		return nil, io.EOF
	}
	return reg, nil
}

func (h *Hub) registerConn(stream pb.JudgeRelay_ConnectServer, reg *pb.Register) *daemonConn {
	langs, _ := json.Marshal(reg.GetLanguages())
	now := time.Now()
	info := &model.JudgeDaemon{
		Name: reg.GetDaemonName(), Status: model.DaemonOnline,
		Capacity: int(reg.GetCapacity()), Languages: string(langs),
		LastHeartbeat: &now,
	}
	h.DB.Where("name = ?", info.Name).Assign(info).FirstOrCreate(info)

	conn := &daemonConn{info: info, send: make(chan *pb.ApiMessage, 64), done: make(chan struct{})}
	h.mu.Lock()
	if old, ok := h.conns[info.Name]; ok {
		old.close() // duplicate login replaces the stale stream
	}
	h.conns[info.Name] = conn
	h.mu.Unlock()
	slog.Info("daemon registered", "daemon", info.Name, "capacity", info.Capacity,
		"languages", info.Languages, "version", reg.GetVersion())

	go sendLoop(stream, conn)
	return conn
}

func sendLoop(stream pb.JudgeRelay_ConnectServer, conn *daemonConn) {
	for {
		select {
		case <-conn.done:
			return
		case <-stream.Context().Done():
			return
		case msg := <-conn.send:
			if err := stream.Send(msg); err != nil {
				return
			}
		}
	}
}

// dropConn removes the connection, marks the daemon offline and requeues
// everything still in flight on it.
func (h *Hub) dropConn(name string, conn *daemonConn) {
	conn.close()
	h.mu.Lock()
	if h.conns[name] == conn {
		delete(h.conns, name)
	}
	orphaned := make([]uint64, 0, len(h.inflight))
	for subID, owner := range h.inflight {
		if owner == name {
			orphaned = append(orphaned, subID)
			delete(h.inflight, subID)
		}
	}
	h.mu.Unlock()

	h.DB.Model(&model.JudgeDaemon{}).Where("name = ?", name).
		Updates(map[string]any{"status": model.DaemonOffline, "active_tasks": 0})
	slog.Warn("daemon disconnected", "daemon", name, "orphaned_tasks", len(orphaned))
	for _, subID := range orphaned {
		sub := &model.Submission{}
		if err := h.DB.First(sub, subID).Error; err == nil && sub.Status == model.SubJudging {
			h.requeue(sub, "daemon disconnected")
		}
	}
}

// readLoop consumes heartbeats and results until the stream breaks.
func (h *Hub) readLoop(stream pb.JudgeRelay_ConnectServer, conn *daemonConn) {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return
		}
		switch body := msg.Body.(type) {
		case *pb.DaemonMessage_Heartbeat:
			h.handleHeartbeat(conn, body.Heartbeat)
		case *pb.DaemonMessage_Result:
			h.handleResult(conn, body.Result)
		case *pb.DaemonMessage_CaseProgress:
			h.handleCaseProgress(conn, body.CaseProgress)
		case *pb.DaemonMessage_Register:
			// protocol violation mid-stream; ignore
		}
	}
}

// handleCaseProgress fans one finished test case out to the submission's WS
// topic so the owner's card lights the dot up Hydro-style. Gated on the
// inflight table so a confused daemon cannot broadcast progress for
// submissions it was not leased; the WS topic itself is owner-only (see
// handler's wsTopicAuth). Delivery is best-effort — the final TaskResult is
// authoritative and the card also has a poll fallback.
func (h *Hub) handleCaseProgress(conn *daemonConn, cp *pb.CaseProgress) {
	subID := cp.GetSubmissionId()
	h.mu.Lock()
	owner, ok := h.inflight[subID]
	h.mu.Unlock()
	if !ok || owner != conn.info.Name {
		return
	}
	h.WS.Publish(fmt.Sprintf("submission:%d", subID), map[string]any{
		"kind":      "case",
		"index":     cp.GetIndex(),
		"status":    cp.GetStatus(),
		"time_ms":   cp.GetTimeMs(),
		"memory_kb": cp.GetMemoryKb(),
		"score":     cp.GetScore(),
		"total":     cp.GetTotal(),
	})
}

func (h *Hub) handleHeartbeat(conn *daemonConn, hb *pb.Heartbeat) {
	now := time.Now()
	// why status=online here too: a dying API's dropConn can land its
	// offline write AFTER a reconnecting daemon registered on the new API
	// (graceful stop waits up to 10s); heartbeats prove liveness, so let
	// them heal the status within one tick.
	h.DB.Model(conn.info).Updates(map[string]any{
		"last_heartbeat": now, "active_tasks": hb.GetActiveTasks(),
		"status":       model.DaemonOnline,
		"load1":        hb.GetLoad1(),
		"mem_used_mb":  hb.GetMemUsedMb(),
		"mem_total_mb": hb.GetMemTotalMb(),
	})
	h.WS.Publish("admin:daemons", map[string]any{
		"name": conn.info.Name, "status": model.DaemonOnline,
		"active_tasks": hb.GetActiveTasks(), "capacity": conn.info.Capacity,
		"load1": hb.GetLoad1(), "mem_used_mb": hb.GetMemUsedMb(), "mem_total_mb": hb.GetMemTotalMb(),
	})
}

func (h *Hub) handleResult(conn *daemonConn, tr *pb.TaskResult) {
	conn.active.Add(-1)
	h.mu.Lock()
	delete(h.inflight, tr.GetSubmissionId())
	h.mu.Unlock()
	h.finalize(tr)
}
