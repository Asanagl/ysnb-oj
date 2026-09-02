package judgehub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/plugin"
	pb "github.com/ysnb/oj/pb"
)

// RequeuePending re-enqueues every PENDING submission at startup. why: the
// queue is best-effort (in-memory in dev; even Redis lists can be drained
// without acks), and the database is the source of truth — without this an
// API restart strands queued submissions in PENDING forever.
func (h *Hub) RequeuePending(ctx context.Context) {
	var subs []model.Submission
	if err := h.DB.Where("status = ?", model.SubPending).Order("id").Find(&subs).Error; err != nil {
		log.Printf("[judgehub] requeue-pending query: %v", err)
		return
	}
	for i := range subs {
		if err := h.Queue.Push(ctx, uint64(subs[i].ID)); err != nil {
			log.Printf("[judgehub] requeue push %d: %v", subs[i].ID, err)
			return
		}
	}
	if len(subs) > 0 {
		log.Printf("[judgehub] re-enqueued %d pending submissions at startup", len(subs))
	}
}

// dispatchLoop pulls PENDING submissions and pushes them to this daemon as
// long as it has spare capacity. It exits when the stream dies.
func (h *Hub) dispatchLoop(ctx context.Context, conn *daemonConn) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.done:
			return
		case <-time.After(200 * time.Millisecond):
		}
		if int(conn.active.Load()) >= conn.info.Capacity {
			continue
		}
		id, ok, err := h.Queue.Pop(ctx)
		if err != nil || !ok {
			if err != nil && ctx.Err() == nil {
				log.Printf("[judgehub] queue pop: %v", err)
			}
			continue
		}
		if !h.dispatchOne(ctx, conn, id) {
			return
		}
	}
}

// dispatchOne loads the submission, marks it JUDGING under lease and sends
// it; on any failure the submission goes back to the queue.
func (h *Hub) dispatchOne(ctx context.Context, conn *daemonConn, subID uint64) bool {
	task, err := h.buildTask(subID)
	if err != nil {
		log.Printf("[judgehub] build task %d: %v", subID, err)
		h.requeue(&model.Submission{ID: uint(subID), Status: model.SubJudging}, "build failed")
		return true
	}
	now := time.Now()
	lease := now.Add(leaseDuration)
	if err := h.DB.Model(&model.Submission{}).Where("id = ?", subID).Updates(map[string]any{
		"status": model.SubJudging, "lease_until": lease,
	}).Error; err != nil {
		log.Printf("[judgehub] mark judging %d: %v", subID, err)
		return true
	}
	conn.active.Add(1)
	h.mu.Lock()
	h.inflight[subID] = conn.info.Name
	h.mu.Unlock()
	select {
	case <-ctx.Done():
		return false
	case <-conn.done:
		return false
	case conn.send <- &pb.ApiMessage{Body: &pb.ApiMessage_Dispatch{Dispatch: &pb.TaskDispatch{Task: task}}}:
		h.WS.Publish(fmt.Sprintf("submission:%d", subID), map[string]any{
			"id": subID, "status": model.SubJudging, "at": now,
		})
		return true
	}
}

