// Package netpath refuses to infer a peer address from Xray server endpoints.
// An Xray node address is not automatically the other computer's ichuansu API.
package netpath

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/henankongming/ichuansu/internal/config"
)

var (
	ErrPeerEndpointUnknown   = errors.New("无法从现有 Xray 配置唯一确定远端 ichuansu 应用端点")
	ErrListenEndpointUnknown = errors.New("无法从现有 Xray 配置唯一确定本机 ichuansu 服务监听地址")
)

func ResolveLocalSOCKS(programDir string) (config.SOCKSInbound, error) {
	return config.UniqueSOCKSInbound(filepath.Join(programDir, "i.json"))
}

func ResolvePeerEndpoint(programDir string) (string, error) {
	if _, _, err := config.Load(filepath.Join(programDir, "i.json")); err != nil {
		return "", fmt.Errorf("本机 i.json 不可用：%w", err)
	}
	return "", ErrPeerEndpointUnknown
}

func ResolveListenEndpoint(programDir string) (string, error) {
	if _, _, err := config.Load(filepath.Join(programDir, "i.json")); err != nil {
		return "", fmt.Errorf("本机 i.json 不可用：%w", err)
	}
	return "", ErrListenEndpointUnknown
}

func ExplainBlocker() string {
	return "需要在两台电脑实测并确定 ichuansu 服务应使用的对端 IP:port、" +
		"本机 Xray SOCKS 入站和实际路由关系。Xray 节点地址不是对端电脑地址；" +
		"程序不会猜测端点、修改 i.json 或回退到公网直连。"
}
