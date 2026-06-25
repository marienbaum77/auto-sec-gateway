package checker

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

func CheckHTTPS(host string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}	
	resp, err := client.Get("https://" + host)
	if err != nil {
		return 0, fmt.Errorf("https check failed: %w", err)
	}
	defer resp.Body.Close()
	return time.Since(start), nil
}