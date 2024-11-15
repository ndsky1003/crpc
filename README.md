# crpc
中心服务的rpc，采用注册机制

#### 数据类型支持
1. coder.JSON
> 	number,string,bool,slice, map *point 测试通过

2. coder.MsgPack
> number,string,bool,slice, map *point 测试通过

3. coder.Msgp
> 当没有代码自动生成的时候(尚未实现其协议的)=>自动使用2.coder.MsgPack序列化

> 这里就需要注意其兼容性了,struct,只有*struct才是实现了其协议的,否则均会降级使用2.coder.MscPack
mark: struct 使用2序列化.将字节码又通过3来反解就会有些不兼容

> number,string,bool,slice, map *point 测试通过 map只支持string作为key




##### 自定义解码器的时候优先要测试需要支持的类型


#### 数据压缩
> Raw,Snappy 测试通过


#### usage

server.go
```go
	crpc.NewServer().Listen(":8080")
```

client1.go
```go
client1 := crpc.Dial("client1", "127.0.0.1:8080", crpc.Options().SetHeartInterval(-1))
time.Sleep(2e9)                          //保证其链接上，正式使用，不需要
//one call
var r1 string
err = client.Call("client2", "func.Hello1", p, &r1, crpc.Options().SetReqCoderT(coder.Msgp).SetResCoderT(coder.Msgp))
```

client2.go

```go
client = crpc.Dial("client2", "127.0.0.1:8080", crpc.Options().SetHeartInterval(-1))
//Mark: 注册方法
client2.RegisterName("crpc", new(msg))
client.RegisterFunc("Hello", Hello)
client.RegisterFunc("Hello1", Hello1)
client.RegisterFunc("Hello2", Hello2)
client.RegisterFunc("Hello3", Hello3)

```
#### 方法类型支持
```go
func (m *msg) Hello(req dto.Person) error {
	return nil
}

func (m *msg) Hello1(req dto.Person) (string, error) {
	return "World", nil
}

func (m *msg) Hello3(meta dto.Meta, req *dto.Person) error {
	return nil
}

func (m *msg) Hello2(meta dto.Meta, req *dto.Person) (*dto.Person, error) {
	return &dto.Person{Name: "World"}, nil
}

//同时支持函数
func Hello(req dto.Person) error {
	return nil
}

func Hello1(req dto.Person) (string, error) {
	return "World", nil
}

func Hello3(meta dto.Meta, req *dto.Person) error {
	return nil
}

func Hello2(meta dto.Meta, req *dto.Person) (*dto.Person, error) {
	return &dto.Person{Name: "World"}, nil
}

```


#### 文件发送
>维护好chunksize就可以支持断点续传
client1.go
```go
	f, err := os.Open("./cc.mp4")
	if err != nil {
		panic(err)
	}
	now := time.Now()
	if err := client.SendFile("client2", "crpc.SaveFile", "./ccc/ccc.mp5", f); err != nil {
		fmt.Println(err)
	}
```
client2.go
```go
//文件保存
func (m *msg) SaveFile(req *cdto.FileBody) error {
	return crpc.WriteFile(req)
}
```

#### Benchmark

