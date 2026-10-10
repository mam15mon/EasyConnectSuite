package easyconnect

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordLoginMarksExplicitRejection(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		reply    string
		rejected bool
		wantErr  bool
	}{
		{"wrong password", "<Auth><Result>0</Result><Message>用户名或密码错误</Message></Auth>", true, true},
		{"success", "<Auth><Result>1</Result></Auth>", false, false},
		{"unexpected response", "<html>service unavailable</html>", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/por/login_auth.csp":
					fmt.Fprintf(w, "<Auth><TwfID>session</TwfID><RSA_ENCRYPT_KEY>%s</RSA_ENCRYPT_KEY><RSA_ENCRYPT_EXP>65537</RSA_ENCRYPT_EXP><RndImg>0</RndImg></Auth>", key.N.Text(16))
				case "/por/login_psw.csp":
					fmt.Fprint(w, test.reply)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			c := NewClient(Options{Server: strings.TrimPrefix(server.URL, "https://"), Auth: AuthOptions{Username: "test", Password: "test"}})
			c.httpClient = server.Client()
			defer c.Close()
			err := c.requestTwfID("")
			if (err != nil) != test.wantErr || errors.Is(err, ErrLoginRejected) != test.rejected {
				t.Fatalf("login error = %v, wantErr=%v rejected=%v", err, test.wantErr, test.rejected)
			}
		})
	}
}
