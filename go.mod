module github.com/ndsky1003/crpc/v3

go 1.24

require (
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/ndsky1003/net v0.0.0-20251205095351-a205d45c878f
)

require (
	github.com/golang/snappy v1.0.0
	github.com/google/uuid v1.6.0
	github.com/tinylib/msgp v1.6.1
	github.com/vmihailenco/msgpack/v5 v5.4.1
)

require (
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
)

replace github.com/ndsky1003/net => ../net
