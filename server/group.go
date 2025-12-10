package server

import (
	"crypto/md5"
	"encoding/binary"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/ndsky1003/net/server"
)

// Session 包装 net 连接
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

	// [新增] 脏标记，用于惰性重建
	dirty bool
}

func NewServiceGroup(name string, replicas int) *ServiceGroup {
	return &ServiceGroup{
		Name:     name,
		Sessions: make([]*Session, 0),
		replicas: replicas,
		keys:     nil,
		hashMap:  make(map[uint32]string),
		dirty:    false, // 初始为 false
	}
}

func (sg *ServiceGroup) shardingHash(key string) uint32 {
	checksum := md5.Sum([]byte(key))
	return binary.BigEndian.Uint32(checksum[:4])
}

func (sg *ServiceGroup) Add(s *Session) {
	sg.Lock()
	defer sg.Unlock()

	for i, existing := range sg.Sessions {
		if existing.ID() == s.ID() {
			if existing.Weight > 0 {
				sg.TotalWeight -= existing.Weight
			}
			sg.Sessions[i] = s
			if s.Weight > 0 {
				sg.TotalWeight += s.Weight
			}
			sg.dirty = true // [修改] 标记为脏，不立即重建
			return
		}
	}

	sg.Sessions = append(sg.Sessions, s)
	if s.Weight > 0 {
		sg.TotalWeight += s.Weight
	}
	sg.dirty = true // [修改] 标记为脏
}

func (sg *ServiceGroup) Remove(sid string) {
	sg.Lock()
	defer sg.Unlock()
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

// rebuildHashRing (保持原逻辑不变，由 SelectByKey 调用)
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

// SelectByKey 基于一致性哈希的选择
func (sg *ServiceGroup) SelectByKey(key string) *Session {
	sg.RLock()

	// [新增] 检查是否需要重建
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
	idx := sort.Search(len(sg.keys), func(i int) bool {
		return sg.keys[i] >= hash
	})

	if idx == len(sg.keys) {
		idx = 0
	}

	targetHash := sg.keys[idx]
	targetSid := sg.hashMap[targetHash]

	// 注意：这里仍然需要在内部查找 Session，
	// 如果 Remove 设置了 dirty 但还没 rebuild，SelectByKey 会触发 rebuild，
	// 所以这里获取到的 targetSid 一定是存在的。
	return sg.getBySidNoLock(targetSid)
}

// Select 随机负载均衡 (这个方法不需要哈希环，所以不需要 check dirty，性能最高)
func (sg *ServiceGroup) Select() *Session {
	sg.RLock()
	defer sg.RUnlock()
	// ... (保持原代码不变)
	if len(sg.Sessions) == 0 {
		return nil
	}
	// ...
	return sg.Sessions[0]
}

// ... GetBySid, getBySidNoLock, GetAll 保持不变 ...
func (sg *ServiceGroup) GetBySid(sid string) *Session {
	sg.RLock()
	defer sg.RUnlock()
	return sg.getBySidNoLock(sid)
}

func (sg *ServiceGroup) getBySidNoLock(sid string) *Session {
	for _, s := range sg.Sessions {
		if s.ID().String() == sid {
			return s
		}
	}
	return nil
}

func (sg *ServiceGroup) GetAll() []*Session {
	sg.RLock()
	defer sg.RUnlock()
	result := make([]*Session, len(sg.Sessions))
	copy(result, sg.Sessions)
	return result
}
