//go:build ignore

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

func main() {
	conn, err := net.DialTimeout("tcp4", "10.90.0.3:12345", 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	message := []byte("ecs-tcp-test")
	if _, err := conn.Write(message); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	response := make([]byte, len(message))
	if _, err := io.ReadFull(conn, response); err != nil || string(response) != string(message) {
		fmt.Fprintf(os.Stderr, "TCP echo failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("TCP payload returned through Android VPN")
}
