//go:build arm64 && !nosme

#include <math.h>

// SME (Scalable Matrix Extension) accelerated vector math.
// Requires ARM v9.2-a with SME support (Apple M4+, Cortex-X925+).
// Functions use __arm_streaming to execute in streaming SVE mode,
// providing wider SIMD registers than NEON's fixed 128-bit.

#if __ARM_FEATURE_SME
#include <arm_sme.h>

// cosine_sim_sme computes cosine similarity using SVE in streaming mode.
__arm_locally_streaming
float cosine_sim_sme(const float *a, const float *b, int dims) {
    svfloat32_t sumDot = svdup_f32(0.0f);
    svfloat32_t sumA = svdup_f32(0.0f);
    svfloat32_t sumB = svdup_f32(0.0f);

    int i = 0;
    int vl = (int)svcntw();
    for (; i + vl <= dims; i += vl) {
        svbool_t pg = svptrue_b32();
        svfloat32_t va = svld1_f32(pg, a + i);
        svfloat32_t vb = svld1_f32(pg, b + i);
        sumDot = svmla_f32_m(pg, sumDot, va, vb);
        sumA = svmla_f32_m(pg, sumA, va, va);
        sumB = svmla_f32_m(pg, sumB, vb, vb);
    }

    // Handle tail with predicated load
    if (i < dims) {
        svbool_t pg = svwhilelt_b32(i, dims);
        svfloat32_t va = svld1_f32(pg, a + i);
        svfloat32_t vb = svld1_f32(pg, b + i);
        sumDot = svmla_f32_m(pg, sumDot, va, vb);
        sumA = svmla_f32_m(pg, sumA, va, va);
        sumB = svmla_f32_m(pg, sumB, vb, vb);
    }

    float dot = svaddv_f32(svptrue_b32(), sumDot);
    float normA = svaddv_f32(svptrue_b32(), sumA);
    float normB = svaddv_f32(svptrue_b32(), sumB);

    if (normA == 0.0f || normB == 0.0f) {
        return 0.0f;
    }
    return dot / sqrtf(normA * normB);
}

// dot_product_sme computes dot product using SVE in streaming mode.
__arm_locally_streaming
float dot_product_sme(const float *a, const float *b, int dims) {
    svfloat32_t sum = svdup_f32(0.0f);

    int i = 0;
    int vl = (int)svcntw();
    for (; i + vl <= dims; i += vl) {
        svbool_t pg = svptrue_b32();
        svfloat32_t va = svld1_f32(pg, a + i);
        svfloat32_t vb = svld1_f32(pg, b + i);
        sum = svmla_f32_m(pg, sum, va, vb);
    }

    if (i < dims) {
        svbool_t pg = svwhilelt_b32(i, dims);
        svfloat32_t va = svld1_f32(pg, a + i);
        svfloat32_t vb = svld1_f32(pg, b + i);
        sum = svmla_f32_m(pg, sum, va, vb);
    }

    return svaddv_f32(svptrue_b32(), sum);
}

// euclidean_dist_sq_sme computes squared Euclidean distance using SVE.
__arm_locally_streaming
float euclidean_dist_sq_sme(const float *a, const float *b, int dims) {
    svfloat32_t sum = svdup_f32(0.0f);

    int i = 0;
    int vl = (int)svcntw();
    for (; i + vl <= dims; i += vl) {
        svbool_t pg = svptrue_b32();
        svfloat32_t va = svld1_f32(pg, a + i);
        svfloat32_t vb = svld1_f32(pg, b + i);
        svfloat32_t diff = svsub_f32_m(pg, va, vb);
        sum = svmla_f32_m(pg, sum, diff, diff);
    }

    if (i < dims) {
        svbool_t pg = svwhilelt_b32(i, dims);
        svfloat32_t va = svld1_f32(pg, a + i);
        svfloat32_t vb = svld1_f32(pg, b + i);
        svfloat32_t diff = svsub_f32_m(pg, va, vb);
        sum = svmla_f32_m(pg, sum, diff, diff);
    }

    return svaddv_f32(svptrue_b32(), sum);
}

// batch_cosine_sim_sme computes cosine similarity of one query against count
// vectors packed in matrix. Uses SVE streaming mode for wider SIMD.
void batch_cosine_sim_sme(const float *query, const float *matrix,
                          int dims, int count, float *out) {
    for (int j = 0; j < count; j++) {
        out[j] = cosine_sim_sme(query, matrix + j * dims, dims);
    }
}

// sme_available returns 1 if SME functions are compiled and available.
int sme_available(void) {
    return 1;
}

#else

// Stub implementations when compiler does not support SME intrinsics.
float cosine_sim_sme(const float *a, const float *b, int dims) {
    (void)a; (void)b; (void)dims; return 0.0f;
}
float dot_product_sme(const float *a, const float *b, int dims) {
    (void)a; (void)b; (void)dims; return 0.0f;
}
float euclidean_dist_sq_sme(const float *a, const float *b, int dims) {
    (void)a; (void)b; (void)dims; return 0.0f;
}
void batch_cosine_sim_sme(const float *query, const float *matrix,
                          int dims, int count, float *out) {
    (void)query; (void)matrix; (void)dims; (void)count; (void)out;
}

int sme_available(void) {
    return 0;
}

#endif
