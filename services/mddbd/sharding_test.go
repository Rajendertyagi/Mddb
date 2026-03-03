package main

import (
	"fmt"
	"sync"
	"testing"
)

// --- ConsistentHash ---

func TestNewConsistentHash(t *testing.T) {
	ch := NewConsistentHash(100)
	if ch == nil {
		t.Fatal("NewConsistentHash returned nil")
	}
	if ch.replicas != 100 {
		t.Errorf("replicas = %d, want 100", ch.replicas)
	}
	if len(ch.ring) != 0 {
		t.Errorf("ring should be empty, got %d entries", len(ch.ring))
	}
}

func TestConsistentHash_AddAndGet(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1)
	ch.Add(1, 1)
	ch.Add(2, 1)

	// With 3 shards, Get should always return a valid shard ID
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		shardID := ch.Get(key)
		if shardID < 0 || shardID > 2 {
			t.Errorf("Get(%q) = %d, want 0..2", key, shardID)
		}
	}
}

func TestConsistentHash_GetEmpty(t *testing.T) {
	ch := NewConsistentHash(50)
	got := ch.Get("anything")
	if got != 0 {
		t.Errorf("Get on empty ring = %d, want 0", got)
	}
}

func TestConsistentHash_GetDeterministic(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1)
	ch.Add(1, 1)

	key := "deterministic-key"
	first := ch.Get(key)
	for i := 0; i < 50; i++ {
		if ch.Get(key) != first {
			t.Fatalf("Get(%q) not deterministic", key)
		}
	}
}

func TestConsistentHash_Remove(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1)
	ch.Add(1, 1)

	// Before removal, some keys should map to shard 1
	foundShard1 := false
	for i := 0; i < 200; i++ {
		if ch.Get(fmt.Sprintf("k%d", i)) == 1 {
			foundShard1 = true
			break
		}
	}
	if !foundShard1 {
		t.Fatal("expected some keys on shard 1 before removal")
	}

	ch.Remove(1)

	// After removal, all keys should map to shard 0
	for i := 0; i < 200; i++ {
		got := ch.Get(fmt.Sprintf("k%d", i))
		if got != 0 {
			t.Errorf("after removing shard 1, Get = %d, want 0", got)
		}
	}
}

func TestConsistentHash_GetN(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1)
	ch.Add(1, 1)
	ch.Add(2, 1)

	shards := ch.GetN("some-key", 2)
	if len(shards) != 2 {
		t.Fatalf("GetN returned %d shards, want 2", len(shards))
	}

	// All returned IDs should be distinct
	seen := make(map[int]bool)
	for _, id := range shards {
		if seen[id] {
			t.Errorf("GetN returned duplicate shard %d", id)
		}
		seen[id] = true
	}
}

func TestConsistentHash_GetN_ExactlyAvailable(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1)
	ch.Add(1, 1)
	ch.Add(2, 1)

	// Request exactly the number of unique shards available
	shards := ch.GetN("key", 3)
	if len(shards) != 3 {
		t.Errorf("GetN returned %d shards, want 3", len(shards))
	}

	seen := make(map[int]bool)
	for _, id := range shards {
		if seen[id] {
			t.Errorf("duplicate shard %d", id)
		}
		seen[id] = true
	}
}

func TestConsistentHash_GetN_ZeroOrNegative(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1)

	if got := ch.GetN("k", 0); got != nil {
		t.Errorf("GetN(0) = %v, want nil", got)
	}
	if got := ch.GetN("k", -1); got != nil {
		t.Errorf("GetN(-1) = %v, want nil", got)
	}
}

func TestConsistentHash_GetN_EmptyRing(t *testing.T) {
	ch := NewConsistentHash(50)
	if got := ch.GetN("k", 3); got != nil {
		t.Errorf("GetN on empty ring = %v, want nil", got)
	}
}

func TestConsistentHash_Weight(t *testing.T) {
	ch := NewConsistentHash(50)
	ch.Add(0, 1) // weight 1
	ch.Add(1, 3) // weight 3 -> 3x virtual nodes

	counts := map[int]int{0: 0, 1: 0}
	for i := 0; i < 1000; i++ {
		id := ch.Get(fmt.Sprintf("test-key-%d", i))
		counts[id]++
	}

	// Shard 1 (weight 3) should get significantly more keys than shard 0 (weight 1)
	if counts[1] <= counts[0] {
		t.Errorf("weighted shard 1 got %d keys, shard 0 got %d keys; expected shard 1 to dominate", counts[1], counts[0])
	}
}

// --- ShardCluster ---

func TestNewShardCluster(t *testing.T) {
	sc := NewShardCluster(3, 2)
	if sc == nil {
		t.Fatal("NewShardCluster returned nil")
	}

	stats := sc.Stats()
	if stats.TotalShards != 3 {
		t.Errorf("TotalShards = %d, want 3", stats.TotalShards)
	}
	if stats.ActiveShards != 3 {
		t.Errorf("ActiveShards = %d, want 3", stats.ActiveShards)
	}
}

