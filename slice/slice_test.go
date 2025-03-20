package slice

import (
	"sync"
	"testing"
)

func TestSafeSliceConcurrentAppend(t *testing.T) {
	s := New[int]()
	var wg sync.WaitGroup
	numGoroutines := 10
	itemsPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				s.Append(base*itemsPerGoroutine + j)
			}
		}(i)
	}
	wg.Wait()

	if s.Len() != numGoroutines*itemsPerGoroutine {
		t.Errorf("Expected length %d, got %d", numGoroutines*itemsPerGoroutine, s.Len())
	}
}

func TestSafeSliceConcurrentReadWrite(t *testing.T) {
	s := New[int]()
	for i := 0; i < 100; i++ {
		s.Append(i)
	}

	var wg sync.WaitGroup
	numReaders := 5
	numWriters := 3

	// 启动读取器
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = s.Get(j)
			}
		}()
	}

	// 启动写入器
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				s.Set(j, id*1000+j)
			}
		}(i)
	}

	wg.Wait()
	// 测试完成没有死锁或崩溃
}

func TestTransaction(t *testing.T) {
	s := FromSlice([]int{1, 2, 3, 4, 5})

	// 创建事务并修改数据
	tx := s.Begin()
	tx.Set(0, 100)
	tx.Append(6, 7)
	tx.Delete(2)

	// 提交前，原始数据应该不变
	if val, _ := s.Get(0); val != 1 {
		t.Errorf("Original slice should not be modified before commit, got %d", val)
	}
	if s.Len() != 5 {
		t.Errorf("Original slice length should be 5 before commit, got %d", s.Len())
	}

	// 提交事务
	if !tx.Commit() {
		t.Errorf("Transaction commit failed")
	}

	// 提交后，原始数据应该被修改
	if val, _ := s.Get(0); val != 100 {
		t.Errorf("Original slice should be modified after commit, got %d", val)
	}
	if s.Len() != 6 { // 5 - 1 + 2 = 6
		t.Errorf("Original slice length should be 6 after commit, got %d", s.Len())
	}

	// 应该无法在提交后进行操作
	if tx.Set(0, 200) {
		t.Errorf("Should not be able to modify after commit")
	}
}

func TestTransactionRollback(t *testing.T) {
	s := FromSlice([]int{1, 2, 3, 4, 5})

	// 创建事务并修改数据
	tx := s.Begin()
	tx.Set(0, 100)
	tx.Delete(1)

	// 回滚事务
	if !tx.Rollback() {
		t.Errorf("Transaction rollback failed")
	}

	// 回滚后，原始数据应该不变
	if val, _ := s.Get(0); val != 1 {
		t.Errorf("Original slice should not be modified after rollback, got %d", val)
	}
	if s.Len() != 5 {
		t.Errorf("Original slice length should remain 5 after rollback, got %d", s.Len())
	}
}

func TestConcurrentTransactions(t *testing.T) {
	s := New[int]()
	for i := 0; i < 10; i++ {
		s.Append(i)
	}

	var wg sync.WaitGroup
	numTransactions := 5

	wg.Add(numTransactions)
	for i := 0; i < numTransactions; i++ {
		go func(id int) {
			defer wg.Done()

			// 一半提交，一半回滚
			tx := s.Begin()
			tx.Append(100 + id)

			if id%2 == 0 {
				tx.Commit()
			} else {
				tx.Rollback()
			}
		}(i)
	}

	wg.Wait()

	// 原始切片长度应该为 10 + 提交的事务数量
	expectedLen := 10 + (numTransactions+1)/2 // 向上取整
	if s.Len() != expectedLen {
		t.Errorf("Expected length %d after concurrent transactions, got %d", expectedLen, s.Len())
	}
}

func TestAtomicOperations(t *testing.T) {
	s := FromSlice([]int{10, 20, 30, 40, 50})

	// 测试 CompareAndSwap
	equals := func(a, b int) bool { return a == b }

	// 成功的 CAS
	if !s.CompareAndSwap(1, 20, 200, equals) {
		t.Errorf("CompareAndSwap should succeed when values match")
	}
	if val, _ := s.Get(1); val != 200 {
		t.Errorf("Value should be updated after successful CAS, got %d", val)
	}

	// 失败的 CAS
	if s.CompareAndSwap(2, 35, 300, equals) {
		t.Errorf("CompareAndSwap should fail when values don't match")
	}
	if val, _ := s.Get(2); val != 30 {
		t.Errorf("Value should not change after failed CAS, got %d", val)
	}

	// 测试 AtomicUpdate
	s.AtomicUpdate(3, func(val int) int {
		return val * 2
	})
	if val, _ := s.Get(3); val != 80 {
		t.Errorf("Value should be doubled after AtomicUpdate, got %d", val)
	}

	// 测试 GetOrCompute
	val, isComputed := s.GetOrCompute(5, func() int {
		return 600
	})
	if !isComputed || val != 600 {
		t.Errorf("GetOrCompute should add a new value, got %d, isComputed: %v", val, isComputed)
	}

	// 测试 FindAndUpdate
	s.FindAndUpdate(
		func(val int) bool { return val > 50 },
		func(val int) int { return val + 1 },
	)

	if val, _ := s.Get(1); val != 201 {
		t.Errorf("FindAndUpdate should update the first matching value (index 1), got %d", val)
	}
}

func TestIterator(t *testing.T) {
	s := FromSlice([]int{1, 2, 3, 4, 5})
	it := NewIterator(s)

	// 测试 Find
	val, found := it.Find(func(val int) bool {
		return val > 3
	})
	if !found || val != 4 {
		t.Errorf("Iterator.Find should return first match, got %d, found: %v", val, found)
	}

	// 测试 Any
	if !it.Any(func(val int) bool { return val == 5 }) {
		t.Errorf("Iterator.Any should return true when element exists")
	}
	if it.Any(func(val int) bool { return val > 10 }) {
		t.Errorf("Iterator.Any should return false when no element matches")
	}

	// 测试 All
	if !it.All(func(val int) bool { return val > 0 }) {
		t.Errorf("Iterator.All should return true when all elements match")
	}
	if it.All(func(val int) bool { return val > 2 }) {
		t.Errorf("Iterator.All should return false when not all elements match")
	}

	// 测试 Filter
	filtered := it.Filter(func(val int) bool {
		return val%2 == 0
	})
	if filtered.Len() != 2 {
		t.Errorf("Filter should return slice with only matching elements, got length %d", filtered.Len())
	}

	// 测试 Map 函数
	doubled := Map(s, func(val int) int {
		return val * 2
	})
	if doubled.Len() != 5 {
		t.Errorf("Map should return slice with same length, got %d", doubled.Len())
	}
	if val, _ := doubled.Get(2); val != 6 {
		t.Errorf("Map should transform values, got %d", val)
	}
}
