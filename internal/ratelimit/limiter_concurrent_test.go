package ratelimit

import (
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestLimiterConcurrent(t *testing.T) {
	const workers, perWorker, burst = 8, 50, 100
	l := New(Rate{Count: 1, Per: time.Hour}, burst)
	k := rateLimitTestKey("192.0.2.1/32")
	allowed := runConcurrentLimiterRequests(l, k, workers, perWorker)
	if allowed < burst || allowed > burst+2 {
		t.Fatalf("allowed = %d, want about %d", allowed, burst)
	}
}

// runConcurrentLimiterRequests runs concurrent Allow calls and returns allowed count.
func runConcurrentLimiterRequests(l *Limiter, k netip.Prefix, workers, perWorker int) int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if ok, _ := l.Allow(k); ok {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
				if i%10 == 0 {
					l.Sweep()
				}
			}
		}()
	}
	wg.Wait()
	return allowed
}

func BenchmarkAllow(b *testing.B) {
	l := New(Rate{Count: 1_000_000, Per: time.Second}, 1000)
	keys := make([]netip.Prefix, 1024)
	for i := range keys {
		keys[i] = netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}), 32)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow(keys[i%len(keys)])
	}
}
