// Package queue abstracts the judge task queue: Redis LIST in production
// (atomic blocking pops, survives restarts) and an in-memory channel for
// dev/tests so the API runs with zero infrastructure.
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrClosed = errors.New("queue closed")

type TaskQueue interface {
	Push(ctx context.Context, submissionID uint64) error
	// Pop blocks until a task id is available, ctx is done, or the queue
	// closes. ok=false means the queue is empty-and-closed or ctx expired.
	Pop(ctx context.Context) (id uint64, ok bool, err error)
	Len(ctx context.Context) (int, error)
	Close() error
}

func New(redisAddr, password string, redisDB int) (TaskQueue, error) {
	if redisAddr == "" {
		return newMemoryQueue(), nil
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, Password: password, DB: redisDB})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping %s: %w", redisAddr, err)
	}
	return &redisQueue{client: client, key: "oj:judge:tasks"}, nil
}

// memoryQueue is a plain channel; losing it on restart only loses PENDING
// markers, which the lease requeue scanner restores from the database.
type memoryQueue struct {
	ch     chan uint64
	closed bool
	mu     sync.Mutex
}

func newMemoryQueue() *memoryQueue {
	return &memoryQueue{ch: make(chan uint64, 8192)}
}

func (m *memoryQueue) Push(_ context.Context, id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	select {
	case m.ch <- id:
		return nil
	default:
		return errors.New("queue full")
	}
}

func (m *memoryQueue) Pop(ctx context.Context) (uint64, bool, error) {
	select {
	case id, ok := <-m.ch:
		return id, ok, nil
	case <-ctx.Done():
		return 0, false, ctx.Err()
	}
}

func (m *memoryQueue) Len(_ context.Context) (int, error) {
	return len(m.ch), nil
}

func (m *memoryQueue) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.ch)
	}
	return nil
}

type redisQueue struct {
	client *redis.Client
	key    string
}

func (r *redisQueue) Push(ctx context.Context, id uint64) error {
	return r.client.RPush(ctx, r.key, fmt.Sprint(id)).Err()
}

func (r *redisQueue) Pop(ctx context.Context) (uint64, bool, error) {
	for {
		res, err := r.client.BLPop(ctx, time.Second, r.key).Result()
		if errors.Is(err, redis.Nil) || (err != nil && ctx.Err() != nil) {
			return 0, false, ctx.Err()
		}
		if err != nil {
			return 0, false, err
		}
		if len(res) < 2 {
			continue
		}
		var id uint64
		if _, err := fmt.Sscanf(res[1], "%d", &id); err != nil {
			continue // skip malformed entries instead of wedging the queue
		}
		return id, true, nil
	}
}

func (r *redisQueue) Len(ctx context.Context) (int, error) {
	return int(r.client.LLen(ctx, r.key).Val()), nil
}

func (r *redisQueue) Close() error {
	return r.client.Close()
}
