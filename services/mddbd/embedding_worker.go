package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// EmbeddingJob represents a document that needs embedding.
type EmbeddingJob struct {
	Collection string
	DocID      string
	ContentMD  string
}

// EmbeddingWorker processes embedding jobs asynchronously.
type EmbeddingWorker struct {
	provider    EmbeddingProvider
	vectorStore *VectorStore
	vectorIndex *VectorIndex
	jobs        chan EmbeddingJob
	wg          sync.WaitGroup
	stopCh      chan struct{}
}

// NewEmbeddingWorker creates a new background embedding worker.
func NewEmbeddingWorker(provider EmbeddingProvider, store *VectorStore, index *VectorIndex, bufferSize int) *EmbeddingWorker {
	return &EmbeddingWorker{
		provider:    provider,
		vectorStore: store,
		vectorIndex: index,
		jobs:        make(chan EmbeddingJob, bufferSize),
		stopCh:      make(chan struct{}),
	}
}

// Start begins processing embedding jobs in the background.
func (w *EmbeddingWorker) Start(workers int) {
	for i := 0; i < workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}
	log.Printf("Embedding worker started (%d workers, buffer=%d)", workers, cap(w.jobs))
}

// Stop gracefully stops the worker.
func (w *EmbeddingWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

// Enqueue adds an embedding job to the queue.
// Returns false if the queue is full (job dropped).
func (w *EmbeddingWorker) Enqueue(job EmbeddingJob) bool {
	select {
	case w.jobs <- job:
		return true
	default:
		log.Printf("WARNING: embedding queue full, dropping job for %s/%s", job.Collection, job.DocID)
		return false
	}
}

func (w *EmbeddingWorker) worker(id int) {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			// Drain remaining jobs
			for {
				select {
				case job := <-w.jobs:
					w.processJob(job)
				default:
					return
				}
			}
		case job := <-w.jobs:
			w.processJob(job)
		}
	}
}

func (w *EmbeddingWorker) processJob(job EmbeddingJob) {
	// Check if content already has a matching embedding
	existing, err := w.vectorStore.Get(job.Collection, job.DocID)
	if err == nil && existing != nil {
		currentHash := ContentHash(job.ContentMD)
		if existing.ContentHash == currentHash {
			return // embedding is up-to-date
		}
	}

	// Generate embedding with retry
	var vector []float32
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		vector, err = w.provider.Embed(ctx, job.ContentMD)
		cancel()
		if err == nil {
			break
		}
		log.Printf("Embedding attempt %d failed for %s/%s: %v", attempt+1, job.Collection, job.DocID, err)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	if err != nil {
		log.Printf("ERROR: failed to embed %s/%s after 3 attempts: %v", job.Collection, job.DocID, err)
		return
	}

	// Store in BoltDB
	contentHash := ContentHash(job.ContentMD)
	if err := w.vectorStore.Put(job.Collection, job.DocID, vector, w.provider.Model(), contentHash); err != nil {
		log.Printf("ERROR: failed to store embedding for %s/%s: %v", job.Collection, job.DocID, err)
		return
	}

	// Update in-memory index
	w.vectorIndex.Add(job.Collection, job.DocID, vector)
}
