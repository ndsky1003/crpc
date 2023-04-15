# crpc
中心服务的rpc，采用注册机制

#### 数据类型支持
1. coder.JSON 
> 	number,string,bool,slice, map *point 测试通过

2. coder.MsgPack
> number,string,bool,slice, map *point 测试通过


>自定义解码器的时候优先要测试需要支持的类型


#### 数据压缩
> Raw,Snappy 测试通过


#### usage

server.go
```go
	crpc.NewServer().Listen(":8080")
```

client1.go
```go
client1 := crpc.Dial("client1", "127.0.0.1:8080", options.Client().SetIsStopHeart(true))
time.Sleep(2e9)                          //保证其链接上，正式使用，不需要
client1.RegisterName("rpc", new(Person)) //注册服务
fmt.Println("start")
var s []*Data
//number,string,bool,slice, map *point
if err := client1.Call("client1", "rpc.GetName", map[string]*Data{"0": {Name: "dd", Age: 18}, "3": {Name: "dd1", Age: 80}}, &s); err != nil {
  fmt.Println(err)
}
time.Sleep(1e9)
fmt.Printf("done:result:%+v", s)
```

client2.go

```go
client2 := crpc.Dial("client2", "127.0.0.1:8080", options.Client().SetIsStopHeart(true))
time.Sleep(2e9)                          //保证其链接上，正式使用，不需要
var s []*Data
if err := client2.Call("client1", "rpc.GetName", map[string]*Data{"0": {Name: "dd", Age: 18}, "3": {Name: "dd1", Age: 80}}, &s); err != nil {
  fmt.Println(err)
}
time.Sleep(1e9)
fmt.Printf("done:result:%+v", s)
```


#### 文件发送
>维护好chunksize就可以支持断点续传
client1.go
```go
	client := crpc.Dial("client", "127.0.0.1:8080", options.Client().SetIsStopHeart(true).SetChunksMaxSize(50*1024*1024))
	time.Sleep(1e9)
	f, err := os.Open("ccc/鲸落.mp4")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := client.SendFile("client1", "rpc.SaveFile", "img/鲸落.mp4", f); err != nil {
		logrus.Error(err)
	}
	logrus.Info("done")
```
client2.go
```go
func main() {
	client1 := crpc.Dial("client1", "127.0.0.1:8080", options.Client().SetIsStopHeart(true))
	client1.RegisterName("rpc", new(o))
	time.Sleep(1e9)
	select {}
}
type o struct {
}
func (*o) SaveFile(req dto.FileBody, _ *int) error {
	f, err := comm.GetWriteFile(req.ChunksIndex, req.Filename)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(req.Data)
	if err != nil {
		return err
	}
	return nil
}
```

#### Benchmark
```bash
➜  crpc git:(main) ✗ go test -v -run ^$ -bench Call$ -benchmem
ERRO[0000]/Users/mac/go/workSpace/self-pkg/crpc/client.go:108 github.com/ndsky1003/crpc.(*Client).keepAlive() dail err:dial tcp 127.0.0.1:8081: connect: connection refused
INFO[0001]/Users/mac/go/workSpace/self-pkg/crpc/server.go:104 github.com/ndsky1003/crpc.(*server).addService() add service:client
goos: darwin
goarch: amd64
pkg: github.com/ndsky1003/crpc
cpu: 12th Gen Intel(R) Core(TM) i5-12400
Benchmark_Call
INFO[0002]/Users/mac/go/workSpace/self-pkg/crpc/server.go:104 github.com/ndsky1003/crpc.(*server).addService() add service:client2
Benchmark_Call/1
Benchmark_Call/1-12  	1000000000	         0.0001096 ns/op	       0 B/op	       0 allocs/op
Benchmark_Call/2
Benchmark_Call/2-12  	1000000000	         0.0001179 ns/op	       0 B/op	       0 allocs/op
Benchmark_Call/3
Benchmark_Call/3-12  	1000000000	         0.0000847 ns/op	       0 B/op	       0 allocs/op
Benchmark_Call/4
Benchmark_Call/4-12  	1000000000	         0.0000895 ns/op	       0 B/op	       0 allocs/op
Benchmark_Call/5
Benchmark_Call/5-12  	1000000000	         0.0000724 ns/op	       0 B/op	       0 allocs/op
Benchmark_Call/6
Benchmark_Call/6-12  	1000000000	         0.0000890 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/ndsky1003/crpc	3.025s
```

