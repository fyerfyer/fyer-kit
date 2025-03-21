package set

import (
	"sync"
	"testing"
)

func TestConcurrentSet_ThreadSafety(t *testing.T) {
	// 创建并发安全集合
	set := NewConcurrent[int]()

	// 并发写入的元素数量
	numElements := 1000

	// 使用WaitGroup等待所有goroutine完成
	var wg sync.WaitGroup

	// 启动多个goroutine并发添加元素
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()

			// 每个goroutine添加100个元素
			for j := 0; j < 100; j++ {
				num := start*100 + j
				set.Add(num)
			}
		}(i)
	}

	// 同时启动读取goroutine
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 重复查询元素
			for j := 0; j < 500; j++ {
				_ = set.Contains(j % numElements)
				_ = set.Size()
			}
		}()
	}

	// 等待所有goroutine完成
	wg.Wait()

	// 验证集合包含所有期望的元素
	if set.Size() != numElements {
		t.Errorf("ConcurrentSet size incorrect after concurrent operations. Expected %d, got %d",
			numElements, set.Size())
	}

	// 验证所有元素都被正确添加
	for i := 0; i < numElements; i++ {
		if !set.Contains(i) {
			t.Errorf("ConcurrentSet missing element %d", i)
		}
	}
}

func TestConcurrentSet_ConcurrentSetOperations(t *testing.T) {
	// 创建两个并发集合
	set1 := NewConcurrent[int](1, 3, 5, 7, 9)
	set2 := NewConcurrent[int](2, 3, 5, 8)

	// 使用WaitGroup等待所有操作完成
	var wg sync.WaitGroup

	// 并发执行集合操作
	wg.Add(4)

	var union, intersection, difference, symmetricDifference Set[int]

	// 并发执行并集
	go func() {
		defer wg.Done()
		union = set1.Union(set2)
	}()

	// 并发执行交集
	go func() {
		defer wg.Done()
		intersection = set1.Intersection(set2)
	}()

	// 并发执行差集
	go func() {
		defer wg.Done()
		difference = set1.Difference(set2)
	}()

	// 并发执行对称差集
	go func() {
		defer wg.Done()
		symmetricDifference = set1.SymmetricDifference(set2)
	}()

	// 等待所有操作完成
	wg.Wait()

	// 验证操作结果
	if union.Size() != 7 {
		t.Errorf("Union size incorrect. Expected 7, got %d", union.Size())
	}

	if intersection.Size() != 2 {
		t.Errorf("Intersection size incorrect. Expected 2, got %d", intersection.Size())
	}

	if difference.Size() != 3 {
		t.Errorf("Difference size incorrect. Expected 3, got %d", difference.Size())
	}

	if symmetricDifference.Size() != 5 {
		t.Errorf("SymmetricDifference size incorrect. Expected 5, got %d", symmetricDifference.Size())
	}
}

func TestConcurrentSet_BatchOperations(t *testing.T) {
	// 创建并发安全集合
	set := NewConcurrent[int]()

	// 测试批量添加
	addedCount := set.AddAll(1, 2, 3, 4, 5, 3) // 3 重复

	if addedCount != 5 {
		t.Errorf("AddAll should return count of added elements. Expected 5, got %d", addedCount)
	}

	if set.Size() != 5 {
		t.Errorf("After AddAll, size should be 5, got %d", set.Size())
	}

	// 测试ContainsAll
	if !set.ContainsAll(1, 3, 5) {
		t.Errorf("ContainsAll should return true when all elements are in the set")
	}

	if set.ContainsAll(1, 3, 6) {
		t.Errorf("ContainsAll should return false when any element is not in the set")
	}

	// 测试批量删除
	removedCount := set.RemoveAll(1, 3, 7) // 7 不存在

	if removedCount != 2 {
		t.Errorf("RemoveAll should return count of removed elements. Expected 2, got %d", removedCount)
	}

	if set.Size() != 3 {
		t.Errorf("After RemoveAll, size should be 3, got %d", set.Size())
	}

	// 测试RetainAll
	set = NewConcurrent[int](1, 2, 3, 4, 5)
	removedByRetain := set.RetainAll(2, 4, 6) // 保留 2,4，6不在集合中

	if removedByRetain != 3 {
		t.Errorf("RetainAll should return count of removed elements. Expected 3, got %d", removedByRetain)
	}

	if set.Size() != 2 {
		t.Errorf("After RetainAll, size should be 2, got %d", set.Size())
	}

	if !set.ContainsAll(2, 4) || set.Contains(1) || set.Contains(3) || set.Contains(5) {
		t.Errorf("After RetainAll, set should contain only 2 and 4")
	}
}

func TestConcurrentSet_ConcurrentBatchOperations(t *testing.T) {
	// 测试并发批量操作
	set := NewConcurrent[int]()

	// 初始填充集合
	for i := 0; i < 1000; i++ {
		set.Add(i)
	}

	// 使用WaitGroup等待所有goroutine完成
	var wg sync.WaitGroup

	// 并发执行批量操作
	wg.Add(3)

	// 批量添加线程
	go func() {
		defer wg.Done()
		// 添加一些已存在的和新的元素
		for i := 0; i < 5; i++ {
			set.AddAll(900+i, 1000+i, 1100+i)
		}
	}()

	// 批量删除线程
	go func() {
		defer wg.Done()
		// 删除一些元素
		for i := 0; i < 5; i++ {
			set.RemoveAll(i*100, i*100+1, i*100+2)
		}
	}()

	// ContainsAll检查线程
	go func() {
		defer wg.Done()
		// 重复检查元素
		for i := 0; i < 100; i++ {
			// 随机挑选一些元素检查
			set.ContainsAll(500, 501, 502)
			set.ContainsAll(800, 801, 802)
		}
	}()

	// 等待所有goroutine完成
	wg.Wait()

	// 不需要验证具体的值，只要没有panic就表示并发安全
	// 但可以做一些基本检查
	if set.Size() <= 0 {
		t.Errorf("After concurrent operations, set should not be empty")
	}

	// 验证一些应该被批量删除的元素
	for i := 0; i < 5; i++ {
		if set.Contains(i*100) || set.Contains(i*100+1) || set.Contains(i*100+2) {
			t.Errorf("Element %d should have been removed", i*100)
		}
	}

	// 验证一些应该被批量添加的元素
	for i := 0; i < 5; i++ {
		if !set.Contains(1000+i) || !set.Contains(1100+i) {
			t.Errorf("Element %d should have been added", 1000+i)
		}
	}
}
