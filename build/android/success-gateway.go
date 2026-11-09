//go:build ignore

// Success gateway for Android integration tests. It uses only synthetic data.
package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

var udpReplies = make(chan []byte, 8)

func main() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	config := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS11, CipherSuites: []uint16{tls.TLS_RSA_WITH_RC4_128_SHA}}
	listener, err := tls.Listen("tcp", "127.0.0.1:18445", config)
	if err != nil {
		panic(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go serve(conn, key)
	}
}

func serve(conn net.Conn, key *rsa.PrivateKey) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	first, err := reader.Peek(1)
	if err != nil {
		return
	}
	if first[0] == 0 || first[0] == 5 || first[0] == 6 {
		header := make([]byte, 64)
		if _, err = io.ReadFull(reader, header); err != nil {
			return
		}
		reply := []byte{0, 0, 0, 0, 10, 90, 0, 2}
		switch header[0] {
		case 5:
			reply[0] = 2
		case 6:
			reply[0] = 1
		}
		if _, err = conn.Write(reply); err != nil {
			return
		}
		fmt.Fprintf(os.Stdout, "handshake=%d\n", header[0])
		switch header[0] {
		case 5:
			for {
				ip := make([]byte, 20)
				if _, err := io.ReadFull(reader, ip); err != nil {
					return
				}
				length := int(binary.BigEndian.Uint16(ip[2:4]))
				if ip[0] != 0x45 || ip[9] != 17 || length < 28 || length > 1500 {
					return
				}
				packet := make([]byte, length)
				copy(packet, ip)
				if _, err := io.ReadFull(reader, packet[20:]); err != nil {
					return
				}
				if string(packet[28:]) != "ecs-udp-test" {
					continue
				}
				copy(packet[12:16], ip[16:20])
				copy(packet[16:20], ip[12:16])
				sourcePort := append([]byte(nil), packet[20:22]...)
				copy(packet[20:22], packet[22:24])
				copy(packet[22:24], sourcePort)
				packet[10], packet[11], packet[26], packet[27] = 0, 0, 0, 0
				var sum uint32
				for i := 0; i < 20; i += 2 {
					sum += uint32(binary.BigEndian.Uint16(packet[i : i+2]))
				}
				for sum>>16 != 0 {
					sum = sum&0xffff + sum>>16
				}
				binary.BigEndian.PutUint16(packet[10:12], ^uint16(sum))
				udpReplies <- packet
				fmt.Fprintln(os.Stdout, "udp_echo")
			}
		case 6:
			for packet := range udpReplies {
				if _, err := conn.Write(packet); err != nil {
					return
				}
			}
		default:
			_, _ = io.Copy(io.Discard, reader)
		}
		return
	}
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	defer request.Body.Close()
	_, _ = io.Copy(io.Discard, request.Body)
	body := "<Conf><Mline enable=\"0\" list=\"\"/></Conf>"
	switch request.URL.Path {
	case "/por/login_auth.csp":
		body = fmt.Sprintf("<Auth><TwfID>0000000000000001</TwfID><RSA_ENCRYPT_KEY>%s</RSA_ENCRYPT_KEY><RSA_ENCRYPT_EXP>65537</RSA_ENCRYPT_EXP><RndImg>0</RndImg></Auth>", key.N.Text(16))
	case "/por/login_psw.csp":
		body = "<Auth><Result>1</Result></Auth>"
	case "/por/update_session.csp":
		body = "<Auth><Message>success</Message><ErrorCode>1</ErrorCode></Auth>"
	case "/por/conf.csp":
	default:
		fmt.Fprintf(os.Stderr, "unexpected HTTP path: %s\n", request.URL.Path)
		return
	}
	response := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Close: true, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
	_ = response.Write(conn)
}
