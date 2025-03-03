package pool

import (
	"context"
	"testing"
	"time"
)

// 性能基准测试
func BenchmarkConnectionPool(b *testing.B) {
	factory := &mockFactory{}
	pool := NewPool(factory, WithMaxIdle(50), WithMaxActive(100))

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Get(ctx)
			if err != nil {
				b.Fatal(err)
			}
			// 模拟使用连接
			time.Sleep(time.Microsecond)
			pool.Put(conn, nil)
		}
	})

	// 关闭池
	pool.Shutdown(ctx)
}
