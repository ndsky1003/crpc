package server

import (
	"crypto/md5"
	"encoding/binary"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/ndsky1003/net/server"
)

type Session struct {
	Name   string
	Weight int
	server.Session
}

// ServiceGroup 管理同名服务的多个连接
type ServiceGroup struct {
	sync.RWMutex
	Sessions    []*Session
	TotalWeight int
	Name        string

	// [一致性哈希支持]
	replicas int
	keys     []uint32
	hashMap  map[uint32]string

	// 脏标记，用于惰性重建
	dirty bool

	//  辅助 Map，实现 O(1) 查找 Session
	sessionMap map[string]*Session
}

func NewServiceGroup(name string, replicas int) *ServiceGroup {
	return &ServiceGroup{
		Name:       name,
		Sessions:   make([]*Session, 0),
		replicas:   replicas,
		keys:       nil,
		hashMap:    make(map[uint32]string),
		dirty:      false,                     // 初始为 false
		sessionMap: make(map[string]*Session), // 初始化 Map
	}
}

func (sg *ServiceGroup) shardingHash(key string) uint32 {
	checksum := md5.Sum([]byte(key))
	return binary.BigEndian.Uint32(checksum[:4])
}

func (sg *ServiceGroup) Add(s *Session) {
	sg.Lock()
	defer sg.Unlock()

	sg.sessionMap[s.ID().String()] = s

	for i, existing := range sg.Sessions {
		if existing.ID() == s.ID() {
			if existing.Weight > 0 {
				sg.TotalWeight -= existing.Weight
			}
			sg.Sessions[i] = s
			if s.Weight > 0 {
				sg.TotalWeight += s.Weight
			}
			sg.dirty = true
			return
		}
	}

	sg.Sessions = append(sg.Sessions, s)
	if s.Weight > 0 {
		sg.TotalWeight += s.Weight
	}
	sg.dirty = true
}

func (sg *ServiceGroup) Remove(sid string) {
	sg.Lock()
	defer sg.Unlock()

	delete(sg.sessionMap, sid)

	for i, s := range sg.Sessions {
		if s.ID().String() == sid {
			if s.Weight > 0 {
				sg.TotalWeight -= s.Weight
			}
			sg.Sessions = append(sg.Sessions[:i], sg.Sessions[i+1:]...)
			sg.dirty = true // [修改] 标记为脏
			return
		}
	}
}

func (sg *ServiceGroup) rebuildHashRing() {
	sg.keys = make([]uint32, 0)
	sg.hashMap = make(map[uint32]string)

	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}
		numVirtualNodes := s.Weight * sg.replicas
		for i := range numVirtualNodes {
			hash := sg.shardingHash(s.ID().String() + "#" + strconv.Itoa(i))
			sg.keys = append(sg.keys, hash)
			sg.hashMap[hash] = s.ID().String()
		}
	}
	slices.Sort(sg.keys)
}

func (sg *ServiceGroup) SelectByKey(key string) *Session {
	sg.RLock()

	if sg.dirty {
		sg.RUnlock() // 释放读锁
		sg.Lock()    // 获取写锁
		// 双重检查
		if sg.dirty {
			sg.rebuildHashRing()
			sg.dirty = false
		}
		sg.Unlock() // 释放写锁
		sg.RLock()  // 重新获取读锁
	}

	defer sg.RUnlock()

	if len(sg.keys) == 0 {
		return nil
	}

	hash := sg.shardingHash(key)
	// 二分查找第一个 >= hash 的节点
	idx := sort.Search(len(sg.keys), func(i int) bool {
		return sg.keys[i] >= hash
	})

	if idx == len(sg.keys) {
		idx = 0
	}

	targetHash := sg.keys[idx]
	targetSid := sg.hashMap[targetHash]

	// [优化] 直接从 Map 获取，O(1)
	return sg.sessionMap[targetSid]
}

// Select 随机负载均衡 (O(1) 复杂度，不涉及哈希环重建)
func (sg *ServiceGroup) Select() *Session {
	sg.RLock()
	defer sg.RUnlock()

	length := len(sg.Sessions)
	if length == 0 {
		return nil
	}

	// 如果只有一个，直接返回
	if length == 1 {
		return sg.Sessions[0]
	}

	// 随机选择一个
	return sg.Sessions[rand.Intn(length)]
}

// GetBySid 根据 SID 获取 Session
func (sg *ServiceGroup) GetBySid(sid string) *Session {
	sg.RLock()
	defer sg.RUnlock()

	// [优化] 直接从 Map 获取，O(1)
	return sg.sessionMap[sid]
}

func (sg *ServiceGroup) GetAll() []*Session {
	sg.RLock()
	defer sg.RUnlock()
	result := make([]*Session, len(sg.Sessions))
	copy(result, sg.Sessions)
	return result
}
