# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 重要提醒

**在协助开发此项目时，请尽量使用中文回复用户的问题和交流。**

## Project Overview

CRPC is a high-performance RPC (Remote Procedure Call) framework for Go, designed for central service communication with a registration mechanism. It provides both client and server capabilities with support for multiple serialization formats, compression algorithms, and flexible method signatures.

## Architecture

### Core Components

- **Client** (`client/`): RPC client with connection pooling, load balancing, and middleware support
- **Server** (`server/`): RPC server with service registration, broadcasting, and middleware support
- **Protocol** (`protocol/`): Binary protocol implementation with headers, error handling, and message packing
- **Coder** (`coder/`): Serialization support for JSON, MessagePack, and raw bytes
- **Compressor** (`compressor/`): Compression support including Snappy and raw (no compression)
- **Buffer** (`buffer/`): Memory pool management for network buffers
- **Examples** (`example/`): Sample implementations and usage patterns

### Key Dependencies

- `github.com/ndsky1003/net/v2`: Low-level networking layer
- `github.com/ndsky1003/buffer/v2`: Buffer management
- `github.com/golang-jwt/jwt/v5`: JWT authentication
- `github.com/tinylib/msgp`: MessagePack serialization
- `github.com/panjf2000/ants/v2`: Goroutine pool
- `github.com/google/uuid`: UUID generation

## Development Workflow

### Build Commands

```bash
# Generate code from annotations (processes //go:generate directives)
go generate ./...

# Build the module
go build ./...

# Install dependencies
go mod tidy
go mod download

# Clean build cache
go clean -cache
```

### Code Generation

The framework uses code generation for creating client and server stubs:

```bash
# Generate server code
gencrpcserverv3

# Generate client code
gencrpcclientv3 --out_dir=./output --package=client --client=ClientName --service=ServiceName --module=ModuleName
```

### Testing

Currently, no dedicated test files exist in the codebase. Tests should be added following Go conventions:
- Create `*_test.go` files alongside source files
- Use `go test ./...` to run all tests
- Use `go test -v ./...` for verbose output
- Use `go test -run TestFunctionName ./package/path` to run a specific test
- Use `go test -race ./...` to enable race detection

## Key Features

### Method Signature Support

The framework supports flexible method signatures:

```go
// Full signature
func (s *Service) Method(ctx context.Context, meta *Meta, req *Req) (*Res, error)

// Common patterns
func (s *Service) Method(ctx context.Context, req *Req) (*Res, error)
func (s *Service) Method(req *Req) (*Res, error)
func (s *Service) Method() (*Res, error)

// Return variations
func (s *Service) Method(req *Req) *Res          // Success only
func (s *Service) Method(req *Req) error         // Error only
func (s *Service) Method(req *Req)               // No return
```

### Annotations

Use annotations to control code generation:

```go
// @crpc:CallType:Call,Send,Go
// @crpc:Client: ClientName
// @crpc:Module: ModuleName
// @crpc:Service: ServiceName
// @crpc:FuncName: CustomFunctionName
// @crpc:IsSkip:true
```

### Configuration

Environment variables:
- `CRPC_SECRET`: Required authentication secret
- `CRPC_WEIGHT`: Client weight for load balancing (default: 10)
- `CRPC_BROADCAST_CAP`: Broadcast channel capacity (default: 64)
- `CRPC_JWT_EXPIRE`: JWT expiration duration (default: 5s)
- `CRPC_DEBUG`: Enable debug mode (default: false)
- `GROUP_REPLICAS`: Group replication count (default: 100)

## API Usage

### Server Setup

```go
import "github.com/ndsky1003/crpc/v3"

// Create server
server, err := crpc.NewServer(context.Background())
if err != nil {
    log.Fatal(err)
}

// Start listening
server.Listen(":8080")
```

### Client Setup

```go
import "github.com/ndsky1003/crpc/v3"

// Create client
client, err := crpc.Dial(context.Background(), "clientName", ":8080")
if err != nil {
    log.Fatal(err)
}

// Register service function
client.RegisterFunc("serviceName", handlerFunction, "functionName")
```

### Middleware

Both client and server support middleware chains for cross-cutting concerns like logging, metrics, and authentication.

## Protocol Details

- Binary protocol with magic number `0x4352` ('CR')
- Packet structure: Magic(2) + HeaderLen(2) + Header + Meta + Body
- Maximum packet size: 100MB
- Header includes metadata like sequence numbers, service names, and error codes

## Code Organization

- `api.go`: Public API exports and convenience functions
- `cmd/`: Code generation tools
- `comm/`: Common utilities
- `protocol/header/`: Header implementation with code generation
- `protocol/errors/`: Error handling with codes
- `example/`: Usage examples and patterns

## Best Practices

1. Always use `CRPC_SECRET` for authentication
2. Choose appropriate coder (JSON for debugging, MessagePack for performance)
3. Use compression for large payloads
4. Implement proper error handling with the framework's error types
5. Use context for timeout and cancellation
6. Register services before starting the server
7. Use middleware for cross-cutting concerns

## Development Tips

- When adding new services, run `go generate ./...` after defining interfaces
- The code generation tools parse annotations starting with `@crpc:`
- Check the `example/` directory for implementation patterns
- The framework uses `github.com/ndsky1003/net/v2` for all network operations
- Buffer pooling is handled automatically via `github.com/ndsky1003/buffer/v2`
- JWT tokens are used for authentication between services

## Future Considerations

- Add comprehensive test suite
- Implement connection health checks
- Add metrics and monitoring
- Consider adding gRPC compatibility
- Implement service discovery
- Add request/response interceptors