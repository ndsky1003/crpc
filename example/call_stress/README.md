# Call 压力测试

这是一个针对 CRPC Call 功能的压力测试示例,可以跑满整个 CPU 并每 5 分钟打印一次性能指标。

## CRPC 架构说明

CRPC 的架构设计如下:
- **Server**: 仅作为数据转发中心,不提供业务服务
- **Client**: 既是调用端,又是服务提供者

本压力测试包含三个部分:
1. **server**: 数据转发服务器
2. **provider**: 服务提供者(Client),注册并处理业务请求
3. **caller**: 调用者(Client),发起压力测试请求

## 目录结构

```
call_stress/
├── server/
│   ├── main.go           # Server 主程序(仅转发)
│   └── server            # 编译后的二进制文件
├── provider/
│   ├── main.go           # Provider 主程序
│   ├── types.go          # 类型定义
│   ├── types_gen.go      # 自动生成的代码
│   └── provider          # 编译后的二进制文件
├── caller/
│   ├── main.go           # Caller 主程序(压力测试)
│   ├── types.go          # 类型定义
│   ├── types_gen.go      # 自动生成的代码
│   └── caller            # 编译后的二进制文件
└── README.md             # 本文件
```

## 功能特点

- **多并发压力测试**: 根据 CPU 核心数启动对应数量的 worker,跑满整个 CPU
- **对象池优化**: 使用 `sync.Pool` 复用请求对象,减少 GC 压力
- **延迟统计**: 记录最小、最大、平均延迟(微秒级)
- **吞吐量监控**: 实时计算每秒请求数(req/s)
- **资源监控**: 监控 goroutine 数量、内存使用、GC 次数等
- **定期报告**: 每 5 分钟打印一次详细统计指标
- **pprof 支持**: 内置 pprof HTTP 服务器,方便性能分析

## 快速开始

### 1. 编译

```bash
# 生成代码
go generate ./example/call_stress/provider
go generate ./example/call_stress/caller

# 编译
cd example/call_stress/server && go build -o server main.go
cd ../provider && go build -o provider *.go
cd ../caller && go build -o caller *.go
```

### 2. 运行

需要按顺序启动三个组件:

**终端 1 - 启动 Server:**
```bash
cd example/call_stress/server
./server
```

**终端 2 - 启动 Provider:**
```bash
cd example/call_stress/provider
./provider
```

**终端 3 - 启动 Caller:**
```bash
cd example/call_stress/caller
./caller
```

### 3. 停止测试

在任意终端按 `Ctrl+C` 停止测试。Caller 会打印最终统计信息。

## 统计指标说明

### 5分钟统计指标

每 5 分钟会打印以下指标:

- **发送**: 本周期内发送的请求数
- **接收**: 本周期内成功接收的响应数
- **错误**: 本周期内的错误数
- **吞吐量**: 每秒请求数(req/s)
- **延迟(微秒)**: min=最小延迟 max=最大延迟 avg=平均延迟
- **成功率**: 成功响应百分比
- **goroutine**: 当前 goroutine 数量
- **内存(堆)**: 堆内存使用量(MB)
- **内存(系统)**: 系统内存分配量(MB)
- **GC次数**: 累计 GC 次数
- **GC暂停(总)**: 累计 GC 暂停时间(ms)
- **累计发送/接收/错误**: 从开始到现在的累计值

### 最终统计指标

程序退出时会打印最终统计,包含上述所有指标的累计值。

## 性能优化

### 对象池

使用 `sync.Pool` 复用请求对象,减少内存分配和 GC 压力:

```go
var reqPool = sync.Pool{
    New: func() any {
        return &StressTestReq{
            Payload: make([]byte, 1024),
        }
    },
}
```

### 多并发

根据 CPU 核心数启动对应数量的 worker:

```go
numCPU := runtime.NumCPU()
runtime.GOMAXPROCS(numCPU)
```

### 原子操作

使用 `atomic.Int64` 进行无锁并发计数:

```go
var stats = &StressStats{
    TotalSent: atomic.Int64{},
    TotalReceived: atomic.Int64{},
    // ...
}
```

