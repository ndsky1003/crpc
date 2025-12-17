package server

import (
	crand "crypto/rand"
	"hash/fnv"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ndsky1003/net/server"
)

// 补充缺失的 ID 方法定义（适配嵌入的 server.Session）
type Session struct {
	Name           string
	Weight         int
	server.Session // 假设 server.Session 有 ID() 方法返回唯一标识（如 uint64/string）
}

// 确保 ID() 方法可调用（若 server.Session 的 ID() 返回非 string 类型，需适配）
func (s *Session) ID() string {
	// 示例：若底层 Session.ID() 返回 uint64，需转换
	// return strconv.FormatUint(s.Session.ID(), 10)
	return s.Session.ID().String() // 假设底层 ID() 返回 fmt.Stringer 或直接返回 string
}

// consistentHashRing 封装一致性哈希环的数据，用于原子替换
type consistentHashRing struct {
	keys    []uint32
	hashMap map[uint32]string
}

// ServiceGroup 管理同名服务的多个连接
type ServiceGroup struct {
	// 读写锁：读操作（Select/SelectByKey/Get*）用 RLock，写操作（Add/Remove）用 Lock
	sync.RWMutex
	Sessions    []*Session
	TotalWeight int
	Name        string

	// 使用原子变量存储哈希环，读操作无锁
	ring atomic.Value // Stores *consistentHashRing

	// [一致性哈希参数]
	replicas int // 每个权重单位的虚拟节点数

	// 辅助 Map，实现 O(1) 查找 Session
	sessionMap map[string]*Session

	// 并发安全的随机数生成器（替代全局 rand）
	randomLock sync.Mutex
	random     *rand.Rand
}

// NewServiceGroup 创建服务组，默认 replicas=100（避免传入 0）
func NewServiceGroup(name string, replicas int) *ServiceGroup {
	if replicas <= 0 {
		replicas = 100 // 默认每个权重单位生成 100 个虚拟节点
	}

	// 初始化并发安全的随机数生成器
	randomSeed, _ := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	random := rand.New(rand.NewSource(randomSeed.Int64()))

	sg := &ServiceGroup{
		Name:       name,
		Sessions:   make([]*Session, 0),
		replicas:   replicas,
		sessionMap: make(map[string]*Session),
		random:     random,
	}

	// 初始化空哈希环
	sg.ring.Store(&consistentHashRing{
		keys:    []uint32{},
		hashMap: make(map[uint32]string),
	})

	return sg
}

// hashFunc 使用 FNV-1a 算法，返回 uint32
func (sg *ServiceGroup) hashFunc(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key)) // 忽略写入错误（字节写入无错误）
	return h.Sum32()
}

// Add 添加/更新 Session，线程安全
func (sg *ServiceGroup) Add(s *Session) {
	sg.Lock()
	defer sg.Unlock()

	// 校验权重（确保非负）
	if s.Weight < 0 {
		s.Weight = 0
	}

	sid := s.ID()
	oldSession, exists := sg.sessionMap[sid]

	// 处理权重变更
	if exists {
		if oldSession.Weight > 0 {
			sg.TotalWeight -= oldSession.Weight
		}
		// 替换切片中的旧节点
		for i, existing := range sg.Sessions {
			if existing.ID() == sid {
				sg.Sessions[i] = s
				break
			}
		}
	} else {
		// 新增节点
		sg.Sessions = append(sg.Sessions, s)
	}

	// 更新总权重和映射
	if s.Weight > 0 {
		sg.TotalWeight += s.Weight
	}
	sg.sessionMap[sid] = s

	// 重建哈希环
	sg.rebuildHashRing()
}

// Remove 移除指定 ID 的 Session，线程安全
func (sg *ServiceGroup) Remove(sid string) {
	sg.Lock()
	defer sg.Unlock()

	oldSession, exists := sg.sessionMap[sid]
	if !exists {
		return
	}

	// 扣除权重
	if oldSession.Weight > 0 {
		sg.TotalWeight -= oldSession.Weight
	}

	// 从切片中删除
	for i, s := range sg.Sessions {
		if s.ID() == sid {
			sg.Sessions = append(sg.Sessions[:i], sg.Sessions[i+1:]...)
			break
		}
	}

	// 删除映射
	delete(sg.sessionMap, sid)

	// 重建哈希环
	sg.rebuildHashRing()
}

