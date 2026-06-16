package cache

import (
	"testing"
	"time"
)

func TestLockFreeCacheCloseAndEvict(t *testing.T) {
	c := NewLockFreeCache(16, 1) // 1-second TTL
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))

	// Wait past the TTL, then run the periodic eviction directly.
	time.Sleep(2100 * time.Millisecond)
	c.evictExpired()

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted after TTL")
	}
	if _, _, size := c.Stats(); size != 0 {
		t.Errorf("expected size 0 after eviction, got %d", size)
	}
	c.Close()
}
