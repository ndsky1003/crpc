package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type broadcastCounterItem struct {
	count atomic.Int32
	timer *time.Timer
}

type broadcastCounterGroup struct {
	l     sync.Mutex
	items map[uint64]*broadcastCounterItem
}

type broadcastCounterAll struct {
	l      sync.Mutex
	groups map[uuid.UUID]*broadcastCounterGroup
}

func (s *server_mgr) setBroadcastCount(id uuid.UUID, seq uint64, count int32, timeout time.Duration) {
	s.broadcastCounter.l.Lock()
	defer s.broadcastCounter.l.Unlock()

	if s.broadcastCounter.groups == nil {
		s.broadcastCounter.groups = make(map[uuid.UUID]*broadcastCounterGroup)
	}
	group, ok := s.broadcastCounter.groups[id]
	if !ok {
		group = &broadcastCounterGroup{
			items: make(map[uint64]*broadcastCounterItem),
		}
		s.broadcastCounter.groups[id] = group
	}
	group.l.Lock()
	defer group.l.Unlock()

	item := &broadcastCounterItem{}
	item.count.Store(count)
	item.timer = time.AfterFunc(timeout, func() {
		// 超时清理，防止内存泄漏
		s.broadcastCounter.l.Lock()
		defer s.broadcastCounter.l.Unlock()
		group.l.Lock()
		defer group.l.Unlock()
		delete(group.items, seq)
	})
	group.items[seq] = item
}

func (s *server_mgr) decreaseBroadcastCount(id uuid.UUID, seq uint64) int32 {
	s.broadcastCounter.l.Lock()
	defer s.broadcastCounter.l.Unlock()

	group, ok := s.broadcastCounter.groups[id]
	if !ok {
		return -1
	}
	group.l.Lock()
	defer group.l.Unlock()

	item, ok := group.items[seq]
	if !ok {
		return -1
	}
	remain := item.count.Add(-1)
	if remain <= 0 {
		if item.timer != nil {
			item.timer.Stop()
		}
		delete(group.items, seq)
	}
	return remain
}
