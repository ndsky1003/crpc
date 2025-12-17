package server

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// MockSession 用于测试的模拟 Session
type MockSession struct {
	id string
}

func (m *MockSession) ID() fmt.Stringer {
	return mockStringer{id: m.id}
}

type mockStringer struct {
	id string
}

func (m mockStringer) String() string {
	return m.id
}

// TestNewServiceGroup 测试创建服务组
func TestNewServiceGroup(t *testing.T) {
	tests := []struct {
		name     string
		sgName   string
		replicas int
		wantErr  bool
	}{
		{"valid group", "test-service", 100, false},
		{"zero replicas", "test-service", 0, false}, // 应该使用默认值 100
		{"negative replicas", "test-service", -10, false}, // 应该使用默认值 100
		{"empty name", "", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg, err := NewServiceGroup(tt.sgName, tt.replicas)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServiceGroup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sg == nil {
				t.Error("Expected non-nil ServiceGroup")
			}
		})
	}
}

// TestAddRemoveSession 测试添加和删除 Session
func TestAddRemoveSession(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	// 测试添加 Session
	s1 := &Session{
		Name:    "server1",
		Weight:  10,
		Session: &MockSession{id: "session1"},
	}

	err := sg.Add(s1)
	if err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	// 验证添加成功
	if sg.SessionCount() != 1 {
		t.Errorf("Expected 1 session, got %d", sg.SessionCount())
	}

	if sg.GetTotalWeight() != 10 {
		t.Errorf("Expected total weight 10, got %d", sg.GetTotalWeight())
	}

	// 测试更新 Session（相同 ID，不同权重）
	s1Updated := &Session{
		Name:    "server1",
		Weight:  20,
		Session: &MockSession{id: "session1"},
	}

	err = sg.Add(s1Updated)
	if err != nil {
		t.Fatalf("Failed to update session: %v", err)
	}

	// 验证更新成功
	if sg.SessionCount() != 1 {
		t.Errorf("Expected 1 session after update, got %d", sg.SessionCount())
	}

	if sg.GetTotalWeight() != 20 {
		t.Errorf("Expected total weight 20 after update, got %d", sg.GetTotalWeight())
	}

	// 测试删除 Session
	err = sg.Remove("session1")
	if err != nil {
		t.Fatalf("Failed to remove session: %v", err)
	}

	// 验证删除成功
	if sg.SessionCount() != 0 {
		t.Errorf("Expected 0 sessions after removal, got %d", sg.SessionCount())
	}

	if sg.GetTotalWeight() != 0 {
		t.Errorf("Expected total weight 0 after removal, got %d", sg.GetTotalWeight())
	}

	// 测试删除不存在的 Session
	err = sg.Remove("non-existent")
	if err == nil {
		t.Error("Expected error when removing non-existent session")
	}
}

// TestSelectByKey 测试一致性哈希选择
func TestSelectByKey(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	// 添加多个 Session
	sessions := []*Session{
		{Name: "server1", Weight: 10, Session: &MockSession{id: "session1"}},
		{Name: "server2", Weight: 20, Session: &MockSession{id: "session2"}},
		{Name: "server3", Weight: 30, Session: &MockSession{id: "session3"}},
	}

	for _, s := range sessions {
		if err := sg.Add(s); err != nil {
			t.Fatalf("Failed to add session: %v", err)
		}
	}

	// 等待异步重建完成
	time.Sleep(100 * time.Millisecond)

	// 测试相同 key 总是选择相同的节点
	key := "test-key-123"
	first, err := sg.SelectByKey(key)
	if err != nil {
		t.Fatalf("Failed to select by key: %v", err)
	}

	for i := 0; i < 10; i++ {
		selected, err := sg.SelectByKey(key)
		if err != nil {
			t.Fatalf("Failed to select by key: %v", err)
		}
		if selected.ID() != first.ID() {
			t.Errorf("Inconsistent selection for same key: expected %s, got %s",
				first.ID(), selected.ID())
		}
	}

	// 测试空 key
	_, err = sg.SelectByKey("")
	if err == nil {
		t.Error("Expected error for empty key")
	}
}

// TestSelect 测试加权随机选择
func TestSelect(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	// 添加不同权重的 Session
	sessions := []*Session{
		{Name: "server1", Weight: 100, Session: &MockSession{id: "session1"}},
		{Name: "server2", Weight: 1, Session: &MockSession{id: "session2"}},
	}

	for _, s := range sessions {
		if err := sg.Add(s); err != nil {
			t.Fatalf("Failed to add session: %v", err)
		}
	}

	// 统计选择分布
	counts := make(map[string]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		selected, err := sg.Select()
		if err != nil {
			t.Fatalf("Failed to select: %v", err)
		}
		counts[selected.ID()]++
	}

	// 验证选择分布大致符合权重比例
	// server1 应该被选中约 99% 的时间
	server1Ratio := float64(counts["session1"]) / float64(iterations)
	if server1Ratio < 0.95 || server1Ratio > 1.0 {
		t.Errorf("Unexpected selection ratio for server1: %f", server1Ratio)
	}
}

