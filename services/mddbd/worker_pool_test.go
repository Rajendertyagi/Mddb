package main

import (
	"context"
	"testing"
)

func TestNewWorkerPool(t *testing.T) {
	pool := NewWorkerPool(4, nil)
	if pool == nil {
		t.Fatal("NewWorkerPool returned nil")
		return
	}
	defer pool.Close()

	if pool.workers != 4 {
		t.Errorf("workers = %d, want 4", pool.workers)
	}
}

func TestWorkerPool_SubmitUnknownJobType(t *testing.T) {
	pool := NewWorkerPool(2, nil)
	defer pool.Close()

	job := &Job{
		Type:    "unknown",
		Context: context.Background(),
	}

	err := pool.Submit(job)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	result, err := pool.GetResult()
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.Error == nil {
		t.Error("expected error for unknown job type")
	}
	if result.Error != ErrUnknownJobType {
		t.Errorf("error = %v, want ErrUnknownJobType", result.Error)
	}
}

func TestWorkerPool_Close(t *testing.T) {
	pool := NewWorkerPool(2, nil)

	// Submit a job before close
	err := pool.Submit(&Job{Type: "unknown", Context: context.Background()})
	if err != nil {
		t.Fatalf("Submit before close: %v", err)
	}

	// Drain the result
	_, _ = pool.GetResult()

	// Close should not panic
	pool.Close()
}

func TestWorkerPool_GetResult_AfterClose(t *testing.T) {
	pool := NewWorkerPool(2, nil)

	// Submit a job before close so there's something in the results channel
	_ = pool.Submit(&Job{Type: "unknown", Context: context.Background()})

	// Give worker time to process and place result
	result, err := pool.GetResult()
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.Error == nil {
		t.Error("expected error for unknown job type")
	}

	pool.Close()

	// After close, the results channel is closed; reading returns nil, zero value
	// The pool's context is cancelled, so GetResult should handle that
	result2, _ := pool.GetResult()
	// After close, we may get a nil result since the channel is drained and closed
	_ = result2
}

func TestWorkerPool_MultipleUnknownJobs(t *testing.T) {
	pool := NewWorkerPool(4, nil)
	defer pool.Close()

	n := 10
	for i := 0; i < n; i++ {
		err := pool.Submit(&Job{
			Type:    "unknown",
			Context: context.Background(),
		})
		if err != nil {
			t.Fatalf("Submit[%d]: %v", i, err)
		}
	}

	for i := 0; i < n; i++ {
		result, err := pool.GetResult()
		if err != nil {
			t.Fatalf("GetResult[%d]: %v", i, err)
		}
		if result.Error == nil {
			t.Errorf("result[%d] error should not be nil", i)
		}
	}
}

// --- Error types ---

func TestJobError_Error(t *testing.T) {
	tests := []struct {
		err     *JobError
		wantMsg string
	}{
		{ErrUnknownJobType, "unknown job type"},
		{ErrJobQueueFull, "job queue full"},
		{ErrPoolClosed, "pool closed"},
		{ErrResultTimeout, "result timeout"},
	}

	for _, tt := range tests {
		if tt.err.Error() != tt.wantMsg {
			t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.wantMsg)
		}
	}
}

func TestJobError_ImplementsError(t *testing.T) {
	var err error = &JobError{Message: "test"}
	if err.Error() != "test" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test")
	}
}

// --- Job and JobResult types ---

func TestJob_Fields(t *testing.T) {
	ctx := context.Background()
	job := &Job{
		Type:    "add",
		Request: "some-request",
		Context: ctx,
	}

	if job.Type != "add" {
		t.Errorf("Type = %q, want %q", job.Type, "add")
	}
	if job.Request != "some-request" {
		t.Errorf("Request = %v, want %q", job.Request, "some-request")
	}
	if job.Context != ctx {
		t.Error("Context mismatch")
	}
}

func TestJobResult_Fields(t *testing.T) {
	result := &JobResult{
		Response: "response-data",
		Error:    nil,
	}

	if result.Response != "response-data" {
		t.Errorf("Response = %v, want %q", result.Response, "response-data")
	}
	if result.Error != nil {
		t.Errorf("Error = %v, want nil", result.Error)
	}
}

func TestWorkerPool_SubmitCancelled(t *testing.T) {
	pool := NewWorkerPool(2, nil)
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	job := &Job{
		Type:    "unknown",
		Context: ctx,
	}

	// Should still be able to submit (context is on the job, not the pool)
	err := pool.Submit(job)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	result, err := pool.GetResult()
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	// Should get an error result since job type is unknown
	if result.Error == nil {
		t.Error("expected error for unknown job type")
	}
}