## pprof 使用

### 查看 CPU 性能分析

```bash
# 在 Server 运行 30 秒
go tool pprof http://localhost:6061/debug/pprof/profile?seconds=30

# 或在 Caller 运行
go tool pprof http://localhost:6062/debug/pprof/profile?seconds=30
```

### 查看内存使用

```bash
# 堆内存
go tool pprof http://localhost:6061/debug/pprof/heap

# 分配内存
go tool pprof http://localhost:6061/debug/pprof/allocs
```

### 查看 goroutine

```bash
go tool pprof http://localhost:6061/debug/pprof/goroutine
```

### 实时查看

```bash
# 实时查看 top
go tool pprof -top http://localhost:6061/debug/pprof/profile

# 生成火焰图
go tool pprof -http=:8080 http://localhost:6061/debug/pprof/profile?seconds=30
```

## 自定义配置

### 修改请求负载大小

在 `caller/types.go` 中修改:

```go
Payload: make([]byte, 1024), // 修改为其他大小,如 2048, 4096 等
```

然后重新生成代码并编译:
```bash
go generate ./example/call_stress/caller
cd example/call_stress/caller && go build -o caller *.go
```

### 修改统计报告间隔

在 `caller/main.go` 中修改:

```go
ticker := time.NewTicker(5 * time.Minute) // 修改为其他间隔,如 1 * time.Minute
```

### 修改 Server 端口

在 `server/main.go` 中修改:

```go
s.Listen(":9092") // 修改为其他端口
```

同时在 `provider/main.go` 和 `caller/main.go` 中修改对应的服务端地址:

```go
cli, err := crpc.Dial(ctx, "xxx", ":9092", ...)
```

## 注意事项

1. **资源占用**: 此压力测试会跑满 CPU,请确保在测试环境中运行
2. **启动顺序**: 必须先启动 Server,然后启动 Provider,最后启动 Caller
3. **网络延迟**: 如果所有组件在同一台机器上,延迟会很低
4. **内存使用**: 长时间运行可能会占用较多内存,建议定期监控
5. **GC 影响**: Go 的 GC 会影响延迟统计,建议在长时间运行后观察平均延迟
6. **文件描述符**: 高并发可能耗尽文件描述符,可能需要调整系统限制:

```bash
ulimit -n 65535
```

## 故障排查

### 连接失败

检查:
1. Server 是否已启动
2. 端口是否被占用
3. secret 是否匹配("call_stress_secret_123456")
4. 启动顺序是否正确(Server -> Provider -> Caller)

### 性能不达标

检查:
1. CPU 核心数是否正确设置
2. 是否有其他进程占用 CPU
3. 网络带宽是否充足
4. 使用 pprof 分析性能瓶颈

### Caller 找不到服务

检查:
1. Provider 是否已成功连接并注册服务
2. Caller 是否等待足够长时间(默认2秒)
3. Server 转发是否正常

### 内存持续增长

检查:
1. 是否有 goroutine 泄漏(使用 pprof goroutine 查看)
2. 对象池是否正常工作
3. 是否有未释放的资源

## 示例输出

```
=== Call 压力测试 - Server 启动 ===
pprof HTTP server started addr=localhost:6061
CPU 核心数 cores=8
Server 已启动,监听端口 :9092
```

```
=== Call 压力测试 - Provider 启动 ===
CPU 核心数 cores=8
Provider 已连接到 Server 并注册服务
```

```
=== Call 压力测试 - Caller 启动 ===
pprof HTTP server started addr=localhost:6062
CPU 核心数 cores=8
Caller 已连接到 Server
等待 Provider 注册服务...
开始压力测试
所有 worker 已启动 workers=8

=== 5分钟统计 ===
发送 1250000 接收 1249987 错误 13 吞吐量 4166 req/s
延迟(微秒) min=45 max=1234 avg=234
成功率 99.99% goroutine 152
内存(堆) 45.2MB 内存(系统) 68.3MB
GC次数 23 GC暂停(总) 12.45ms
累计发送 1250000 累计接收 1249987 累计错误 13
```
