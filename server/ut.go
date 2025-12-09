package server

import "fmt"

// 辅助生成 Key
func getBroadcastKey(clientID string, seq uint64) string {
	return fmt.Sprintf("%s:%d", clientID, seq)
}
