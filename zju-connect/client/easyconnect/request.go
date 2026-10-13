package easyconnect

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"EasyConnectSuite/vpncore/client/authchallenge"
	"EasyConnectSuite/vpncore/log"
	"github.com/pquerna/otp/totp"
	utls "github.com/refraction-networking/utls"
)

var errSMSRequired = errors.New("SMS code required")
var errTOTPRequired = errors.New("TOTP required")
var errCertRequired = errors.New("cert required")
var errNotFound = errors.New("not found")

// ErrLoginRejected marks an explicit rejection from the password login endpoint.
var ErrLoginRejected = errors.New("Login failed")

func (c *Client) requestTwfID(graphCodeFile string) error {
	err := c.loginAuthAndPsw(graphCodeFile)
	if err != nil {
		if errors.Is(err, errSMSRequired) {
			err = c.loginSMS()
			if err != nil {
				return err
			}
		} else if errors.Is(err, errTOTPRequired) {
			err = c.loginTOTP()
			if err != nil {
				return err
			}
		} else if errors.Is(err, errCertRequired) {
			err = c.loginCert()
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func (c *Client) loginAuthAndPsw(graphCodeFile string) error {
	// First we request the TwfID from server
	addr := "https://" + c.server + "/por/login_auth.csp?apiversion=1"
	log.Printf("Request: %s", addr)

	req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodGet, addr, nil)
	if err != nil {
		return err
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	c.twfID, err = requiredAuthField(buf.Bytes(), "TwfID")
	if err != nil {
		return err
	}
	log.Println("Authentication session received")
	rsaKey, err := requiredAuthField(buf.Bytes(), "RSA_ENCRYPT_KEY")
	if err != nil {
		return err
	}

	rsaExpMatch := regexp.MustCompile(`<RSA_ENCRYPT_EXP>(.*)</RSA_ENCRYPT_EXP>`).FindSubmatch(buf.Bytes())
	rsaExp := ""
	if rsaExpMatch != nil {
		rsaExp = string(rsaExpMatch[1])
	} else {
		log.Printf("Warning: No RSA_ENCRYPT_EXP, using default")
		rsaExp = "65537"
	}

	csrfMatch := regexp.MustCompile(`<CSRF_RAND_CODE>(.*)</CSRF_RAND_CODE>`).FindSubmatch(buf.Bytes())
	csrfCode := ""
	password := c.password
	if csrfMatch != nil {
		csrfCode = string(csrfMatch[1])
		password += "_" + csrfCode
	} else {
		log.Printf("Warning: No CSRF rand code")
	}

	pubKey := rsa.PublicKey{}
	pubKey.E, err = strconv.Atoi(rsaExp)
	if err != nil || pubKey.E < 3 || pubKey.E > 1<<31-1 || pubKey.E%2 == 0 {
		return errors.New("login_auth: invalid RSA exponent")
	}
	modulus := big.Int{}
	if _, ok := modulus.SetString(rsaKey, 16); !ok || modulus.Sign() <= 0 {
		return errors.New("login_auth: invalid RSA modulus")
	}
	pubKey.N = &modulus

	encryptedPassword, err := rsa.EncryptPKCS1v15(rand.Reader, &pubKey, []byte(password))
	if err != nil {
		return err
	}
	encryptedPasswordHex := hex.EncodeToString(encryptedPassword)

	rndImg, err := requiredAuthField(buf.Bytes(), "RndImg")
	if err != nil {
		return err
	}
	if rndImg != "0" && rndImg != "1" {
		return errors.New("login_auth: invalid RndImg")
	}

	randCode := ""
	if rndImg == "1" {
		addr = "https://" + c.server + "/por/rand_code.csp?apiversion=1"
		log.Printf("Request: %s", addr)
		req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodGet, addr, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Cookie", "TWFID="+c.twfID)
		req.Header.Set("User-Agent", "EasyConnect_windows")

		resp, err = c.doRequest(req)
		if err != nil {
			return err
		}
		captchaImage, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		challenge := authchallenge.TextCaptchaChallenge{
			Image:      captchaImage,
			OutputPath: graphCodeFile,
			Message:    "Please enter rand code:",
		}
		captchaResponse, err := c.challengeHandler.HandleTextCaptcha(challenge)
		if err != nil {
			return fmt.Errorf("complete EasyConnect captcha challenge: %w", err)
		}
		if err := challenge.Validate(captchaResponse); err != nil {
			return fmt.Errorf("invalid EasyConnect captcha response: %w", err)
		}
		randCode = captchaResponse.Code
	}

	addr = "https://" + c.server + "/por/login_psw.csp?anti_replay=1&encrypt=1&type=cs"
	log.Printf("Request: %s", addr)

	form := url.Values{
		"svpn_rand_code":    {randCode},
		"mitm":              {""},
		"svpn_req_randcode": {csrfCode},
		"svpn_name":         {c.username},
		"svpn_password":     {encryptedPasswordHex},
	}

	req, err = http.NewRequestWithContext(c.lifecycleCtx, http.MethodPost, addr, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_windows")

	resp, err = c.doRequest(req)
	if err != nil {
		return err
	}

	buf.Reset()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	if strings.Contains(buf.String(), "<NextService>auth/sms</NextService>") || strings.Contains(buf.String(), "<NextAuth>2</NextAuth>") {
		log.Print("SMS code required")

		return errSMSRequired
	}

	if strings.Contains(buf.String(), "<NextService>auth/token</NextService>") || strings.Contains(buf.String(), "<NextAuth>7</NextAuth>") {
		log.Print("TOTP required")

		return errTOTPRequired
	}

	if strings.Contains(buf.String(), "<NextAuth>0</NextAuth>") {
		log.Print("Cert required")

		return errCertRequired
	}

	if strings.Contains(buf.String(), "<NextAuth>-1</NextAuth>") || !strings.Contains(buf.String(), "<NextAuth>") {
		log.Print("No NextAuth found")
	} else {
		return authReplyError("unsupported authentication method", buf.Bytes())
	}

	if !strings.Contains(buf.String(), "<Result>1</Result>") {
		if strings.Contains(buf.String(), "<Result>0</Result>") {
			return fmt.Errorf("%w: %v", ErrLoginRejected, authReplyError("password rejected", buf.Bytes()))
		}
		return authReplyError("password login failed", buf.Bytes())
	}

	twfIDMatch := regexp.MustCompile(`<TwfID>(.*)</TwfID>`).FindSubmatch(buf.Bytes())
	if twfIDMatch != nil {
		c.twfID = string(twfIDMatch[1])
		log.Println("Authentication session updated")
	}

	log.Printf("TWFID has been authorized")

	return nil
}

func (c *Client) loginSMS() error {
	addr := "https://" + c.server + "/por/login_sms.csp?apiversion=1"
	log.Printf("SMS request: %s", addr)
	req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodPost, addr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_windows")

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Reset()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(buf.String(), "验证码已发送到您的手机") && !strings.Contains(buf.String(), "请输入验证码：") && !strings.Contains(buf.String(), "<USER_PHONE>") {
		return authReplyError("SMS request failed", buf.Bytes())
	}

	log.Printf("SMS code is sent or still valid")

	codeResponse, err := c.challengeHandler.HandleCodeChallenge(authchallenge.CodeChallenge{
		Kind:    authchallenge.CodeSMS,
		Message: "Please enter your SMS code:",
	})
	if err != nil {
		return fmt.Errorf("complete EasyConnect SMS challenge: %w", err)
	}
	addr = "https://" + c.server + "/por/login_sms1.csp?apiversion=1"
	log.Printf("SMS Request: %s", addr)
	form := url.Values{
		"svpn_inputsms": {codeResponse.Code},
	}

	req, err = http.NewRequestWithContext(c.lifecycleCtx, http.MethodPost, addr, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_windows")

	resp, err = c.doRequest(req)
	if err != nil {
		return err
	}

	buf.Reset()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(buf.String(), "Auth sms suc") && !strings.Contains(buf.String(), "欢迎访问") {
		return authReplyError("SMS verification failed", buf.Bytes())
	}

	twfIDMatch := regexp.MustCompile(`<TwfID>(.*)</TwfID>`).FindSubmatch(buf.Bytes())
	if twfIDMatch != nil {
		c.twfID = string(twfIDMatch[1])
		log.Println("Authentication session updated")
	}
	log.Print("SMS code verification success")

	return nil
}

func (c *Client) loginTOTP() error {
	var totpCode string
	var err error
	if c.totpSecret == "" {
		challenge := authchallenge.CodeChallenge{
			Kind:    authchallenge.CodeTOTP,
			Message: "Please enter your TOTP code:",
		}
		var response authchallenge.CodeResponse
		response, err = c.challengeHandler.HandleCodeChallenge(challenge)
		totpCode = response.Code
	} else {
		totpCode, err = totp.GenerateCode(c.totpSecret, time.Now())
		log.Println("Generated TOTP code")
	}
	if err != nil {
		return err
	}

	addr := "https://" + c.server + "/por/login_token.csp"
	log.Printf("TOTP Request: %s", addr)
	form := url.Values{
		"svpn_inputtoken": {totpCode},
	}
	req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodPost, addr, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_windows")

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Reset()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(buf.String(), "Totp auth succ") {
		return authReplyError("TOTP verification failed", buf.Bytes())
	}

	c.twfID, err = requiredAuthField(buf.Bytes(), "TwfID")
	if err != nil {
		return err
	}
	log.Print("TOTP verification success")

	return nil
}

func (c *Client) loginCert() error {
	addr := "https://" + c.server + "/com/server.crt"
	log.Printf("Get server cert: %s", addr)
	req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodPost, addr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_windows")

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Reset()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(buf.Bytes())
	if !ok {
		return errors.New("failed to parse server certificate")
	}

	c.setHTTPTransport(&tls.Config{
		InsecureSkipVerify: true,
		Renegotiation:      tls.RenegotiateOnceAsClient,
		Certificates:       []tls.Certificate{c.tlsCert},
		RootCAs:            caCertPool,
	})

	addr = "https://" + c.server + "/por/login_cert.csp?anti_replay=1&encrypt=1&type=cs"
	log.Printf("Cert Request: %s", addr)
	req, err = http.NewRequestWithContext(c.lifecycleCtx, http.MethodPost, addr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_windows")

	resp, err = c.doRequest(req)
	if err != nil {
		return err
	}

	buf.Reset()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}

	response := buf.String()
	certSuccess := strings.Contains(response, "<Result>1</Result>") ||
		strings.Contains(response, "Login successfully") ||
		strings.Contains(response, "Auth cert suc")
	if !certSuccess {
		return authReplyError("certificate verification failed", buf.Bytes())
	}

	twfIDMatch := regexp.MustCompile(`<TwfID>(.*)</TwfID>`).FindSubmatch(buf.Bytes())
	if twfIDMatch != nil {
		c.twfID = string(twfIDMatch[1])
		log.Println("Authentication session updated")
	}

	if strings.Contains(response, "<pwpErrorCode>16</pwpErrorCode>") {
		log.Print("Cert verification success (server returned pwpErrorCode=16, profile redirect ignored)")
	} else {
		log.Print("Cert verification success")
	}

	return nil
}

func (c *Client) requestConfig() (string, error) {
	addr := "https://" + c.server + "/por/conf.csp"
	log.Printf("Request: %s", addr)

	req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodGet, addr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)

	resp, err := c.doRequest(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// requestUpdateSession pings /por/update_session.csp to keep the server-side
// session alive. The official EasyConnect client calls this periodically;
// without it, sangfor servers with strict idle policies (e.g. HUST) close
// the session, which the tunnel layer surfaces as a "broken pipe" →
// "unexpected handshake reply" kick cascade.
func (c *Client) requestUpdateSession(ctx context.Context) error {
	// Keepalive must not observe a partially updated authentication session or
	// race with certificate authentication replacing the HTTP transport.
	c.setupMu.Lock()
	defer c.setupMu.Unlock()
	u := url.URL{
		Scheme: "https",
		Host:   c.server,
		Path:   "/por/update_session.csp",
	}
	q := url.Values{}
	q.Set("twfid", c.twfID)
	q.Set("apiversion", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)
	req.Header.Set("User-Agent", "EasyConnect_Linux_Ubuntu")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return safeRequestError(err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return errNotFound
		}
		return fmt.Errorf("update_session: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Successful body looks like:
	//   <Auth><Message>success</Message><ErrorCode>1</ErrorCode><TwfID>...</TwfID></Auth>
	var reply struct {
		Message   string `xml:"Message"`
		ErrorCode string `xml:"ErrorCode"`
	}
	if err := xml.Unmarshal(body, &reply); err != nil {
		return errors.New("update_session: invalid XML reply")
	}
	if reply.Message != "success" || reply.ErrorCode != "1" {
		return authReplyError("update_session rejected", body)
	}
	return nil
}

func (c *Client) requestResources() (string, error) {
	addr := "https://" + c.server + "/por/rclist.csp"
	log.Printf("Request: %s", addr)

	req, err := http.NewRequestWithContext(c.lifecycleCtx, http.MethodGet, addr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", "TWFID="+c.twfID)

	resp, err := c.doRequest(req)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (c *Client) requestToken() error {
	ctx, cancel := c.rawRequestContext()
	defer cancel()
	dialConn, err := c.dialContext(ctx, "tcp", c.server)
	if err != nil {
		return err
	}
	defer func(dialConn net.Conn) {
		_ = dialConn.Close()
	}(dialConn)
	conn := utls.UClient(dialConn, &utls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       c.tlsKeyLogWriter,
	}, utls.HelloGolang)
	defer func(conn *utls.UConn) {
		_ = conn.Close()
	}(conn)
	clearDeadline, err := armConnectionContext(ctx, conn)
	if err != nil {
		return err
	}
	defer clearDeadline()
	if err := conn.HandshakeContext(ctx); err != nil {
		return err
	}

	// When establish an HTTPS connection to server and send a valid request with TWFID to it
	// The **TLS ServerHello SessionId** is the first part of token
	log.Printf("ECAgent request: /por/conf.csp & /por/rclist.csp")
	_, err = io.WriteString(
		conn,
		"GET /por/conf.csp HTTP/1.1\r\nHost: "+c.server+
			"\r\nCookie: TWFID="+c.twfID+
			"\r\n\r\nGET /por/rclist.csp HTTP/1.1\r\nHost: "+c.server+
			"\r\nCookie: TWFID="+c.twfID+"\r\n\r\n",
	)
	if err != nil {
		return err
	}

	if conn.HandshakeState.ServerHello == nil {
		return errors.New("tunnel token: missing ServerHello")
	}
	sessionID := hex.EncodeToString(conn.HandshakeState.ServerHello.SessionId)
	if len(sessionID) < 31 || len(c.twfID) != 16 {
		return errors.New("tunnel token: invalid session ID length")
	}

	buf := make([]byte, 8)
	n, err := conn.Read(buf)
	if n == 0 || err != nil {
		return fmt.Errorf("ECAgent request invalid: read %d bytes: %w", n, err)
	}

	c.token = (*[48]byte)([]byte(sessionID[:31] + "\x00" + c.twfID))

	log.Println("Tunnel token received")

	return nil
}

func (c *Client) requestIP() error {
	ctx, cancel := c.rawRequestContext()
	defer cancel()
	conn, err := c.tlsConn(ctx)
	if err != nil {
		return err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = conn.Close()
		}
	}()
	clearDeadline, err := armConnectionContext(ctx, conn)
	if err != nil {
		return err
	}
	defer clearDeadline()

	// Request IP Packet
	message := []byte{0x00, 0x00, 0x00, 0x00}
	message = append(message, c.token[:]...)
	message = append(message, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff}...)

	n, err := conn.Write(message)
	if err != nil {
		return err
	}

	log.DebugPrintf("Request IP: wrote %d bytes", n)

	reply := make([]byte, 0x80)
	n, err = io.ReadAtLeast(conn, reply, 8)
	if err != nil {
		return err
	}

	log.DebugPrintf("Request IP: read %d bytes", n)

	if reply[0] != 0x00 {
		return errors.New("unexpected request IP reply")
	}

	ip := net.IP(reply[4:8])
	if ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return errors.New("request IP: invalid IPv4 address")
	}
	if c.updateIP != nil {
		if err := c.updateIP(append(net.IP(nil), ip...)); err != nil {
			return fmt.Errorf("update tunnel IP: %w", err)
		}
	}
	c.ip = ip
	c.ipReverse = []byte{c.ip[3], c.ip[2], c.ip[1], c.ip[0]}
	if c.underlayDialer != nil {
		c.underlayDialer.ExcludeIP(c.ip)
	}

	log.Printf("Client IP: %s", c.ip.String())

	// Request IP conn CAN NOT be closed, otherwise tx/rx handshake will fail
	c.setRequestIPConn(conn)
	succeeded = true

	return nil
}

func (c *Client) setRequestIPConn(conn net.Conn) {
	c.requestIPConnMu.Lock()
	if c.requestIPConn != nil {
		_ = c.requestIPConn.Close()
	}
	c.requestIPConn = conn
	c.requestIPConnMu.Unlock()

	c.requestIPKeepAlive.Do(func() {
		go c.requestIPKeepAliveLoop()
	})
}

func (c *Client) requestIPKeepAliveLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.lifecycleCtx.Done():
			return
		case <-ticker.C:
			c.requestIPConnMu.Lock()
			conn := c.requestIPConn
			c.requestIPConnMu.Unlock()
			runtime.KeepAlive(conn)
		}
	}
}

// Authentication payloads may contain credentials even on failure. Only trusted
// stage names and bounded numeric protocol codes may leave this boundary.
func authReplyError(stage string, body []byte) error {
	code := regexp.MustCompile(`<(?:ErrorCode|pwpErrorCode)>(-?[0-9]{1,10})</(?:ErrorCode|pwpErrorCode)>`).FindSubmatch(body)
	if code != nil {
		return fmt.Errorf("%s: code=%s", stage, code[1])
	}
	return errors.New(stage + ": invalid or rejected response")
}

func requiredAuthField(body []byte, name string) (string, error) {
	match := regexp.MustCompile(`<` + name + `>([^<]+)</` + name + `>`).FindSubmatch(body)
	if match == nil {
		return "", fmt.Errorf("authentication: missing or empty %s", name)
	}
	return string(match[1]), nil
}

func safeRequestError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (c *Client) doRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, safeRequestError(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: unexpected HTTP status %d", req.URL.Path, resp.StatusCode)
	}
	return resp, nil
}
