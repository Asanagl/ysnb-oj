// Package daemon is the judge-side client: it dials the API's JudgeRelay,
// registers itself, pulls dispatched tasks into a bounded worker pool and
// streams results back, reconnecting with backoff when the stream breaks.
package daemon

import (
	"context"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/sysload"
	pb "github.com/ysnb/oj/pb"
	"github.com/ysnb/oj/pkg/judge"
)

type Daemon struct {
	Cfg     *config.Config
	Service *judge.Service

	active atomic.Int32
	// sendMu serializes SendMsg on the shared gRPC stream: grpc forbids
	// concurrent Send from heartbeat and result goroutines (observed as
	// silent heartbeat death in cloud E2E).
	sendMu sync.Mutex
}

// send is the only sanctioned way to write to the stream.
func (d *Daemon) send(stream pb.JudgeRelay_ConnectClient, msg *pb.DaemonMessage) error {
	d.sendMu.Lock()
	defer d.sendMu.Unlock()
	return stream.Send(msg)
}

// Run blocks forever, maintaining the connection and processing tasks.
func (d *Daemon) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := d.session(ctx); err != nil {
			log.Printf("[daemon] session ended: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (d *Daemon) session(ctx context.Context) error {
	conn, err := grpc.NewClient(d.Cfg.Judge.APIEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	// why fresh ctx per session: a dead stream must also unblock in-flight
	// dispatch reads, and the outer Run loop owns process lifetime.
	stream, err := pb.NewJudgeRelayClient(conn).Connect(ctx)
	if err != nil {
		return err
	}
	log.Printf("[daemon] connected to %s", d.Cfg.Judge.APIEndpoint)

	langs := d.languageIDs()
	if err := stream.Send(&pb.DaemonMessage{Body: &pb.DaemonMessage_Register{Register: &pb.Register{
		DaemonName: d.Cfg.Judge.DaemonName,
		Token:      d.Cfg.Judge.DaemonToken,
		Capacity:   int32(d.Cfg.Judge.MaxParallel),
		Languages:  langs,
		Version:    "v1",
	}}}); err != nil {
		return err
	}
	go d.heartbeatLoop(ctx, stream)

	var wg sync.WaitGroup
	sem := make(chan struct{}, d.Cfg.Judge.MaxParallel)
	for {
		msg, err := stream.Recv()
		if err != nil {
			wg.Wait()
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return err
		}
		dispatch := msg.GetDispatch()
		if dispatch == nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(task *pb.JudgeTask) {
			defer wg.Done()
			defer func() { <-sem }()
			d.runTask(ctx, stream, task)
		}(dispatch.GetTask())
	}
}

func (d *Daemon) languageIDs() []string {
	ids := make([]string, 0, 8)
	for _, l := range d.Service.Reg.List() {
		ids = append(ids, l.ID)
	}
	return ids
}

func (d *Daemon) heartbeatLoop(ctx context.Context, stream pb.JudgeRelay_ConnectClient) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			load := sysload.Read()
			if err := d.send(stream, &pb.DaemonMessage{Body: &pb.DaemonMessage_Heartbeat{
				Heartbeat: &pb.Heartbeat{
					ActiveTasks: d.active.Load(),
					Load1:       load.Load1,
					MemUsedMb:   load.MemUsedMB,
					MemTotalMb:  load.MemTotalMB,
				},
			}}); err != nil {
				// why log-and-continue: liveness is best-effort; a single
				// failed heartbeat must not blind the monitor forever while
				// the stream (results!) keeps working.
				log.Printf("[daemon] heartbeat send: %v", err)
			}
		}
	}
}

// runTask executes one judge task and reports the outcome; infra errors are
// surfaced as SE so the API side stops waiting on the lease.
func (d *Daemon) runTask(ctx context.Context, stream pb.JudgeRelay_ConnectClient, task *pb.JudgeTask) {
	d.active.Add(1)
	defer d.active.Add(-1)

	t := fromPbTask(task)
	result := d.Service.Run(ctx, t)
	if result.Status == "SE" || result.Error != "" {
		// why log here: SE means infrastructure trouble on this machine; the
		// daemon log is the first place an operator looks.
		log.Printf("[daemon] task %d SE: %s", task.SubmissionId, result.Error)
	}
	if err := d.send(stream, &pb.DaemonMessage{Body: &pb.DaemonMessage_Result{Result: &pb.TaskResult{
		SubmissionId:   task.SubmissionId,
		Status:         result.Status,
		TimeMs:         result.TimeMS,
		MemoryKb:       result.MemKB,
		Score:          int64(result.Score),
		CompileMessage: result.CompileMessage,
		Cases:          toPbCases(result.Cases),
		Error:          result.Error,
	}}}); err != nil {
		log.Printf("[daemon] send result %d: %v", task.SubmissionId, err)
	}
}

func fromPbTask(t *pb.JudgeTask) *judge.Task {
	cases := make([]judge.CaseRef, 0, len(t.GetCases()))
	for _, c := range t.GetCases() {
		cases = append(cases, judge.CaseRef{
			Index: int(c.GetIndex()), InputSHA: c.GetInputSha256(),
			AnswerSHA: c.GetAnswerSha256(), InputSize: c.GetInputSize(),
			AnswerSize: c.GetAnswerSize(), Score: int(c.GetScore()),
		})
	}
	return &judge.Task{
		SubmissionID: t.GetSubmissionId(), ProblemID: t.GetProblemId(),
		LanguageID: t.GetLanguageId(), Code: t.GetCode(),
		TimeLimitMS: t.GetTimeLimitMs(), MemLimitMB: t.GetMemLimitMb(),
		JudgeMode: t.GetJudgeMode(), CheckerSource: t.GetCheckerSource(),
		InteractorSource: t.GetInteractorSource(), Cases: cases,
		StopOnFail: t.GetStopOnFail(),
	}
}

func toPbCases(cases []judge.CaseResult) []*pb.CaseResult {
	out := make([]*pb.CaseResult, 0, len(cases))
	for _, c := range cases {
		out = append(out, &pb.CaseResult{
			Index: int32(c.Index), Status: c.Status, TimeMs: c.TimeMS,
			MemoryKb: c.MemKB, Message: c.Message, Score: int64(c.Score),
		})
	}
	return out
}
