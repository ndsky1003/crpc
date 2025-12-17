package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var itemPool = sync.Pool{
	New: func() any {
		return &broadcastCounterItem{}
	},
}

type broadcastCounterItem struct {
	count atomic.Int32
	timer *time.Timer
}

type broadcastCounterGroup struct {
	l         sync.Mutex
	items     map[uint64]*broadcastCounterItem
	updatedAt int64 // 用于记录最后活跃时间，辅助清理
}

type broadcastCounterAll struct {
	l          sync.RWMutex // 改为读写锁，读多写少场景更有利
	groups     map[uuid.UUID]*broadcastCounterGroup
	quit       chan struct{}
	expiration time.Duration
}

// NewBroadcastCounterAll 初始化并启动后台清理协程
func NewBroadcastCounterAll(t time.Duration) *broadcastCounterAll {
	b := &broadcastCounterAll{
		groups:     make(map[uuid.UUID]*broadcastCounterGroup),
		quit:       make(chan struct{}),
		expiration: t,
	}
	// 启动后台清理任务，每分钟执行一次
	go b.startCleanupLoop(1 * time.Minute)
	return b
}

// Stop 停止清理协程（用于程序退出时）
func (b *broadcastCounterAll) Stop() {
	close(b.quit)
}

// setBroadcastCount 设置计数
func (s *broadcastCounterAll) setBroadcastCount(id uuid.UUID, seq uint64, count int32, timeout time.Duration) {
	// 1. 获取 Group (使用双重检查锁定，尽量减少写锁持有时间)
	s.l.RLock()
	group, ok := s.groups[id]
	s.l.RUnlock()

	if !ok {
		s.l.Lock()
		// 二次检查
		group, ok = s.groups[id]
		if !ok {
			group = &broadcastCounterGroup{
				items: make(map[uint64]*broadcastCounterItem),
			}
			s.groups[id] = group
		}
		s.l.Unlock()
	}

	// 2. 操作 Group
	group.l.Lock()
	defer group.l.Unlock()

	// 更新活跃时间
	atomic.StoreInt64(&group.updatedAt, time.Now().Unix())

	// 从对象池获取 item
	item := itemPool.Get().(*broadcastCounterItem)
	item.count.Store(count)

	// 注意：必须先清理旧的 timer（虽然逻辑上不太可能重复 set 同一个 seq，但为了安全）
	if oldItem, exists := group.items[seq]; exists {
		if oldItem.timer != nil {
			oldItem.timer.Stop()
		}
		// 归还旧对象回池子前需要重置状态吗？通常由 Put 前或者 Get 后做
		itemPool.Put(oldItem)
	}

	group.items[seq] = item

	// 3. 设置超时
	item.timer = time.AfterFunc(timeout, func() {
		group.l.Lock()
		defer group.l.Unlock()

		if target, exists := group.items[seq]; exists && target == item {
			delete(group.items, seq)
			target.timer = nil
			itemPool.Put(target) // 归还对象
		}
	})
}

// decreaseBroadcastCount 减少计数
func (s *broadcastCounterAll) decreaseBroadcastCount(id uuid.UUID, seq uint64) int32 {
	s.l.RLock()
	group, ok := s.groups[id]
	s.l.RUnlock()

	if !ok {
		return -1
	}

	group.l.Lock()
	defer group.l.Unlock()

	atomic.StoreInt64(&group.updatedAt, time.Now().Unix())

	item, ok := group.items[seq]
	if !ok {
		return -1
	}

	remain := item.count.Add(-1)
	if remain <= 0 {
		if remain == 0 {
			if item.timer != nil {
				item.timer.Stop()
				item.timer = nil
			}
			delete(group.items, seq)
			itemPool.Put(item) // 归还对象
		}
		return 0
	}
	return remain
}

// startCleanupLoop 后台清理空 Group 的逻辑
func (s *broadcastCounterAll) startCleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.quit:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *broadcastCounterAll) cleanup() {
	var expiration = s.expiration
	threshold := time.Now().Add(-expiration).Unix()

	s.l.RLock()
	var candidates []uuid.UUID

	for id, group := range s.groups {
		group.l.Lock()

		isEmpty := len(group.items) == 0

		lastActive := atomic.LoadInt64(&group.updatedAt)
		isExpired := lastActive < threshold

		if isEmpty && isExpired {
			candidates = append(candidates, id)
		}

		group.l.Unlock()
	}
	s.l.RUnlock()

	if len(candidates) == 0 {
		return
	}

	s.l.Lock()
	defer s.l.Unlock()

	for _, id := range candidates {
		group, exists := s.groups[id]
		if !exists {
			continue
		}

		group.l.Lock()
		if len(group.items) == 0 {
			delete(s.groups, id)
		}
		group.l.Unlock()
	}
}
