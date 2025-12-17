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

// Session 包装底层 server.Session，添加服务发现所需的元数据
type Session struct {
	Name           string
	Weight         int
	server.Session // 假设 server.Session 有 ID() 方法返回唯一标识
}

// ID 返回 Session 的唯一标识（适配底层 Session）
func (s *Session) ID() string {
	// 假设底层 Session.ID() 返回 fmt.Stringer 或直接返回 string
	// 若返回 uint64，需改为: return strconv.FormatUint(s.Session.ID(), 10)
	id := s.Session.ID().String()
	if id == "" {
		panic("Session.ID() returned empty string")
	}
	return id
}

// consistentHashRing 一致性哈希环的不可变快照（支持无锁读）
type consistentHashRing struct {
	keys       []uint32            // 排序后的哈希值数组
	sessionMap map[uint32]*Session // 哈希值 -> Session 指针（直接存储，避免二次查找）
}

// ServiceGroup 管理同名服务的多个连接
type ServiceGroup struct {
	sync.RWMutex                     // 保护 Sessions/sessionMap/TotalWeight
	Sessions     []*Session          // 所有有效节点（Weight > 0）
	TotalWeight  int                 // 所有节点权重之和
	Name         string              // 服务组名称
	ring         atomic.Value        // 存储 *consistentHashRing（原子读写）
	replicas     int                 // 每个权重单位的虚拟节点数
	sessionMap   map[string]*Session // sid -> Session 的快速查找表

	// 并发安全的随机数生成器
	randomLock sync.Mutex
	random     *rand.Rand
}

// NewServiceGroup 创建服务组
func NewServiceGroup(name string, replicas int) *ServiceGroup {
	if replicas <= 0 {
		replicas = 100 // 默认每个权重单位生成 100 个虚拟节点
	}

	// 初始化加密安全的随机数种子
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
		keys:       []uint32{},
		sessionMap: make(map[uint32]*Session),
	})

	return sg
}

// hashFunc 使用 FNV-1a 算法计算哈希值
func (sg *ServiceGroup) hashFunc(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32()
}

// Add 添加或更新 Session（线程安全）
func (sg *ServiceGroup) Add(s *Session) {
	// 权重 ≤ 0 视为删除操作
	if s.Weight <= 0 {
		sg.Remove(s.ID())
		return
	}

	sg.Lock()
	defer sg.Unlock()

	sid := s.ID()
	oldSession, exists := sg.sessionMap[sid]

	if exists {
		// 更新现有节点
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
	sg.TotalWeight += s.Weight
	sg.sessionMap[sid] = s

	// 重建哈希环
	sg.rebuildHashRing()
}

// Remove 移除指定 Session（线程安全）
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

// rebuildHashRing 重建一致性哈希环（仅在持有写锁时调用）
func (sg *ServiceGroup) rebuildHashRing() {
	// 1. 计算实际容量（仅统计 Weight > 0 的节点）
	actualCapacity := 0
	for _, s := range sg.Sessions {
		if s.Weight > 0 {
			actualCapacity += s.Weight * sg.replicas
		}
	}

	newKeys := make([]uint32, 0, actualCapacity)
	newSessionMap := make(map[uint32]*Session, actualCapacity)

	// 2. 为每个节点生成虚拟节点
	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}

		numVirtualNodes := s.Weight * sg.replicas
		baseKey := s.ID() + "#"

		for i := 0; i < numVirtualNodes; i++ {
			virtualKey := baseKey + strconv.Itoa(i)
			hash := sg.hashFunc(virtualKey)

			// 处理哈希碰撞（追加随机后缀重新哈希）
			for {
				if _, exists := newSessionMap[hash]; !exists {
					break
				}
				// 并发安全地生成随机数
				sg.randomLock.Lock()
				randomSuffix := sg.random.Intn(1000000)
				sg.randomLock.Unlock()

				virtualKey += "#" + strconv.Itoa(randomSuffix)
				hash = sg.hashFunc(virtualKey)
			}

			newKeys = append(newKeys, hash)
			newSessionMap[hash] = s // 直接存储 Session 指针
		}
	}

	// 3. 排序哈希环
	sort.Slice(newKeys, func(i, j int) bool {
		return newKeys[i] < newKeys[j]
	})

	// 4. 原子替换哈希环
	sg.ring.Store(&consistentHashRing{
		keys:       newKeys,
		sessionMap: newSessionMap,
	})
}

// SelectByKey 根据 key 使用一致性哈希选择节点（完全无锁）
func (sg *ServiceGroup) SelectByKey(key string) *Session {
	// 1. 原子加载哈希环
	ringVal := sg.ring.Load()
	ring, ok := ringVal.(*consistentHashRing)
	if !ok || len(ring.keys) == 0 {
		return nil
	}

	// 2. 计算 key 的哈希值
	hash := sg.hashFunc(key)

	// 3. 二分查找第一个 >= hash 的虚拟节点
	idx := sort.Search(len(ring.keys), func(i int) bool {
		return ring.keys[i] >= hash
	})

	// 4. 处理环形边界（哈希值大于所有节点时回绕到第一个）
	if idx == len(ring.keys) {
		idx = 0
	}

	// 5. 直接返回 Session 指针（无需二次查找）
	return ring.sessionMap[ring.keys[idx]]
}

// Select 加权随机选择节点（用于负载均衡）
func (sg *ServiceGroup) Select() *Session {
	sg.RLock()
	defer sg.RUnlock()

	if len(sg.Sessions) == 0 || sg.TotalWeight <= 0 {
		return nil
	}
	if len(sg.Sessions) == 1 {
		return sg.Sessions[0]
	}

	// 加权随机：生成 [0, TotalWeight) 范围内的随机数
	sg.randomLock.Lock()
	r := sg.random.Intn(sg.TotalWeight)
	sg.randomLock.Unlock()

	// 遍历累加权重，找到对应区间的节点
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

	// 不应到达此处（数据不一致时触发 panic）
	panic("inconsistent state: TotalWeight > 0 but no valid session selected")
}

// GetBySid 根据 Session ID 获取 Session（线程安全）
func (sg *ServiceGroup) GetBySid(sid string) *Session {
	sg.RLock()
	defer sg.RUnlock()
	return sg.sessionMap[sid]
}

// GetAll 返回所有 Session 的副本（线程安全）
func (sg *ServiceGroup) GetAll() []*Session {
	sg.RLock()
	defer sg.RUnlock()
	result := make([]*Session, len(sg.Sessions))
	copy(result, sg.Sessions)
	return result
}

// GetTotalWeight 返回总权重（线程安全）
func (sg *ServiceGroup) GetTotalWeight() int {
	sg.RLock()
	defer sg.RUnlock()
	return sg.TotalWeight
}

// Metrics 服务组的统计指标
type Metrics struct {
	NodeCount        int // 节点数量
	TotalWeight      int // 总权重
	VirtualNodeCount int // 虚拟节点数量
}

// GetMetrics 获取当前统计指标（线程安全）
func (sg *ServiceGroup) GetMetrics() Metrics {
	sg.RLock()
	defer sg.RUnlock()

	ring := sg.ring.Load().(*consistentHashRing)
	return Metrics{
		NodeCount:        len(sg.Sessions),
		TotalWeight:      sg.TotalWeight,
		VirtualNodeCount: len(ring.keys),
	}
}
