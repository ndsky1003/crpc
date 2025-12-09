package server

import (
	"crypto/md5"
	"encoding/binary"
	"math/rand"
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
	// 虚拟节点扩充倍数，权重越高，虚拟节点越多，分布越均匀
	replicas int
	// 哈希环：存储排序后的哈希值
	keys []uint32
	// 哈希值到 Sid 的映射
	hashMap map[uint32]string
}

func NewServiceGroup(name string, replicas int) *ServiceGroup {
	return &ServiceGroup{
		Name:     name,
		Sessions: make([]*Session, 0),
		replicas: replicas, // 默认倍率，可调整
		keys:     nil,
		hashMap:  make(map[uint32]string),
	}
}

// shardingHash 生成更强的一致性哈希值 (使用 MD5)
// 相比 CRC32，MD5 的碰撞概率极低，分布更均匀，适合一致性哈希环
func (sg *ServiceGroup) shardingHash(key string) uint32 {
	checksum := md5.Sum([]byte(key))
	// 取 MD5 的前 4 个字节转为 uint32
	return binary.BigEndian.Uint32(checksum[:4])
}

func (sg *ServiceGroup) Add(s *Session) {
	sg.Lock()
	defer sg.Unlock()

	// --- 修复开始 ---
	// 先检查是否已存在，避免重复添加导致权重计算错误
	for i, existing := range sg.Sessions {
		if existing.ID() == s.ID() {
			// 如果已存在，更新权重和连接，而不是追加
			if existing.Weight > 0 {
				sg.TotalWeight -= existing.Weight
			}
			sg.Sessions[i] = s // 更新
			if s.Weight > 0 {
				sg.TotalWeight += s.Weight
			}
			sg.rebuildHashRing() // 权重变了，需要重建
			return
		}
	}
	// --- 修复结束 ---

	sg.Sessions = append(sg.Sessions, s)
	if s.Weight > 0 {
		sg.TotalWeight += s.Weight
	}
	sg.rebuildHashRing()
}

// Remove 移除连接并更新哈希环
func (sg *ServiceGroup) Remove(sid string) {
	sg.Lock()
	defer sg.Unlock()
	for i, s := range sg.Sessions {
		if s.ID().String() == sid {
			if s.Weight > 0 {
				sg.TotalWeight -= s.Weight
			}
			// 删除切片元素
			sg.Sessions = append(sg.Sessions[:i], sg.Sessions[i+1:]...)

			// 重建哈希环
			sg.rebuildHashRing()
			return
		}
	}
}

// rebuildHashRing 重建一致性哈希环
// 当服务上线或下线时调用，操作成本为 O(N * Replicas * logN)
func (sg *ServiceGroup) rebuildHashRing() {
	sg.keys = make([]uint32, 0)
	sg.hashMap = make(map[uint32]string)

	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}
		// 根据权重生成虚拟节点
		numVirtualNodes := s.Weight * sg.replicas
		for i := 0; i < numVirtualNodes; i++ {
			// 生成虚拟节点 Key: sid#0, sid#1 ...
			// 使用 MD5 替代 CRC32
			hash := sg.shardingHash(s.ID().String() + "#" + strconv.Itoa(i))
			sg.keys = append(sg.keys, hash)
			sg.hashMap[hash] = s.ID().String()
		}
	}
	// 排序，方便二分查找
	sort.Slice(sg.keys, func(i, j int) bool {
		return sg.keys[i] < sg.keys[j]
	})
}

// SelectByKey 基于一致性哈希的选择 (粘性会话 / 分片上传)
// 性能：O(log N)
func (sg *ServiceGroup) SelectByKey(key string) *Session {
	sg.RLock()
	defer sg.RUnlock()

	if len(sg.keys) == 0 {
		return nil
	}

	// 1. 计算 Key 的哈希 (MD5)
	hash := sg.shardingHash(key)

	// 2. 二分查找：找到第一个 >= hash 的虚拟节点
	idx := sort.Search(len(sg.keys), func(i int) bool {
		return sg.keys[i] >= hash
	})

	// 3. 环状结构：如果没找到（idx == len），则绕回第一个
	if idx == len(sg.keys) {
		idx = 0
	}

	// 4. 映射回真实的 Session
	targetHash := sg.keys[idx]
	targetSid := sg.hashMap[targetHash]

	return sg.getBySidNoLock(targetSid)
}

// Select 随机加权负载均衡 (普通 RPC)
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

// GetBySid 指定获取
func (sg *ServiceGroup) GetBySid(sid string) *Session {
	sg.RLock()
	defer sg.RUnlock()
	return sg.getBySidNoLock(sid)
}

// 内部无锁查找，复用代码
func (sg *ServiceGroup) getBySidNoLock(sid string) *Session {
	for _, s := range sg.Sessions {
		if s.ID().String() == sid {
			return s
		}
	}
	return nil
}

// GetAll 获取全部 (广播用)
func (sg *ServiceGroup) GetAll() []*Session {
	sg.RLock()
	defer sg.RUnlock()
	// 返回副本，防止并发读写切片
	result := make([]*Session, len(sg.Sessions))
	copy(result, sg.Sessions)
	return result
}
