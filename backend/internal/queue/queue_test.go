package queue

import (
	"context"
	"testing"
	"time"
)

func TestMemoryQueueRoundTrip(t *testing.T) {
	q := newMemoryQueue()
	defer q.Close()
	if err := q.Push(context.Background(), 42); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := q.Push(context.Background(), 43); err != nil {
		t.Fatalf("push: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, ok, err := q.Pop(ctx)
	if err != nil || !ok || id != 42 {
		t.Fatalf("pop: id=%d ok=%v err=%v (FIFO order expected)", id, ok, err)
	}
	if n, _ := q.Len(ctx); n != 1 {
		t.Fatalf("len: got %d, want 1", n)
	}
}

func TestMemoryQueuePopBlocksUntilClose(t *testing.T) {
	q := newMemoryQueue()
	go func() {
		time.Sleep(50 * time.Millisecond)
		q.Close()
	}()
	done := make(chan struct{})
	go func() {
		_, ok, _ := q.Pop(context.Background())
		if ok {
			t.Error("pop after close must not yield items")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pop must unblock when the queue closes")
	}
}

func TestMemoryQueuePopCtxCancel(t *testing.T) {
	q := newMemoryQueue()
	defer q.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, ok, _ := q.Pop(ctx); ok {
		t.Fatal("pop must not yield items after ctx cancel")
	}
}
