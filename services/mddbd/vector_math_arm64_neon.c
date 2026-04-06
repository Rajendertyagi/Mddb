//go:build arm64 && !nosme

#include <arm_neon.h>
#include <math.h>

// cosine_sim_neon computes cosine similarity between two float32 vectors using NEON.
// Uses float32x4_t (128-bit SIMD) for 4-way parallel FMA.
float cosine_sim_neon(const float *a, const float *b, int dims) {
    float32x4_t sumDot = vdupq_n_f32(0.0f);
    float32x4_t sumA = vdupq_n_f32(0.0f);
    float32x4_t sumB = vdupq_n_f32(0.0f);

    int i = 0;
    for (; i + 4 <= dims; i += 4) {
        float32x4_t va = vld1q_f32(a + i);
        float32x4_t vb = vld1q_f32(b + i);
        sumDot = vfmaq_f32(sumDot, va, vb);
        sumA = vfmaq_f32(sumA, va, va);
        sumB = vfmaq_f32(sumB, vb, vb);
    }

    float dot = vaddvq_f32(sumDot);
    float normA = vaddvq_f32(sumA);
    float normB = vaddvq_f32(sumB);

    // Handle remaining elements (dims not divisible by 4)
    for (; i < dims; i++) {
        dot += a[i] * b[i];
        normA += a[i] * a[i];
        normB += b[i] * b[i];
    }

    if (normA == 0.0f || normB == 0.0f) {
        return 0.0f;
    }
    return dot / sqrtf(normA * normB);
}

// dot_product_neon computes dot product between two float32 vectors using NEON.
float dot_product_neon(const float *a, const float *b, int dims) {
    float32x4_t sum = vdupq_n_f32(0.0f);

    int i = 0;
    for (; i + 4 <= dims; i += 4) {
        float32x4_t va = vld1q_f32(a + i);
        float32x4_t vb = vld1q_f32(b + i);
        sum = vfmaq_f32(sum, va, vb);
    }

    float result = vaddvq_f32(sum);
    for (; i < dims; i++) {
        result += a[i] * b[i];
    }
    return result;
}

// euclidean_dist_sq_neon computes squared Euclidean distance using NEON.
float euclidean_dist_sq_neon(const float *a, const float *b, int dims) {
    float32x4_t sum = vdupq_n_f32(0.0f);

    int i = 0;
    for (; i + 4 <= dims; i += 4) {
        float32x4_t va = vld1q_f32(a + i);
        float32x4_t vb = vld1q_f32(b + i);
        float32x4_t diff = vsubq_f32(va, vb);
        sum = vfmaq_f32(sum, diff, diff);
    }

    float result = vaddvq_f32(sum);
    for (; i < dims; i++) {
        float d = a[i] - b[i];
        result += d * d;
    }
    return result;
}

// batch_cosine_sim_neon computes cosine similarity of one query against count
// vectors packed contiguously in matrix (row-major, dims floats per row).
// Results are written to out[0..count-1].
void batch_cosine_sim_neon(const float *query, const float *matrix,
                           int dims, int count, float *out) {
    for (int j = 0; j < count; j++) {
        out[j] = cosine_sim_neon(query, matrix + j * dims, dims);
    }
}
