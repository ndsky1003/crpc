package server

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ndsky1003/net/v2/server"
)

var (
	// ErrEmptySessionID Session ID 为空错误
	ErrEmptySessionID = errors.New("session ID is empty")
	// ErrNoAvailableSession 没有可用的 Session
	ErrNoAvailableSession = errors.New("no available session")
	// ErrInconsistentState 状态不一致错误
	ErrInconsistentState = errors.New("inconsistent state detected")
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
	id := s.Session.ID().String()
	if id == "" {
		// 生成一个默认 ID 而不是 panic
		slog.Warn("Session.ID() returned empty string, using fallback",
			"name", s.Name,
			"weight", s.Weight)
		return fmt.Sprintf("session_%p", s) // 使用指针地址作为后备 ID
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

	// 使用 sync.Pool 复用随机数生成器，减少锁竞争
	randomPool *sync.Pool

	// 监控指标
	selectCount    atomic.Uint64 // 选择操作计数
	rebuildCount   atomic.Uint64 // 重建哈希环计数
	collisionCount atomic.Uint64 // 哈希碰撞计数
}

// NewServiceGroup 创建服务组
func NewServiceGroup(name string, replicas int) (*ServiceGroup, error) {
	if name == "" {
		return nil, errors.New("service group name cannot be empty")
	}

	if replicas <= 0 {
		replicas = 100 // 默认每个权重单位生成 100 个虚拟节点
	}

	// 初始化加密安全的随机数种子
	randomSeed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		// 如果加密随机失败，使用时间戳作为后备
		slog.Warn("failed to generate crypto random seed, using timestamp",
			"error", err)
		randomSeed = big.NewInt(time.Now().UnixNano())
	}

	sg := &ServiceGroup{
		Name:       name,
		Sessions:   make([]*Session, 0),
		replicas:   replicas,
		sessionMap: make(map[string]*Session),
		randomPool: &sync.Pool{
			New: func() interface{} {
				// 每个 goroutine 创建独立的随机数生成器
				return rand.New(rand.NewSource(time.Now().UnixNano()))
			},
		},
	}

	// 初始化第一个随机数生成器
	sg.randomPool.Put(rand.New(rand.NewSource(randomSeed.Int64())))

	// 初始化空哈希环
	sg.ring.Store(&consistentHashRing{
		keys:       []uint32{},
		sessionMap: make(map[uint32]*Session),
	})

	return sg, nil
}

// hashFunc 使用 FNV-1a 算法计算哈希值
func (sg *ServiceGroup) hashFunc(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32()
}

// Add 添加或更新 Session（线程安全）
func (sg *ServiceGroup) Add(s *Session) error {
	if s == nil {
		return errors.New("session cannot be nil")
	}

	// 权重 ≤ 0 视为删除操作
	if s.Weight <= 0 {
		return sg.Remove(s.ID())
	}

	// 验证 Session ID
	sid := s.ID()
	if sid == "" {
		return ErrEmptySessionID
	}

	sg.Lock()
	defer sg.Unlock()

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

	// 异步重建哈希环，减少锁持有时间
	go sg.asyncRebuildHashRing()

	return nil
}

// Remove 移除指定 Session（线程安全）
func (sg *ServiceGroup) Remove(sid string) error {
	if sid == "" {
		return ErrEmptySessionID
	}

	sg.Lock()
	defer sg.Unlock()

	oldSession, exists := sg.sessionMap[sid]
	if !exists {
		return fmt.Errorf("session %s not found", sid)
	}

	// 扣除权重
	if oldSession.Weight > 0 {
		sg.TotalWeight -= oldSession.Weight
	}

	// 从切片中删除（优化：使用最后一个元素替换）
	for i, s := range sg.Sessions {
		if s.ID() == sid {
			// 将最后一个元素移到删除位置
			lastIdx := len(sg.Sessions) - 1
			if i != lastIdx {
				sg.Sessions[i] = sg.Sessions[lastIdx]
			}
			sg.Sessions = sg.Sessions[:lastIdx]
			break
		}
	}

	// 删除映射
	delete(sg.sessionMap, sid)

	// 异步重建哈希环
	go sg.asyncRebuildHashRing()

	return nil
}

// asyncRebuildHashRing 异步重建哈希环（避免阻塞写操作）
func (sg *ServiceGroup) asyncRebuildHashRing() {
	// 复制必要的数据
	sg.RLock()
	sessions := make([]*Session, len(sg.Sessions))
	copy(sessions, sg.Sessions)
	replicas := sg.replicas
	sg.RUnlock()

	// 在锁外重建哈希环
	newRing := sg.buildHashRing(sessions, replicas)

	// 原子替换
	sg.ring.Store(newRing)
	sg.rebuildCount.Add(1)
}

