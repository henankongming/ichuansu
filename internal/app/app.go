package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/henankongming/ichuansu/internal/client"
	"github.com/henankongming/ichuansu/internal/diagnostics"
	"github.com/henankongming/ichuansu/internal/netpath"
	"github.com/henankongming/ichuansu/internal/server"
	"github.com/henankongming/ichuansu/internal/socks5"
	"github.com/henankongming/ichuansu/internal/transfer"
)

func Run(args []string, version string, stdout, stderr io.Writer) int {
	dir, err := diagnostics.ExecutableDir()
	if err != nil {
		fmt.Fprintln(stderr, "[ichuansu] 无法确定程序所在目录。")
		return 3
	}
	return RunInDir(args, version, dir, stdout, stderr)
}

func RunInDir(args []string, version, dir string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	switch strings.ToLower(args[0]) {
	case "--help", "-h", "help":
		if len(args) != 1 {
			return usageError(stderr, "帮助命令不接受额外参数。")
		}
		printHelp(stdout)
		return 0
	case "--version", "version":
		if len(args) != 1 {
			return usageError(stderr, "版本命令不接受额外参数。")
		}
		fmt.Fprintf(stdout, "ichuansu %s\n", version)
		return 0
	case "diagnose":
		if len(args) != 1 {
			return usageError(stderr, "diagnose 不接受额外参数。")
		}
		report := diagnostics.Collect(dir)
		diagnostics.Print(stdout, report)
		if !report.LocalChecksPassed() {
			return 3
		}
		return 0
	case "serve":
		if len(args) != 1 {
			return usageError(stderr, "serve 不接受额外参数。")
		}
		address, err := netpath.ResolveListenEndpoint(dir)
		if err != nil {
			fmt.Fprintln(stderr, "[ichuansu] 暂不能安全启动接收服务。")
			fmt.Fprintln(stderr, netpath.ExplainBlocker())
			fmt.Fprintf(stderr, "缺少条件：%s\n", err)
			return 3
		}
		token, created, err := server.LoadOrCreateToken(dir)
		if err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 应用认证初始化失败：%s\n", err)
			return 3
		}
		if created {
			fmt.Fprintln(stdout, "[ichuansu] 已生成 .ichuansu-token。请通过可信线下方式将同一文件部署到对端；不要复制密钥内容到聊天或日志。")
		}
		fmt.Fprintf(stdout, "[ichuansu] 正在监听 %s\n", address)
		if err := server.ListenAndServe(address, dir, token); err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 接收服务退出：%s\n", err)
			return 1
		}
		return 0
	case "iget":
		if len(args) != 2 {
			return usageError(stderr, "用法：ichuansu iget <远端文件或文件夹路径>")
		}
		peer, err := netpath.ResolvePeerEndpoint(dir)
		if err != nil {
			fmt.Fprintln(stderr, "[ichuansu] 无法确定唯一远端 ichuansu 服务端点。")
			fmt.Fprintln(stderr, netpath.ExplainBlocker())
			fmt.Fprintf(stderr, "缺少条件：%s\n", err)
			return 4
		}
		transferClient, err := newTransferClient(dir, peer)
		if err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 本地 Xray SOCKS 入站或应用认证密钥不可用：%s\n", err)
			return 3
		}
		defer transferClient.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err = transferClient.Health(ctx)
		cancel()
		if err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 对端文件服务无响应：%s\n", err)
			return 4
		}
		fmt.Fprintln(stdout, "[ichuansu] 已通过本地 Xray SOCKS 通道连接对端服务。")
		path, err := transferClient.IGet(context.Background(), args[1], dir, progress(stdout))
		if err != nil {
			switch {
			case errors.Is(err, client.ErrRemoteConnection):
				return fail(stderr, 4, "对端文件服务不可达；请检查对端服务和现有 Xray 路径。")
			case errors.Is(err, client.ErrRemoteSourceNotFound), errors.Is(err, client.ErrRemotePermission):
				return fail(stderr, 5, "远端源路径不存在或无读取权限。")
			case errors.Is(err, client.ErrRemoteAuthentication):
				return fail(stderr, 1, "应用认证失败；两端 .ichuansu-token 必须一致。")
			case errors.Is(err, transfer.ErrConflict):
				return fail(stderr, 6, "本机程序目录已存在同名文件。")
			case errors.Is(err, transfer.ErrChecksum), errors.Is(err, transfer.ErrLength):
				return fail(stderr, 7, "文件长度或 SHA-256 校验失败。")
			default:
				fmt.Fprintf(stderr, "[ichuansu] 下载失败：%s\n", err)
				return 1
			}
		}
		fmt.Fprintln(stdout, "[ichuansu] SHA-256 校验通过。")
		fmt.Fprintf(stdout, "[ichuansu] 下载完成：%s\n", path)
		return 0
	case "iput":
		if len(args) != 2 {
			return usageError(stderr, "用法：ichuansu iput <本地文件或文件夹路径>")
		}
		prepared, err := transfer.PrepareSource(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 本地源路径无效：%s\n", err)
			return 5
		}
		defer prepared.Cleanup()
		peer, err := netpath.ResolvePeerEndpoint(dir)
		if err != nil {
			fmt.Fprintln(stderr, "[ichuansu] 无法确定唯一远端 ichuansu 服务端点。")
			fmt.Fprintln(stderr, netpath.ExplainBlocker())
			fmt.Fprintf(stderr, "缺少条件：%s\n", err)
			return 4
		}
		transferClient, err := newTransferClient(dir, peer)
		if err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 本地 Xray SOCKS 入站或应用认证密钥不可用：%s\n", err)
			return 3
		}
		defer transferClient.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err = transferClient.Health(ctx)
		cancel()
		if err != nil {
			fmt.Fprintf(stderr, "[ichuansu] 对端文件服务无响应：%s\n", err)
			return 4
		}
		fmt.Fprintf(stdout, "[ichuansu] 正在上传 %s\n", prepared.Name)
		name, err := transferClient.IPutPrepared(context.Background(), prepared, progress(stdout))
		if err != nil {
			switch {
			case errors.Is(err, client.ErrRemoteConnection):
				return fail(stderr, 4, "对端文件服务不可达；请检查对端服务和现有 Xray 路径。")
			case errors.Is(err, client.ErrRemoteAuthentication):
				return fail(stderr, 1, "应用认证失败；两端 .ichuansu-token 必须一致。")
			case errors.Is(err, client.ErrRemoteConflict), errors.Is(err, transfer.ErrConflict):
				return fail(stderr, 6, "对端程序目录已存在同名文件。")
			case errors.Is(err, transfer.ErrChecksum), errors.Is(err, transfer.ErrLength):
				return fail(stderr, 7, "文件长度或 SHA-256 校验失败。")
			default:
				fmt.Fprintf(stderr, "[ichuansu] 上传失败：%s\n", err)
				return 1
			}
		}
		fmt.Fprintln(stdout, "[ichuansu] SHA-256 校验通过。")
		fmt.Fprintf(stdout, "[ichuansu] 上传完成：%s\n", name)
		return 0
	default:
		return usageError(stderr, "未知命令："+args[0])
	}
}

