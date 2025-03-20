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