// buildHashRing 构建新的哈希环（不持有锁）
func (sg *ServiceGroup) buildHashRing(sessions []*Session, replicas int) *consistentHashRing {
	// 1. 计算实际容量
	actualCapacity := 0
	for _, s := range sessions {
		if s.Weight > 0 {
			actualCapacity += s.Weight * replicas
		}
	}

	newKeys := make([]uint32, 0, actualCapacity)
	newSessionMap := make(map[uint32]*Session, actualCapacity)

	// 获取随机数生成器
	randomObj := sg.randomPool.Get()
	random, ok := randomObj.(*rand.Rand)
	if !ok {
		// 如果类型不匹配，创建一个新的随机数生成器
		slog.Error("Invalid type in randomPool, creating new rand.Rand")
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	defer sg.randomPool.Put(random)

	// 2. 为每个节点生成虚拟节点
	for _, s := range sessions {
		if s.Weight <= 0 {
			continue
		}

		numVirtualNodes := s.Weight * replicas
		baseKey := s.ID() + "#"

		for i := 0; i < numVirtualNodes; i++ {
			virtualKey := baseKey + strconv.Itoa(i)
			hash := sg.hashFunc(virtualKey)

			// 改进的哈希碰撞处理（设置最大重试次数）
			maxRetries := 10
			for retries := 0; retries < maxRetries; retries++ {
				if _, exists := newSessionMap[hash]; !exists {
					break
				}

				sg.collisionCount.Add(1)

				// 使用更复杂的后缀避免连续碰撞
				randomSuffix := random.Intn(1000000)
				virtualKey = fmt.Sprintf("%s#%d#%d", baseKey, i, randomSuffix)
				hash = sg.hashFunc(virtualKey)

				if retries == maxRetries-1 {
					// 如果还是碰撞，跳过这个虚拟节点
					slog.Warn("max hash collision retries reached",
						"session", s.ID(),
						"virtualNode", i)
					continue
				}
			}

			newKeys = append(newKeys, hash)
			newSessionMap[hash] = s
		}
	}

	// 3. 排序哈希环
	sort.Slice(newKeys, func(i, j int) bool {
		return newKeys[i] < newKeys[j]
	})

	return &consistentHashRing{
		keys:       newKeys,
		sessionMap: newSessionMap,
	}
}

// SelectByKey 根据 key 使用一致性哈希选择节点（完全无锁）
func (sg *ServiceGroup) SelectByKey(key string) (*Session, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	// 1. 原子加载哈希环
	ringVal := sg.ring.Load()
	ring, ok := ringVal.(*consistentHashRing)
	if !ok || len(ring.keys) == 0 {
		return nil, ErrNoAvailableSession
	}

	// 2. 计算 key 的哈希值
	hash := sg.hashFunc(key)

	// 3. 二分查找第一个 >= hash 的虚拟节点
	idx := sort.Search(len(ring.keys), func(i int) bool {
		return ring.keys[i] >= hash
	})

	// 4. 处理环形边界
	if idx == len(ring.keys) {
		idx = 0
	}

	sg.selectCount.Add(1)

	// 5. 直接返回 Session 指针
	session := ring.sessionMap[ring.keys[idx]]
	if session == nil {
		return nil, ErrNoAvailableSession
	}

	return session, nil
}

// Select 加权随机选择节点（用于负载均衡）
func (sg *ServiceGroup) Select() (*Session, error) {
	sg.RLock()
	defer sg.RUnlock()

	if len(sg.Sessions) == 0 || sg.TotalWeight <= 0 {
		return nil, ErrNoAvailableSession
	}

	if len(sg.Sessions) == 1 {
		sg.selectCount.Add(1)
		return sg.Sessions[0], nil
	}

	// 获取随机数生成器
	randomObj := sg.randomPool.Get()
	random, ok := randomObj.(*rand.Rand)
	if !ok {
		slog.Error("Invalid type in randomPool, creating new rand.Rand")
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	defer sg.randomPool.Put(random)

	// 加权随机：生成 [0, TotalWeight) 范围内的随机数
	r := random.Intn(sg.TotalWeight)

	// 遍历累加权重，找到对应区间的节点
	current := 0
	for _, s := range sg.Sessions {
		if s.Weight <= 0 {
			continue
		}
		current += s.Weight
		if current > r {
			sg.selectCount.Add(1)
			return s, nil
		}
	}

	// 数据不一致时记录错误并返回第一个可用节点
	slog.Error("inconsistent state detected",
		"totalWeight", sg.TotalWeight,
		"sessionCount", len(sg.Sessions))

	// 降级处理：返回第一个权重大于 0 的节点
	for _, s := range sg.Sessions {
		if s.Weight > 0 {
			return s, nil
		}
	}

	return nil, ErrInconsistentState
}

// GetBySid 根据 Session ID 获取 Session（线程安全）
func (sg *ServiceGroup) GetBySid(sid string) (*Session, error) {
	if sid == "" {
		return nil, ErrEmptySessionID
	}

	sg.RLock()
	defer sg.RUnlock()

	session, exists := sg.sessionMap[sid]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sid)
	}

	return session, nil
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
	NodeCount        int    // 节点数量
	TotalWeight      int    // 总权重
	VirtualNodeCount int    // 虚拟节点数量
	SelectCount      uint64 // 选择操作计数
	RebuildCount     uint64 // 重建哈希环计数
	CollisionCount   uint64 // 哈希碰撞计数
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
		SelectCount:      sg.selectCount.Load(),
		RebuildCount:     sg.rebuildCount.Load(),
		CollisionCount:   sg.collisionCount.Load(),
	}
}

// Reset 重置服务组（清空所有节点）
func (sg *ServiceGroup) Reset() {
	sg.Lock()
	defer sg.Unlock()

	sg.Sessions = sg.Sessions[:0]
	sg.sessionMap = make(map[string]*Session)
	sg.TotalWeight = 0

	// 重置哈希环
	sg.ring.Store(&consistentHashRing{
		keys:       []uint32{},
		sessionMap: make(map[uint32]*Session),
	})

	// 重置计数器
	sg.selectCount.Store(0)
	sg.rebuildCount.Store(0)
	sg.collisionCount.Store(0)
}

// IsEmpty 检查服务组是否为空
func (sg *ServiceGroup) IsEmpty() bool {
	sg.RLock()
	defer sg.RUnlock()
	return len(sg.Sessions) == 0
}

// SessionCount 返回 Session 数量
func (sg *ServiceGroup) SessionCount() int {
	sg.RLock()
	defer sg.RUnlock()
	return len(sg.Sessions)
}
