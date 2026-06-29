package vector

import (
	"fmt"
	"math/rand"
	"testing"
)

// --- Single-pair benchmarks ---

func BenchmarkCosineSim768(b *testing.B) {
	benchCosineSim(b, 768)
}

func BenchmarkCosineSim1024(b *testing.B) {
	benchCosineSim(b, 1024)
}

func BenchmarkCosineSim1536(b *testing.B) {
	benchCosineSim(b, 1536)
}

func BenchmarkCosineSim3072(b *testing.B) {
	benchCosineSim(b, 3072)
}

func benchCosineSim(b *testing.B, dims int) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	a := randVec(dims, rng)
	v := randVec(dims, rng)
	b.SetBytes(int64(dims * 4 * 2)) // bytes read: two float32 vectors
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CosineSimilarity(a, v)
	}
}

func BenchmarkDotProduct768(b *testing.B) {
	benchDotProduct(b, 768)
}

func BenchmarkDotProduct1536(b *testing.B) {
	benchDotProduct(b, 1536)
}

func benchDotProduct(b *testing.B, dims int) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	a := randVec(dims, rng)
	v := randVec(dims, rng)
	b.SetBytes(int64(dims * 4 * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dotProductSimilarity(a, v)
	}
}

func BenchmarkEuclideanSim768(b *testing.B) {
	benchEuclideanSim(b, 768)
}

func BenchmarkEuclideanSim1536(b *testing.B) {
	benchEuclideanSim(b, 1536)
}

func benchEuclideanSim(b *testing.B, dims int) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	a := randVec(dims, rng)
	v := randVec(dims, rng)
	b.SetBytes(int64(dims * 4 * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = euclideanSimilarity(a, v)
	}
}

// --- Batch benchmarks ---

func BenchmarkBatchCosineSim_1K_768(b *testing.B) {
	benchBatchCosine(b, 768, 1000)
}

func BenchmarkBatchCosineSim_1K_1536(b *testing.B) {
	benchBatchCosine(b, 1536, 1000)
}

func BenchmarkBatchCosineSim_10K_768(b *testing.B) {
	benchBatchCosine(b, 768, 10000)
}

func BenchmarkBatchCosineSim_10K_1536(b *testing.B) {
	benchBatchCosine(b, 1536, 10000)
}

func BenchmarkBatchCosineSim_50K_768(b *testing.B) {
	benchBatchCosine(b, 768, 50000)
}

func benchBatchCosine(b *testing.B, dims, count int) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	query := randVec(dims, rng)
	matrix := make([]float32, dims*count)
	for i := range matrix {
		matrix[i] = rng.Float32()*2 - 1
	}
	out := make([]float32, count)

	b.SetBytes(int64(dims*4*(count+1) + count*4)) // input + output bytes
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batchCosineSim(query, matrix, dims, count, out)
	}
}

// --- EuclideanDistSq benchmark ---

func BenchmarkEuclideanDistSq768(b *testing.B) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	a := randVec(768, rng)
	v := randVec(768, rng)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = euclideanDistSq(a, v)
	}
}

func BenchmarkEuclideanDistSq1536(b *testing.B) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	a := randVec(1536, rng)
	v := randVec(1536, rng)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = euclideanDistSq(a, v)
	}
}

func randVec(dims int, rng *rand.Rand) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = rng.Float32()*2 - 1
	}
	return v
}

// BenchmarkVectorMathTier logs the active tier for reference.
func BenchmarkVectorMathTier(b *testing.B) {
	b.Log("tier:", vectorMathTier())
	b.ReportMetric(0, "ns/op")
	fmt.Printf("# vector_math tier: %s\n", vectorMathTier())
}