func newTransferClient(dir, peer string) (*client.TransferClient, error) {
	proxy, err := netpath.ResolveLocalSOCKS(dir)
	if err != nil {
		return nil, err
	}
	token, err := server.LoadToken(dir)
	if err != nil {
		return nil, err
	}
	dialer := socks5.Dialer{
		ProxyAddress: proxy.Address,
		Username: proxy.Username,
		Password: proxy.Password,
		Timeout: 10 * time.Second,
	}
	baseURL := (&url.URL{Scheme: "http", Host: peer}).String()
	return client.New(baseURL, token, &dialer)
}

func progress(w io.Writer) func(int64, int64) {
	last := int64(-1)
	return func(done, total int64) {
		pct := int64(100)
		if total > 0 {
			pct = done * 100 / total
		}
		if pct != last && (pct == 100 || pct%5 == 0) {
			fmt.Fprintf(w, "[ichuansu] 进度 %d%%（%d/%d 字节）\n", pct, done, total)
			last = pct
		}
	}
}

func usageError(w io.Writer, message string) int {
	fmt.Fprintf(w, "[ichuansu] %s\n使用 ichuansu --help 查看用法。\n", message)
	return 2
}

func fail(w io.Writer, code int, message string) int {
	fmt.Fprintf(w, "[ichuansu] %s\n", message)
	return code
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "ichuansu - 跨 Windows/Linux 的命令行文件传输工具")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "用法：")
	fmt.Fprintln(w, "  ichuansu serve                         启动本机文件接收服务")
	fmt.Fprintln(w, "  ichuansu iget <远端文件或文件夹路径>    下载到本机程序目录")
	fmt.Fprintln(w, "  ichuansu iput <本地文件或文件夹路径>    上传到对端程序目录")
	fmt.Fprintln(w, "  ichuansu diagnose                      只读检查本机 Xray 和 i.json")
	fmt.Fprintln(w, "  ichuansu --help                        显示帮助")
	fmt.Fprintln(w, "  ichuansu --version                     显示版本")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Windows 通过 ichuansu.cmd 调用内部 _ichuansu.exe；Linux 使用原生二进制。")
	fmt.Fprintln(w, "i.json 只读；不改写 Xray、不创建中转、不回退到公网直连。")
	fmt.Fprintln(w, "网络命令仅在双机实测确定唯一远端端点和本机监听地址后才能启用。")
}
