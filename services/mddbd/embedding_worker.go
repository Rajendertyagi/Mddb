package main

import (
	"context"
	"fmt"
	"log"
	"mddb/internal/embedding"
	"mddb/internal/envconf"
	"mddb/internal/metrics"
	vec "mddb/internal/vector"
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
	provider     embedding.Provider
	vectorStore  *vec.VectorStore
	vectorIndex  *vec.VectorIndex
	jobs         chan EmbeddingJob
	wg           sync.WaitGroup
	stopCh       chan struct{}
	chunkSize    int
	chunkEnabled bool
	metrics      *metrics.Metrics
}

// NewEmbeddingWorker creates a new background embedding worker.
func NewEmbeddingWorker(provider embedding.Provider, store *vec.VectorStore, index *vec.VectorIndex, bufferSize int) *EmbeddingWorker {
	return &EmbeddingWorker{
		provider:     provider,
		vectorStore:  store,
		vectorIndex:  index,
		jobs:         make(chan EmbeddingJob, bufferSize),
		stopCh:       make(chan struct{}),
		chunkSize:    envconf.Int("MDDB_EMBEDDING_CHUNK_SIZE", 1500),
		chunkEnabled: envconf.String("MDDB_EMBEDDING_CHUNK_ENABLED", "true") == "true",
	}
}

// Start begins processing embedding jobs in the background.
func (w *EmbeddingWorker) Start(workers int) {
	for i := 0; i < workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}
	log.Printf("Embedding worker started (%d workers, buffer=%d, chunking=%v, chunkSize=%d)",
		workers, cap(w.jobs), w.chunkEnabled, w.chunkSize)
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
	contentHash := vec.ContentHash(job.ContentMD)

	// Check if content already has a matching embedding
	existing, err := w.vectorStore.Get(job.Collection, job.DocID)
	if err == nil && existing != nil && existing.ContentHash == contentHash {
		return // embedding is up-to-date
	}

	// Split into chunks if enabled
	var chunks []string
	if w.chunkEnabled {
		chunks = ChunkText(job.ContentMD, w.chunkSize)
	} else {
		chunks = []string{job.ContentMD}
	}

	if len(chunks) == 0 {
		return
	}

	// Generate embedding for each chunk
	var chunkEmbeddings []vec.ChunkEmbedding
	for i, chunk := range chunks {
		var vector []float32
		var embedErr error
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			vector, embedErr = w.provider.Embed(ctx, chunk)
			cancel()
			if embedErr == nil {
				break
			}
			log.Printf("Embedding attempt %d failed for %s/%s chunk %d: %v", attempt+1, job.Collection, job.DocID, i, embedErr)
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}

		if embedErr != nil {
			log.Printf("ERROR: failed to embed %s/%s chunk %d after 3 attempts: %v", job.Collection, job.DocID, i, embedErr)
			if w.metrics != nil {
				w.metrics.IncOp("embedding", "error")
			}
			return
		}

		chunkEmbeddings = append(chunkEmbeddings, vec.ChunkEmbedding{
			ChunkIndex: i,
			Vector:     vector,
		})
	}

	// Store all chunks in BoltDB
	if err := w.vectorStore.PutChunks(job.Collection, job.DocID, chunkEmbeddings, w.provider.Model(), contentHash); err != nil {
		log.Printf("ERROR: failed to store embedding for %s/%s: %v", job.Collection, job.DocID, err)
		return
	}

	// Update in-memory index
	for _, ce := range chunkEmbeddings {
		chunkKey := fmt.Sprintf("%s#%d", job.DocID, ce.ChunkIndex)
		w.vectorIndex.Add(job.Collection, chunkKey, ce.Vector)
	}

	// Clean stale chunks from index (if document shrank)
	w.vectorStore.CleanStaleChunks(job.Collection, job.DocID, len(chunkEmbeddings), w.vectorIndex)

	if w.metrics != nil {
		w.metrics.IncOp("embedding", "completed")
	}
}
