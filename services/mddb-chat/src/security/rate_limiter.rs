use governor::{Quota, RateLimiter as GovRateLimiter};
use std::num::NonZeroU32;
use std::sync::Arc;

use dashmap::DashMap;

/// Per-IP rate limiter using token bucket algorithm
pub struct RateLimiter {
    limiters: DashMap<String, Arc<GovRateLimiter<governor::state::NotKeyed, governor::state::InMemoryState, governor::clock::DefaultClock>>>,
    quota: Quota,
}

impl RateLimiter {
    pub fn new(requests_per_minute: u32) -> Self {
        let rpm = NonZeroU32::new(requests_per_minute.max(1)).unwrap();
        let quota = Quota::per_minute(rpm);

        Self {
            limiters: DashMap::new(),
            quota,
        }
    }

    /// Check if request from this IP is allowed
    pub fn check(&self, ip: &str) -> bool {
        let limiter = self
            .limiters
            .entry(ip.to_string())
            .or_insert_with(|| Arc::new(GovRateLimiter::direct(self.quota)));

        limiter.check().is_ok()
    }

    /// Clean up stale entries (call periodically)
    pub fn cleanup(&self) {
        // Remove entries that haven't been used
        // In practice, governor handles this internally
        // but we can limit map growth
        if self.limiters.len() > 10_000 {
            self.limiters.clear();
        }
    }
}
