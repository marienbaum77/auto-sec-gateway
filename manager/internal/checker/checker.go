package checker

import (
	"net"
	"time"
)

// CheckPort проверяет доступность порта
func CheckPort(address string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return time.Since(start), nil
}
