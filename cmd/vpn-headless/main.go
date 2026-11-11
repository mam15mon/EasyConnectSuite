// vpn-headless runs one Suite engine without a graphical session.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/vpncore/configs"
	"EasyConnectSuite/vpncore/log"
)

type runningEngine interface{ Stop() }
type starter func(engine.Options) (runningEngine, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, func(opts engine.Options) (runningEngine, error) {
		return engine.Start(opts)
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, input io.Reader, output, diagnostic io.Writer, start starter) error {
	flags := flag.NewFlagSet("vpn-headless", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	protocol := flags.String("protocol", "easyconnect", "协议：easyconnect、univpn 或 topsec")
	server := flags.String("server", "", "网关主机名或 IP")
	port := flags.Int("port", 443, "网关端口")
	username := flags.String("username", "", "账号")
	domain := flags.String("domain", "", "UniVPN 认证域")
	socks := flags.Int("socks-port", 1080, "本地 SOCKS5 端口")
	httpPort := flags.Int("http-port", 1081, "本地 HTTP 代理端口")
	proxyAll := flags.Bool("proxy-all", false, "将代理请求路由至 VPN（仍遵守网关资源限制）")
	totp := flags.Bool("totp-stdin", false, "从标准输入的第二行读取 TOTP 密钥")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("不接受位置参数")
	}
	if *protocol != "easyconnect" && *protocol != "univpn" && *protocol != "topsec" {
		return errors.New("不支持的 VPN 协议")
	}
	if *server == "" || *username == "" || *port < 1 || *port > 65535 {
		return errors.New("服务器、账号或端口无效")
	}
	if *socks < 1 || *socks > 65535 || *httpPort < 1 || *httpPort > 65535 || *socks == *httpPort {
		return errors.New("代理端口无效或重复")
	}
	if *totp && *protocol != "easyconnect" {
		return errors.New("此协议暂不支持 TOTP")
	}
	if file, ok := input.(*os.File); ok {
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("检查标准输入失败: %w", err)
		}
		if info.Mode()&os.ModeCharDevice != 0 {
			return errors.New("请通过管道或受限权限的文件提供密码，避免终端回显")
		}
	}
	type credentials struct {
		password, secret string
		err              error
	}
	credentialResult := make(chan credentials, 1)
	go func() {
		reader := bufio.NewReader(io.LimitReader(input, 8196))
		password, err := readSecret(reader)
		if err != nil {
			credentialResult <- credentials{err: fmt.Errorf("读取密码失败: %w", err)}
			return
		}
		secret := ""
		if *totp {
			secret, err = readSecret(reader)
			if err != nil {
				credentialResult <- credentials{err: fmt.Errorf("读取 TOTP 密钥失败: %w", err)}
				return
			}
		}
		credentialResult <- credentials{password: password, secret: secret}
	}()
	var values credentials
	select {
	case <-ctx.Done():
		return nil
	case values = <-credentialResult:
	}
	if values.err != nil {
		return values.err
	}
	if ctx.Err() != nil {
		return nil
	}
	conf := configs.Default()
	conf.Protocol, conf.ServerAddress, conf.ServerPort = *protocol, *server, *port
	conf.Username, conf.Password, conf.TOTPSecret = *username, values.password, values.secret
	conf.SocksBind = net.JoinHostPort("127.0.0.1", strconv.Itoa(*socks))
	conf.HTTPBind = net.JoinHostPort("127.0.0.1", strconv.Itoa(*httpPort))
	conf.AutoDetectInterface, conf.DisableZJUConfig = true, true
	conf.ProxyAll = *proxyAll
	if *protocol == "univpn" {
		conf.DisableRemoteDNS, conf.DisableKeepAlive = true, true
	}
	if *protocol == "topsec" {
		conf.DisableKeepAlive = true
	}
	failed := make(chan struct{}, 1)
	var outputMu sync.Mutex
	opts := engine.Options{ConnectionID: "headless", Config: conf, Context: ctx, Domain: *domain,
		OnStatus: func(status engine.Status) {
			op := log.BeginOperation("[VPN profile=\"headless\"] ", "headless_status_output", ctx.Done())
			op.Step("wait_output_lock")
			outputMu.Lock()
			op.Step("write_output")
			_, writeErr := fmt.Fprintf(output, "状态: %s\n", status.Phase)
			outputMu.Unlock()
			outcome := "written"
			if writeErr != nil {
				outcome = "failed"
			}
			op.Finish(outcome, writeErr)
			if status.Phase == engine.PhaseError {
				select {
				case failed <- struct{}{}:
				default:
				}
			}
		},
	}
	session, err := start(opts)
	if err != nil {
		return err
	}
	defer session.Stop()
	select {
	case <-ctx.Done():
		return nil
	case <-failed:
		return errors.New("VPN 连接终止，详见诊断日志")
	}
}

func readSecret(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if len(line) == 0 || len(line) > 4096 {
		return "", errors.New("输入为空或过长")
	}
	return line, nil
}
