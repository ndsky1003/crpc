# crpc
中心服务的rpc，采用注册机制

#### 数据类型支持
1. coder.JSON 
> 	number,string,bool,slice, map *point 测试通过

2. coder.MsgPack
> 未测试


>自定义解码器的时候优先要测试需要支持的类型


#### 数据压缩
> 暂未测试


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

