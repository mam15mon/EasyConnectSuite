package univpn

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAuthenticationStatusesRemainDistinct(t *testing.T) {
	for _, code := range []uint32{1, 9, 0x01020304, 0xffffffff} {
		t.Run(fmt.Sprintf("%08X", code), func(t *testing.T) {
			body := make([]byte, 8)
			binary.BigEndian.PutUint32(body[:4], code)
			token, err := parseToken(body)
			var rejection *AuthenticationError
			if !errors.Is(err, ErrLoginRejected) || !errors.As(err, &rejection) || rejection.Code != code || token != [32]byte{} {
				t.Fatal("authentication code/classification lost", err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d / 0x%08X", code, code)) || strings.Contains(err.Error(), "拒绝账号或密码") {
				t.Fatal("authentication result misreported", err)
			}
		})
	}
	for _, malformed := range [][]byte{nil, make([]byte, 7), make([]byte, 8)} {
		if _, err := parseToken(malformed); !errors.Is(err, errProtocol) || errors.Is(err, ErrLoginRejected) {
			t.Fatal("malformed response classified as rejection", err)
		}
	}
	expected := bytes.Repeat([]byte{0x5a}, 32)
	body, err := appendTLV(make([]byte, 8), 3, expected)
	if err != nil {
		t.Fatal(err)
	}
	token, err := parseToken(body)
	if err != nil || !bytes.Equal(token[:], expected) {
		t.Fatal("valid success response changed", err)
	}
}