// buildTask assembles the judge task: code, problem limits, judge-mode
// sources and the testdata manifest (blobs fetched by the daemon on demand).
func (h *Hub) buildTask(subID uint64) (*pb.JudgeTask, error) {
	sub := &model.Submission{}
	if err := h.DB.First(sub, subID).Error; err != nil {
		return nil, fmt.Errorf("submission %d: %w", subID, err)
	}
	prob := &model.Problem{}
	if err := h.DB.First(prob, sub.ProblemID).Error; err != nil {
		return nil, fmt.Errorf("problem %d: %w", sub.ProblemID, err)
	}
	var cases []model.TestCase
	if err := h.DB.Where("problem_id = ?", prob.ID).Order("case_index").Find(&cases).Error; err != nil {
		return nil, err
	}
	code, err := os.ReadFile(sub.CodePath)
	if err != nil {
		return nil, fmt.Errorf("read code: %w", err)
	}
	task := &pb.JudgeTask{
		SubmissionId:     subID,
		ProblemId:        uint64(prob.ID),
		LanguageId:       sub.Language,
		Code:             code,
		TimeLimitMs:      int64(prob.TimeLimitMS),
		MemLimitMb:       int64(prob.MemLimitMB),
		JudgeMode:        prob.JudgeMode,
		CheckerSource:    []byte(prob.CheckerSource),
		InteractorSource: []byte(prob.InteractorSrc),
		FetchBase:        h.Cfg.FetchBase,
		FetchToken:       h.Cfg.JWT.DaemonSecret,
		// contest submissions stop at the first failing case (ICPC style:
		// the verdict is what matters, and skipping the remaining cases
		// reclaims judge capacity); training keeps full case feedback.
		StopOnFail: sub.ContestID != nil && !sub.IsPractice,
	}
	for _, tc := range cases {
		task.Cases = append(task.Cases, &pb.CaseRef{
			Index:        int32(tc.CaseIndex),
			InputSha256:  tc.InputSHA,
			AnswerSha256: tc.AnswerSHA,
			InputSize:    tc.InputSize,
			AnswerSize:   tc.AnswerSize,
			// IOI partial credit rides with each case; zero in ACM contests.
			Score: int32(tc.Score),
		})
	}
	return task, nil
}

// finalize persists the daemon's verdict and fans out WS updates.
func (h *Hub) finalize(tr *pb.TaskResult) {
	sub := &model.Submission{}
	if err := h.DB.First(sub, tr.GetSubmissionId()).Error; err != nil {
		log.Printf("[judgehub] finalize missing submission %d", tr.GetSubmissionId())
		return
	}
	now := time.Now()
	casesJSON, _ := json.Marshal(toModelCases(tr.GetCases()))
	compileMsg := tr.GetCompileMessage()
	// why: SE details used to be dropped, leaving unexplainable verdicts in
	// the admin UI; surface them where users and operators can see them.
	if tr.GetError() != "" {
		log.Printf("[judgehub] submission %d SE: %s", sub.ID, tr.GetError())
		compileMsg = compileMsg + "\n[judge SE] " + tr.GetError()
	}
	updates := map[string]any{
		"status":          tr.GetStatus(),
		"time_ms":         tr.GetTimeMs(),
		"memory_kb":       tr.GetMemoryKb(),
		"score":           tr.GetScore(), // IOI partial credit; 0 in ACM mode
		"compile_message": compileMsg,
		"cases":           string(casesJSON),
		"judged_at":       now,
		"lease_until":     nil,
	}
	if err := h.DB.Model(sub).Updates(updates).Error; err != nil {
		log.Printf("[judgehub] finalize %d: %v", sub.ID, err)
		return
	}
	payload := map[string]any{
		"id": sub.ID, "status": tr.GetStatus(), "time_ms": tr.GetTimeMs(),
		"memory_kb": tr.GetMemoryKb(), "score": tr.GetScore(),
		"problem_id": sub.ProblemID,
		"contest_id": sub.ContestID, "user_id": sub.UserID, "at": now,
	}
	h.WS.Publish(fmt.Sprintf("submission:%d", sub.ID), payload)
	if sub.ContestID != nil {
		h.WS.Publish(fmt.Sprintf("contest:%d", *sub.ContestID), map[string]any{
			"kind": "standings-dirty", "at": now,
		})
	}
	// plugin event hooks (webhooks for bots/dashboards) — emitted on a
	// detached goroutine inside the registry, never blocking judging.
	plugin.EmitJudgeEvent(payload)
}

func toModelCases(cases []*pb.CaseResult) []model.CaseResult {
	out := make([]model.CaseResult, 0, len(cases))
	for _, c := range cases {
		out = append(out, model.CaseResult{
			Index: int(c.GetIndex()), Status: c.GetStatus(),
			TimeMS: c.GetTimeMs(), MemKB: c.GetMemoryKb(), Message: c.GetMessage(),
			Score: int(c.GetScore()),
		})
	}
	return out
}
