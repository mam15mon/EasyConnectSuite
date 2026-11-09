//go:build ignore

package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	conn, err := net.DialTimeout("udp4", "10.90.0.3:12345", 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("ecs-udp-test")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if string(buf[:n]) != "ecs-udp-test" {
		fmt.Fprintln(os.Stderr, "unexpected UDP response")
		os.Exit(1)
	}
	fmt.Println("UDP payload returned through Android VPN")
}
