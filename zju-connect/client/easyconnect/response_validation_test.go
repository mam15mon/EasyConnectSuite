package easyconnect

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"EasyConnectSuite/vpncore/client/authchallenge"
	zlog "EasyConnectSuite/vpncore/log"
)

func TestAuthenticationFailuresDoNotExposePayloads(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "private-canary-do-not-log"
	for _, stage := range []string{"auth", "password", "nextauth", "sms-request", "sms-code", "totp", "totp-missing-id", "cert", "keepalive"} {
		t.Run(stage, func(t *testing.T) {
			var logs bytes.Buffer
			previous := stdlog.Writer()
			stdlog.SetOutput(&logs)
			defer stdlog.SetOutput(previous)
			debug := zlog.DebugEnabled()
			zlog.EnableDebug()
			defer func() {
				if !debug {
					zlog.DisableDebug()
				}
			}()
			reply := "<Auth><Result>0</Result><Message>" + secret + "</Message><ErrorCode>42</ErrorCode></Auth>"
			var cert []byte
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/com/server.crt":
					w.Write(cert)
				case "/por/login_auth.csp":
					if stage == "auth" {
						io.WriteString(w, "<html>"+secret+"</html>")
						return
					}
					fmt.Fprintf(w, "<Auth><TwfID>%s</TwfID><RSA_ENCRYPT_KEY>%s</RSA_ENCRYPT_KEY><RndImg>0</RndImg></Auth>", secret, key.N.Text(16))
				case "/por/login_sms.csp":
					if stage == "sms-code" {
						io.WriteString(w, "<USER_PHONE>"+secret+"</USER_PHONE>")
						return
					}
					io.WriteString(w, reply)
				case "/por/login_token.csp":
					if stage == "totp-missing-id" {
						io.WriteString(w, "Totp auth succ "+secret)
						return
					}
					io.WriteString(w, reply)
				default:
					if stage == "nextauth" {
						io.WriteString(w, "<NextAuth>99</NextAuth>"+secret)
						return
					}
					io.WriteString(w, reply)
				}
			}))
			defer server.Close()
			cert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
			c := NewClient(Options{Server: strings.TrimPrefix(server.URL, "https://"), Auth: AuthOptions{Password: secret}, ChallengeHandler: authchallenge.HandlerFuncs{Code: func(authchallenge.CodeChallenge) (authchallenge.CodeResponse, error) {
				return authchallenge.CodeResponse{Code: secret}, nil
			}}})
			defer c.Close()
			c.twfID = secret
			var failure error
			switch stage {
			case "auth", "password", "nextauth":
				failure = c.loginAuthAndPsw("")
			case "sms-request", "sms-code":
				failure = c.loginSMS()
			case "totp", "totp-missing-id":
				failure = c.loginTOTP()
			case "cert":
				failure = c.loginCert()
			case "keepalive":
				failure = c.requestUpdateSession(context.Background())
			}
			if failure == nil {
				t.Fatal("invalid response accepted")
			}
			if strings.Contains(failure.Error(), secret) || strings.Contains(logs.String(), secret) {
				t.Fatal("private payload leaked in error or debug log")
			}
		})
	}
}

func TestLoginAuthValidatesRequiredFields(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf("<Auth><TwfID>session</TwfID><RSA_ENCRYPT_KEY>%s</RSA_ENCRYPT_KEY><RSA_ENCRYPT_EXP>65537</RSA_ENCRYPT_EXP><RndImg>0</RndImg></Auth>", key.N.Text(16))
	for _, field := range []string{"TwfID", "RSA_ENCRYPT_KEY", "RndImg", "RSA_ENCRYPT_EXP"} {
		t.Run(field, func(t *testing.T) {
			value, err := requiredAuthField([]byte(valid), field)
			if err != nil {
				t.Fatal(err)
			}
			bad := strings.Replace(valid, "<"+field+">"+value+"</"+field+">", "<"+field+">invalid!</"+field+">", 1)
			if field == "TwfID" {
				bad = strings.Replace(bad, "<TwfID>invalid!</TwfID>", "", 1)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/por/login_auth.csp" {
					t.Error("invalid auth progressed to password request")
				}
				io.WriteString(w, bad)
			}))
			defer server.Close()
			c := NewClient(Options{Server: strings.TrimPrefix(server.URL, "https://")})
			defer c.Close()
			if err := c.loginAuthAndPsw(""); err == nil {
				t.Fatal("malformed field accepted")
			}
		})
	}
}

func TestResourceParsingRejectsMissingStructure(t *testing.T) {
	for _, body := range []string{"<html/>", "<Resource/>", `<Resource><Rcs><Rc/></Rcs></Resource>`, `<Resource><Rcs><Rc type="1"/></Rcs></Resource>`, `<Resource><Rcs/></Resource>`, `<Resource><Rcs/><Dns/></Resource>`, `<Resource><Rcs/><Dns data=""/></Resource>`, `<Resource><Rcs/><Dns data="" dnsserver=""/></Resource>`} {
		c := NewClient(Options{})
		c.dnsServer = "10.0.0.1"
		if err := c.parseResources(body); err == nil {
			t.Errorf("invalid resource accepted: %s", body)
		}
		c.Close()
	}
	c := NewClient(Options{})
	defer c.Close()
	if err := c.parseResources(`<Resource><Rcs><Rc type="1" proto="0" host="10.0.0.1" port="443~443"/></Rcs><Dns data="" dnsserver="10.0.0.53"/></Resource>`); err != nil {
		t.Fatal(err)
	}
	if len(c.ipResources) != 1 || c.dnsServer != "10.0.0.53" {
		t.Fatal("valid resource compatibility lost")
	}
}