// TestConcurrentOperations 测试并发操作
func TestConcurrentOperations(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// 并发添加 Session
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				session := &Session{
					Name:    fmt.Sprintf("server-%d-%d", id, j),
					Weight:  id + j + 1,
					Session: &MockSession{id: fmt.Sprintf("session-%d-%d", id, j)},
				}
				if err := sg.Add(session); err != nil {
					t.Errorf("Failed to add session: %v", err)
				}
			}
		}(i)
	}

	// 并发选择操作
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				sg.Select()
				sg.SelectByKey(fmt.Sprintf("key-%d", j))
			}
		}()
	}

	// 并发获取指标
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				sg.GetMetrics()
				sg.GetAll()
				sg.GetTotalWeight()
			}
		}()
	}

	wg.Wait()

	// 验证最终状态
	metrics := sg.GetMetrics()
	expectedSessions := numGoroutines * numOperations
	if metrics.NodeCount != expectedSessions {
		t.Errorf("Expected %d sessions, got %d", expectedSessions, metrics.NodeCount)
	}
}

// TestEmptySessionID 测试空 Session ID 的处理
func TestEmptySessionID(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	// 创建一个返回空 ID 的 MockSession
	emptySession := &Session{
		Name:    "empty-id-server",
		Weight:  10,
		Session: &MockSession{id: ""},
	}

	// 应该使用后备 ID
	id := emptySession.ID()
	if id == "" {
		t.Error("Expected non-empty fallback ID")
	}

	// 添加应该成功
	err := sg.Add(emptySession)
	if err != nil {
		t.Fatalf("Failed to add session with empty ID: %v", err)
	}
}

// TestMetrics 测试指标收集
func TestMetrics(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	// 添加一些 Session
	for i := 0; i < 5; i++ {
		session := &Session{
			Name:    fmt.Sprintf("server-%d", i),
			Weight:  10,
			Session: &MockSession{id: fmt.Sprintf("session-%d", i)},
		}
		sg.Add(session)
	}

	// 执行一些操作
	for i := 0; i < 100; i++ {
		sg.Select()
		sg.SelectByKey(fmt.Sprintf("key-%d", i))
	}

	// 等待异步操作完成
	time.Sleep(100 * time.Millisecond)

	metrics := sg.GetMetrics()

	if metrics.NodeCount != 5 {
		t.Errorf("Expected 5 nodes, got %d", metrics.NodeCount)
	}

	if metrics.TotalWeight != 50 {
		t.Errorf("Expected total weight 50, got %d", metrics.TotalWeight)
	}

	if metrics.SelectCount != 200 {
		t.Errorf("Expected 200 select operations, got %d", metrics.SelectCount)
	}

	// 虚拟节点数应该是 权重 * replicas = 50 * 10 = 500
	if metrics.VirtualNodeCount != 500 {
		t.Errorf("Expected 500 virtual nodes, got %d", metrics.VirtualNodeCount)
	}
}

// TestReset 测试重置功能
func TestReset(t *testing.T) {
	sg, _ := NewServiceGroup("test-service", 10)

	// 添加一些 Session
	for i := 0; i < 5; i++ {
		session := &Session{
			Name:    fmt.Sprintf("server-%d", i),
			Weight:  10,
			Session: &MockSession{id: fmt.Sprintf("session-%d", i)},
		}
		sg.Add(session)
	}

	// 执行重置
	sg.Reset()

	// 验证重置后的状态
	if !sg.IsEmpty() {
		t.Error("ServiceGroup should be empty after reset")
	}

	if sg.GetTotalWeight() != 0 {
		t.Errorf("Expected total weight 0 after reset, got %d", sg.GetTotalWeight())
	}

	metrics := sg.GetMetrics()
	if metrics.SelectCount != 0 || metrics.RebuildCount != 0 || metrics.CollisionCount != 0 {
		t.Error("All counters should be 0 after reset")
	}
}

// BenchmarkSelectByKey 基准测试：一致性哈希选择
func BenchmarkSelectByKey(b *testing.B) {
	sg, _ := NewServiceGroup("bench-service", 100)

	// 添加 100 个 Session
	for i := 0; i < 100; i++ {
		session := &Session{
			Name:    fmt.Sprintf("server-%d", i),
			Weight:  10,
			Session: &MockSession{id: fmt.Sprintf("session-%d", i)},
		}
		sg.Add(session)
	}

	// 等待异步重建完成
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sg.SelectByKey(fmt.Sprintf("key-%d", i))
			i++
		}
	})
}

// BenchmarkSelect 基准测试：加权随机选择
func BenchmarkSelect(b *testing.B) {
	sg, _ := NewServiceGroup("bench-service", 100)

	// 添加 100 个 Session
	for i := 0; i < 100; i++ {
		session := &Session{
			Name:    fmt.Sprintf("server-%d", i),
			Weight:  10,
			Session: &MockSession{id: fmt.Sprintf("session-%d", i)},
		}
		sg.Add(session)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sg.Select()
		}
	})
}