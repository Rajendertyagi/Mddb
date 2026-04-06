//go:build arm64 && !nosme && cgo

package main

/*
#cgo CFLAGS: -O3
#include <stdlib.h>

// NEON functions (always available on arm64)
extern float cosine_sim_neon(const float *a, const float *b, int dims);
extern float dot_product_neon(const float *a, const float *b, int dims);
extern float euclidean_dist_sq_neon(const float *a, const float *b, int dims);
extern void batch_cosine_sim_neon(const float *query, const float *matrix,
                                  int dims, int count, float *out);

// SME functions (compiled only when compiler supports SME)
extern float cosine_sim_sme(const float *a, const float *b, int dims);
extern float dot_product_sme(const float *a, const float *b, int dims);
extern float euclidean_dist_sq_sme(const float *a, const float *b, int dims);
extern void batch_cosine_sim_sme(const float *query, const float *matrix,
                                 int dims, int count, float *out);
extern int sme_available(void);

// Runtime SME hardware detection (macOS)
#if defined(__APPLE__)
#include <sys/sysctl.h>

static int detect_sme(void) {
    int val = 0;
    size_t size = sizeof(val);
    if (sysctlbyname("hw.optional.arm.FEAT_SME", &val, &size, NULL, 0) == 0) {
        return val;
    }
    return 0;
}

#elif defined(__linux__)
#include <sys/auxv.h>
#ifndef HWCAP2_SME
#define HWCAP2_SME (1UL << 23)
#endif

static int detect_sme(void) {
    unsigned long hwcap2 = getauxval(AT_HWCAP2);
    return (hwcap2 & HWCAP2_SME) ? 1 : 0;
}

#else

static int detect_sme(void) {
    return 0;
}

#endif
*/
import "C"

import (
	"math"
	"unsafe"
)

// tier holds the active SIMD acceleration tier, set once during init.
var tier string

func init() {
	if C.sme_available() == 1 && C.detect_sme() == 1 {
		tier = "sme"
	} else {
		tier = "neon"
	}
}

// vectorMathTier returns the active SIMD acceleration tier.
func vectorMathTier() string { return tier }

// cosineSimilarity computes cosine similarity using NEON or SME acceleration.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dims := C.int(len(a))
	pa := (*C.float)(unsafe.Pointer(&a[0]))
	pb := (*C.float)(unsafe.Pointer(&b[0]))
	if tier == "sme" {
		return float32(C.cosine_sim_sme(pa, pb, dims))
	}
	return float32(C.cosine_sim_neon(pa, pb, dims))
}

// dotProductSimilarity computes dot product using NEON or SME acceleration.
func dotProductSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dims := C.int(len(a))
	pa := (*C.float)(unsafe.Pointer(&a[0]))
	pb := (*C.float)(unsafe.Pointer(&b[0]))
	if tier == "sme" {
		return float32(C.dot_product_sme(pa, pb, dims))
	}
	return float32(C.dot_product_neon(pa, pb, dims))
}

// euclideanSimilarity converts Euclidean distance to a similarity score.
// Returns 1/(1+dist), so closer vectors -> higher score (range 0 to 1).
func euclideanSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dims := C.int(len(a))
	pa := (*C.float)(unsafe.Pointer(&a[0]))
	pb := (*C.float)(unsafe.Pointer(&b[0]))
	var distSq float32
	if tier == "sme" {
		distSq = float32(C.euclidean_dist_sq_sme(pa, pb, dims))
	} else {
		distSq = float32(C.euclidean_dist_sq_neon(pa, pb, dims))
	}
	return float32(1.0 / (1.0 + math.Sqrt(float64(distSq))))
}

// euclideanDistSq computes squared Euclidean distance using NEON or SME.
func euclideanDistSq(a, b []float32) float64 {
	n := len(a)
	if n > len(b) {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	dims := C.int(n)
	pa := (*C.float)(unsafe.Pointer(&a[0]))
	pb := (*C.float)(unsafe.Pointer(&b[0]))
	if tier == "sme" {
		return float64(C.euclidean_dist_sq_sme(pa, pb, dims))
	}
	return float64(C.euclidean_dist_sq_neon(pa, pb, dims))
}

// batchCosineSim computes cosine similarity of query against count vectors
// packed contiguously in matrix (row-major, dims per row). Results written to out.
func batchCosineSim(query []float32, matrix []float32, dims, count int, out []float32) {
	if count == 0 || dims == 0 {
		return
	}
	pq := (*C.float)(unsafe.Pointer(&query[0]))
	pm := (*C.float)(unsafe.Pointer(&matrix[0]))
	po := (*C.float)(unsafe.Pointer(&out[0]))
	cd := C.int(dims)
	cc := C.int(count)
	if tier == "sme" {
		C.batch_cosine_sim_sme(pq, pm, cd, cc, po)
	} else {
		C.batch_cosine_sim_neon(pq, pm, cd, cc, po)
	}
}