func TestShardCluster_GetShard(t *testing.T) {
	sc := NewShardCluster(4, 1)

	shard := sc.GetShard("my-document")
	if shard == nil {
		t.Fatal("GetShard returned nil")
	}
	if shard.ID < 0 || shard.ID >= 4 {
		t.Errorf("shard.ID = %d, want 0..3", shard.ID)
	}
	if !shard.Active {
		t.Error("returned shard is inactive")
	}
}

func TestShardCluster_GetShards_Replication(t *testing.T) {
	sc := NewShardCluster(5, 3)

	shards := sc.GetShards("replicated-key")
	if len(shards) != 3 {
		t.Fatalf("GetShards returned %d shards, want 3", len(shards))
	}

	// All shards should be distinct
	ids := make(map[int]bool)
	for _, s := range shards {
		if ids[s.ID] {
			t.Errorf("duplicate shard ID %d in replication set", s.ID)
		}
		ids[s.ID] = true
	}
}

func TestShardCluster_AddShard(t *testing.T) {
	sc := NewShardCluster(2, 1)

	newShard := &Shard{
		Name:   "shard-new",
		Weight: 1,
		Active: true,
	}
	sc.AddShard(newShard)

	stats := sc.Stats()
	if stats.TotalShards != 3 {
		t.Errorf("TotalShards after add = %d, want 3", stats.TotalShards)
	}
	if newShard.ID != 2 {
		t.Errorf("new shard ID = %d, want 2", newShard.ID)
	}
}

func TestShardCluster_RemoveShard(t *testing.T) {
	sc := NewShardCluster(3, 1)

	err := sc.RemoveShard(1)
	if err != nil {
		t.Fatalf("RemoveShard: %v", err)
	}

	stats := sc.Stats()
	if stats.ActiveShards != 2 {
		t.Errorf("ActiveShards after removal = %d, want 2", stats.ActiveShards)
	}
	if stats.Shards[1].Active {
		t.Error("removed shard still active")
	}
}

func TestShardCluster_RemoveShard_InvalidID(t *testing.T) {
	sc := NewShardCluster(2, 1)

	if err := sc.RemoveShard(-1); err == nil {
		t.Error("RemoveShard(-1) should return error")
	}
	if err := sc.RemoveShard(99); err == nil {
		t.Error("RemoveShard(99) should return error")
	}
}

func TestShardCluster_Rebalance(t *testing.T) {
	sc := NewShardCluster(3, 1)

	// Set doc counts
	sc.shards[0].DocCount.Store(100)
	sc.shards[1].DocCount.Store(200)
	sc.shards[2].DocCount.Store(300)

	err := sc.Rebalance()
	if err != nil {
		t.Fatalf("Rebalance: %v", err)
	}

	stats := sc.Stats()
	if stats.TotalDocs != 600 {
		t.Errorf("TotalDocs = %d, want 600", stats.TotalDocs)
	}
}

func TestShardCluster_Rebalance_NoActiveShards(t *testing.T) {
	sc := NewShardCluster(2, 1)
	_ = sc.RemoveShard(0)
	_ = sc.RemoveShard(1)

	err := sc.Rebalance()
	if err == nil {
		t.Error("Rebalance with no active shards should return error")
	}
}

func TestShardCluster_Stats(t *testing.T) {
	sc := NewShardCluster(3, 1)
	sc.shards[0].DocCount.Store(10)
	sc.shards[1].DocCount.Store(20)
	sc.shards[2].DocCount.Store(30)

	stats := sc.Stats()

	if stats.TotalShards != 3 {
		t.Errorf("TotalShards = %d, want 3", stats.TotalShards)
	}
	if stats.ActiveShards != 3 {
		t.Errorf("ActiveShards = %d, want 3", stats.ActiveShards)
	}
	if stats.TotalDocs != 60 {
		t.Errorf("TotalDocs = %d, want 60", stats.TotalDocs)
	}
	if len(stats.Shards) != 3 {
		t.Fatalf("Shards len = %d, want 3", len(stats.Shards))
	}

	for i, ss := range stats.Shards {
		expected := fmt.Sprintf("shard-%d", i)
		if ss.Name != expected {
			t.Errorf("shard[%d].Name = %q, want %q", i, ss.Name, expected)
		}
	}
}

func TestShardCluster_ConcurrentGetShard(t *testing.T) {
	sc := NewShardCluster(8, 2)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent-key-%d", n)
			shard := sc.GetShard(key)
			if shard == nil {
				t.Errorf("GetShard returned nil for key %q", key)
			}
		}(i)
	}
	wg.Wait()
}

func TestConsistentHash_Distribution(t *testing.T) {
	ch := NewConsistentHash(150)
	numShards := 4
	for i := 0; i < numShards; i++ {
		ch.Add(i, 1)
	}

	counts := make(map[int]int)
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		id := ch.Get(fmt.Sprintf("doc-%d", i))
		counts[id]++
	}

	// Each shard should get at least 10% of keys (ideal is 25%)
	minExpected := numKeys / 10
	for id, count := range counts {
		if count < minExpected {
			t.Errorf("shard %d got only %d/%d keys (less than %d minimum)", id, count, numKeys, minExpected)
		}
	}
}
