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

	// 明确计算期望提交的事务数量
	expectedCommits := 0
	for i := 0; i < numTransactions; i++ {
		if i%2 == 0 {
			expectedCommits++
		}
	}

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
	expectedLen := 10 + expectedCommits
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

func TestBatchOperations(t *testing.T) {
	// 初始化测试数据
	s := FromSlice([]int{10, 20, 30, 40, 50})

	// 测试 GetMany
	indices := []int{1, 3, 4}
	values, allSuccess := s.GetMany(indices)
	if !allSuccess {
		t.Errorf("GetMany should succeed with valid indices")
	}
	if len(values) != 3 || values[0] != 20 || values[1] != 40 || values[2] != 50 {
		t.Errorf("GetMany returned incorrect values: %v", values)
	}

	// 测试部分无效索引的 GetMany
	invalidIndices := []int{1, 10, 3}
	values, allSuccess = s.GetMany(invalidIndices)
	if allSuccess {
		t.Errorf("GetMany should report partial failure with invalid indices")
	}
	if len(values) != 3 || values[0] != 20 || values[2] != 40 {
		t.Errorf("GetMany with invalid indices returned incorrect values: %v", values)
	}

	// 测试 BatchUpdate
	updates := map[int]int{
		0: 100,
		2: 300,
		4: 500,
	}
	count, allSuccess := s.BatchUpdate(updates)
	if !allSuccess || count != 3 {
		t.Errorf("BatchUpdate should succeed with valid indices, got count=%d, allSuccess=%v", count, allSuccess)
	}

	for idx, expected := range updates {
		if val, _ := s.Get(idx); val != expected {
			t.Errorf("Value at index %d should be %d after BatchUpdate, got %d", idx, expected, val)
		}
	}

	// 测试部分无效索引的 BatchUpdate
	invalidUpdates := map[int]int{
		1:  200,
		10: 1000,
	}
	count, allSuccess = s.BatchUpdate(invalidUpdates)
	if allSuccess {
		t.Errorf("BatchUpdate should report partial failure with invalid indices")
	}
	if count != 1 {
		t.Errorf("BatchUpdate should update only valid indices, got count=%d", count)
	}
	if val, _ := s.Get(1); val != 200 {
		t.Errorf("Value at index 1 should be updated to 200, got %d", val)
	}

	// 测试 BatchDelete
	s = FromSlice([]int{10, 20, 30, 40, 50, 60, 70})
	deleteIndices := []int{5, 2, 0} // 删除索引 5, 2, 0 (注意顺序不是从大到小)
	count, allSuccess = s.BatchDelete(deleteIndices)
	if !allSuccess || count != 3 {
		t.Errorf("BatchDelete should succeed with valid indices, got count=%d, allSuccess=%v", count, allSuccess)
	}
	if s.Len() != 4 {
		t.Errorf("Slice length should be 4 after BatchDelete, got %d", s.Len())
	}
	if val, _ := s.Get(0); val != 20 {
		t.Errorf("Value at index 0 should be 20 after BatchDelete, got %d", val)
	}

	// 测试 RangeAccess
	s = FromSlice([]int{10, 20, 30, 40, 50})
	sum := 0
	s.RangeAccess(func(items []int) {
		for _, item := range items {
			sum += item
		}
	})
	if sum != 150 {
		t.Errorf("RangeAccess should allow read access to all items, sum should be 150, got %d", sum)
	}

	// 测试 BatchOperation
	s = FromSlice([]int{10, 20, 30, 40, 50})
	success := s.BatchOperation(func(items []int) ([]int, bool) {
		// 将所有元素乘以2
		for i := range items {
			items[i] *= 2
		}
		return items, true
	})
	if !success {
		t.Errorf("BatchOperation should succeed when returning true")
	}
	if val, _ := s.Get(2); val != 60 {
		t.Errorf("Value at index 2 should be 60 after BatchOperation, got %d", val)
	}

	// 测试取消的 BatchOperation
	beforeLen := s.Len()
	success = s.BatchOperation(func(items []int) ([]int, bool) {
		// 添加元素但取消操作
		return append(items, 999), false
	})
	if success {
		t.Errorf("BatchOperation should not succeed when returning false")
	}
	if s.Len() != beforeLen {
		t.Errorf("Slice length should not change after cancelled BatchOperation, got %d", s.Len())
	}
}

func TestConcurrentBatchOperations(t *testing.T) {
	s := FromSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	var wg sync.WaitGroup
	concurrency := 5

	// 并发 BatchUpdate 测试
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			updates := map[int]int{
				id:            id * 100,
				(id + 5) % 10: id * 200,
			}
			s.BatchUpdate(updates)
		}(i)
	}
	wg.Wait()

	// 检查长度没有变化
	if s.Len() != 10 {
		t.Errorf("Slice length should remain 10 after concurrent BatchUpdate, got %d", s.Len())
	}

	// 并发 RangeAccess 和 BatchOperation 测试
	s = FromSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	wg.Add(concurrency * 2)

	// 启动读取器
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			s.RangeAccess(func(items []int) {
				sum := 0
				for _, item := range items {
					sum += item
				}
				// 只读取，不修改
			})
		}()
	}

	// 启动写入器
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			s.BatchOperation(func(items []int) ([]int, bool) {
				// 只修改自己负责的部分
				start := (id * 2) % 10
				if start < len(items) {
					items[start] += 1000
				}
				if start+1 < len(items) {
					items[start+1] += 1000
				}
				return items, true
			})
		}(i)
	}

	wg.Wait()

	// 检查长度没有变化
	if s.Len() != 10 {
		t.Errorf("Slice length should remain 10 after concurrent operations, got %d", s.Len())
	}

	// 所有元素应该都被修改过
	modified := 0
	s.RangeAccess(func(items []int) {
		for _, item := range items {
			if item >= 1000 {
				modified++
			}
		}
	})
	if modified == 0 {
		t.Errorf("Some elements should be modified after concurrent BatchOperations, but none was found")
	}
}

func TestBatchTransactionOperations(t *testing.T) {
	// 初始化测试数据
	s := FromSlice([]int{10, 20, 30, 40, 50})

	// 为 Transaction 添加批量操作支持的测试
	tx := s.Begin()

	// 在事务中执行多个操作
	tx.Set(0, 100)
	tx.Set(1, 200)
	tx.Delete(2)
	tx.Append(60, 70)

	// 事务提交前的状态
	if s.Len() != 5 {
		t.Errorf("Original slice should remain unchanged before commit")
	}

	// 提交事务
	tx.Commit()

	// 验证所有批量操作被原子地应用
	if s.Len() != 6 { // 5 - 1 + 2 = 6
		t.Errorf("Slice length should be 6 after transaction commit, got %d", s.Len())
	}

	expected := []int{100, 200, 40, 50, 60, 70}
	for i, exp := range expected {
		if val, _ := s.Get(i); val != exp {
			t.Errorf("Value at index %d should be %d after transaction commit, got %d", i, exp, val)
		}
	}
}
