package server

import (
	"math/rand"
	"sync"

	"github.com/ndsky1003/net/conn"
)

// Session 包装 net 连接
type Session struct {
	Sid    string
	Weight int
	Conn   *conn.Conn
}

// ServiceGroup 管理同名服务的多个连接
type ServiceGroup struct {
	sync.RWMutex
	Sessions    []*Session
	TotalWeight int
	Name        string
}

func NewServiceGroup(name string) *ServiceGroup {
	return &ServiceGroup{
		Name:     name,
		Sessions: make([]*Session, 0),
	}
}

// Add 添加连接并更新总权重
func (sg *ServiceGroup) Add(s *Session) {
	sg.Lock()
	defer sg.Unlock()
	sg.Sessions = append(sg.Sessions, s)
	if s.Weight > 0 {
		sg.TotalWeight += s.Weight
	}
}

// Remove 移除连接
func (sg *ServiceGroup) Remove(sid string) {
	sg.Lock()
	defer sg.Unlock()
	for i, s := range sg.Sessions {
		if s.Sid == sid {
			if s.Weight > 0 {
				sg.TotalWeight -= s.Weight
			}
			sg.Sessions = append(sg.Sessions[:i], sg.Sessions[i+1:]...)
			return
		}
	}
}

// Select 加权随机选择 (需求 3)
func (sg *ServiceGroup) Select() *Session {
	sg.RLock()
	defer sg.RUnlock()

	if len(sg.Sessions) == 0 {
		return nil
	}

	if sg.TotalWeight <= 0 {
		return sg.Sessions[rand.Intn(len(sg.Sessions))]
	}

	r := rand.Intn(sg.TotalWeight)
	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}
		r -= s.Weight
		if r < 0 {
			return s
		}
	}
	return sg.Sessions[0]
}

// GetBySid 指定获取 (需求 4)
func (sg *ServiceGroup) GetBySid(sid string) *Session {
	sg.RLock()
	defer sg.RUnlock()
	for _, s := range sg.Sessions {
		if s.Sid == sid {
			return s
		}
	}
	return nil
}

// GetAll 获取全部 (需求 4 广播用)
func (sg *ServiceGroup) GetAll() []*Session {
	sg.RLock()
	defer sg.RUnlock()
	// 返回副本防止并发问题
	result := make([]*Session, len(sg.Sessions))
	copy(result, sg.Sessions)
	return result
}
