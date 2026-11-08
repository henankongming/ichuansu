package client

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/henankongming/ichuansu/internal/server"
	"github.com/henankongming/ichuansu/internal/transfer"
)

var (
	ErrRemoteConflict       = errors.New("对端程序目录已存在同名文件")
	ErrRemoteSourceNotFound = errors.New("远端源路径不存在")
	ErrRemotePermission     = errors.New("远端源路径没有读取权限")
	ErrRemoteAuthentication = errors.New("对端应用认证失败")
	ErrRemoteConnection     = errors.New("对端文件服务连接失败")
)

type ContextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type TransferClient struct {
	baseURL string
	token   []byte
	http    *http.Client
}

func New(baseURL string, token []byte, xray ContextDialer) (*TransferClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return nil, fmt.Errorf("对端应用端点格式无效；必须是已经验证的 HTTP host:port")
	}
	if _, port, splitErr := net.SplitHostPort(parsed.Host); splitErr != nil || port == "" {
		return nil, fmt.Errorf("对端应用端点必须显式包含端口")
	}
	if len(token) != 32 {
		return nil, fmt.Errorf("应用认证密钥无效")
	}
	transport := &http.Transport{
		DialContext:           xray.DialContext,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          2,
		DisableCompression:    true,
	}
	return &TransferClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   append([]byte(nil), token...),
		http: &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *TransferClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+server.HealthPath, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w：%v", ErrRemoteConnection, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("对端 ichuansu 健康检查失败：HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *TransferClient) IGet(ctx context.Context, remotePath, destinationDir string, progress func(int64, int64)) (string, error) {
	query := url.Values{}
	query.Set("path", remotePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+server.DownloadPath+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	c.authorize(req)
	response, err := c.http.Do(req)
	if err != nil {
		return "", ErrRemoteConnection
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusNotFound:
			return "", ErrRemoteSourceNotFound
		case http.StatusForbidden:
			return "", ErrRemotePermission
		case http.StatusUnauthorized:
			return "", ErrRemoteAuthentication
		case http.StatusBadRequest:
			return "", fmt.Errorf("远端源路径非法或不受支持")
		default:
			return "", fmt.Errorf("远端文件服务返回 HTTP %d", response.StatusCode)
		}
	}
	name, err := transfer.SafeFilename(response.Header.Get("X-Ichuansu-Name"))
	if err != nil {
		return "", fmt.Errorf("远端返回了非法文件名")
	}
	size, err := strconv.ParseInt(response.Header.Get("X-Ichuansu-Size"), 10, 64)
	if err != nil || size < 0 || response.ContentLength != size {
		return "", fmt.Errorf("远端文件长度元数据无效")
	}
	sha := strings.ToLower(response.Header.Get("X-Ichuansu-SHA256"))
	if len(sha) != 64 {
		return "", fmt.Errorf("远端 SHA-256 元数据无效")
	}
	if _, err := hex.DecodeString(sha); err != nil {
		return "", fmt.Errorf("远端 SHA-256 元数据无效")
	}
	return transfer.SaveAtomic(destinationDir, name, response.Body, size, sha, progress)
}

func (c *TransferClient) IPut(ctx context.Context, sourcePath string, progress func(int64, int64)) (string, error) {
	prepared, err := transfer.PrepareSource(sourcePath)
	if err != nil {
		return "", err
	}
	defer prepared.Cleanup()
	return c.IPutPrepared(ctx, prepared, progress)
}

func (c *TransferClient) IPutPrepared(ctx context.Context, prepared *transfer.PreparedSource, progress func(int64, int64)) (string, error) {
	if prepared == nil {
		return "", fmt.Errorf("本地源文件未准备")
	}
	file, err := prepared.Open()
	if err != nil {
		return "", fmt.Errorf("无法打开本地源文件")
	}
	defer file.Close()
	query := url.Values{}
	query.Set("name", prepared.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+server.UploadPath+"?"+query.Encode(), &progressReader{reader: file, total: prepared.Size, callback: progress})
	if err != nil {
		return "", err
	}
	req.ContentLength = prepared.Size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Ichuansu-SHA256", prepared.SHA256)
	c.authorize(req)
	response, err := c.http.Do(req)
	if err != nil {
		return "", ErrRemoteConnection
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		return filepath.Base(prepared.Name), nil
	case http.StatusConflict:
		return "", ErrRemoteConflict
	case http.StatusUnauthorized:
		return "", ErrRemoteAuthentication
	case http.StatusUnprocessableEntity:
		return "", transfer.ErrChecksum
	default:
		return "", fmt.Errorf("远端文件服务返回 HTTP %d", response.StatusCode)
	}
}

func (c *TransferClient) CloseIdleConnections() {
	c.http.CloseIdleConnections()
}

func (c *TransferClient) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+hex.EncodeToString(c.token))
}

type progressReader struct {
	reader   io.Reader
	total    int64
	written  int64
	callback func(int64, int64)
}

func (p *progressReader) Read(buffer []byte) (int, error) {
	n, err := p.reader.Read(buffer)
	p.written += int64(n)
	if n > 0 && p.callback != nil {
		p.callback(p.written, p.total)
	}
	return n, err
}

// LocalTCPDialer is useful for tests only. Production code must use socks5.Dialer.
type LocalTCPDialer struct{ Timeout time.Duration }

func (d LocalTCPDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: d.Timeout}).DialContext(ctx, network, address)
}