// rebuildHashRing 构建新环并原子替换（仅被加锁的写操作调用）
func (sg *ServiceGroup) rebuildHashRing() {
	capacity := sg.TotalWeight * sg.replicas
	newKeys := make([]uint32, 0, capacity)
	newHashMap := make(map[uint32]string, capacity)

	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}

		// 按权重生成虚拟节点（权重为 N 则生成 N*replicas 个虚拟节点）
		numVirtualNodes := s.Weight * sg.replicas
		baseKey := s.ID() + "#"

		for i := 0; i < numVirtualNodes; i++ {
			// 生成虚拟节点 key：基础 ID + 虚拟节点索引
			virtualKey := baseKey + strconv.Itoa(i)
			hash := sg.hashFunc(virtualKey)

			// 处理哈希碰撞（追加随机后缀）
			for {
				if _, exists := newHashMap[hash]; !exists {
					break
				}
				// 碰撞时添加随机数后缀重新哈希
				virtualKey += "#" + strconv.Itoa(sg.random.Intn(1000000))
				hash = sg.hashFunc(virtualKey)
			}

			newKeys = append(newKeys, hash)
			newHashMap[hash] = s.ID()
		}
	}

	// 排序哈希环
	sort.Slice(newKeys, func(i, j int) bool {
		return newKeys[i] < newKeys[j]
	})

	// 原子替换哈希环
	sg.ring.Store(&consistentHashRing{
		keys:    newKeys,
		hashMap: newHashMap,
	})
}

// SelectByKey 一致性哈希选择节点（无锁读环，读映射时用 RLock）
func (sg *ServiceGroup) SelectByKey(key string) *Session {
	// 1. 原子加载哈希环（无锁）
	ringVal := sg.ring.Load()
	ring, ok := ringVal.(*consistentHashRing)
	if !ok || len(ring.keys) == 0 {
		return nil
	}

	// 2. 计算 key 的哈希值
	hash := sg.hashFunc(key)

	// 3. 二分查找第一个大于等于 hash 的节点
	idx := sort.Search(len(ring.keys), func(i int) bool {
		return ring.keys[i] >= hash
	})

	// 4. 处理边界：哈希值大于所有节点时，取第一个节点
	if idx == len(ring.keys) {
		idx = 0
	}

	// 5. 获取目标节点 ID（读映射无锁，因为 ring 是不可变的）
	targetSid := ring.hashMap[ring.keys[idx]]

	// 6. 读 sessionMap（用 RLock 保证并发安全）
	sg.RLock()
	defer sg.RUnlock()
	return sg.sessionMap[targetSid]
}

// Select 加权随机选择节点（读锁保护，按权重分配概率）
func (sg *ServiceGroup) Select() *Session {
	sg.RLock()
	defer sg.RUnlock()

	if len(sg.Sessions) == 0 || sg.TotalWeight <= 0 {
		return nil
	}
	if len(sg.Sessions) == 1 {
		return sg.Sessions[0]
	}

	// 加权随机：按总权重生成随机数，遍历找到对应的节点
	sg.randomLock.Lock()
	r := sg.random.Intn(sg.TotalWeight)
	sg.randomLock.Unlock()

	current := 0
	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}
		current += s.Weight
		if current > r {
			return s
		}
	}

	// 兜底：返回第一个有效节点
	for _, s := range sg.Sessions {
		if s.Weight > 0 {
			return s
		}
	}
	return nil
}

// GetBySid 线程安全地获取 Session（读锁）
func (sg *ServiceGroup) GetBySid(sid string) *Session {
	sg.RLock()
	defer sg.RUnlock()
	return sg.sessionMap[sid]
}

// GetAll 线程安全地获取所有 Session 副本（读锁）
func (sg *ServiceGroup) GetAll() []*Session {
	sg.RLock()
	defer sg.RUnlock()
	result := make([]*Session, len(sg.Sessions))
	copy(result, sg.Sessions)
	return result
}

// GetTotalWeight 返回总权重（读锁）
func (sg *ServiceGroup) GetTotalWeight() int {
	sg.RLock()
	defer sg.RUnlock()
	return sg.TotalWeight
}
