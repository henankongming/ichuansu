// Package config reads Xray JSON configuration without modifying it and
// exposes only the small, non-secret subset needed for local diagnostics.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const MaxConfigBytes int64 = 10 << 20

var (
	ErrNoSOCKSInbound       = errors.New("no explicit local SOCKS inbound was found in i.json")
	ErrAmbiguousSOCKS       = errors.New("multiple local SOCKS inbounds were found; refusing to guess which one to use")
	ErrUnsupportedSOCKSAuth = errors.New("the SOCKS inbound authentication settings are not supported safely")
)

type rawInbound struct {
	Protocol string          `json:"protocol"`
	Listen   string          `json:"listen"`
	Port     json.RawMessage `json:"port"`
	Tag      string          `json:"tag"`
	Settings json.RawMessage `json:"settings"`
}

type rawOutbound struct {
	Protocol string `json:"protocol"`
}

type rawConfig struct {
	Inbounds  []rawInbound  `json:"inbounds"`
	Outbounds []rawOutbound `json:"outbounds"`
	Routing   struct {
		Rules []json.RawMessage `json:"rules"`
	} `json:"routing"`
}

type InboundSummary struct {
	Protocol string
	Listen   string
	Port     string
}

type Summary struct {
	Inbounds          []InboundSummary
	OutboundProtocols []string
	RoutingRuleCount  int
}

// SOCKSInbound contains credentials only in memory. Never print or serialize it.
type SOCKSInbound struct {
	Address  string
	Username string
	Password string
}

func Load(path string) (Summary, []SOCKSInbound, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Summary{}, nil, fmt.Errorf("无法读取 i.json 元数据")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Summary{}, nil, fmt.Errorf("i.json 路径不是普通文件")
	}
	if info.Size() > MaxConfigBytes {
		return Summary{}, nil, fmt.Errorf("i.json 超过 10 MiB 安全读取上限")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, nil, fmt.Errorf("无法读取 i.json（请检查权限）")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw rawConfig
	if err := decoder.Decode(&raw); err != nil {
		return Summary{}, nil, fmt.Errorf("i.json 不是有效的 Xray JSON 配置（错误位置约在字节 %d）", syntaxOffset(err))
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Summary{}, nil, fmt.Errorf("i.json 包含 JSON 对象之后的额外内容")
	}

	summary := Summary{
		Inbounds:          make([]InboundSummary, 0, len(raw.Inbounds)),
		OutboundProtocols: make([]string, 0, len(raw.Outbounds)),
		RoutingRuleCount:  len(raw.Routing.Rules),
	}
	var socks []SOCKSInbound
	for _, inbound := range raw.Inbounds {
		protocol := safeToken(inbound.Protocol)
		listen := safeListen(inbound.Listen)
		port := safePort(inbound.Port)
		summary.Inbounds = append(summary.Inbounds, InboundSummary{Protocol: protocol, Listen: listen, Port: port})
		if strings.EqualFold(strings.TrimSpace(inbound.Protocol), "socks") {
			if listen == "" || port == "" {
				continue // Xray defaults are intentionally not guessed.
			}
			creds, authErr := parseSOCKSCredentials(inbound.Settings)
			if authErr != nil {
				return Summary{}, nil, authErr
			}
			socks = append(socks, SOCKSInbound{
				Address:  net.JoinHostPort(listen, port),
				Username: creds.Username,
				Password: creds.Password,
			})
		}
	}
	for _, outbound := range raw.Outbounds {
		summary.OutboundProtocols = append(summary.OutboundProtocols, safeToken(outbound.Protocol))
	}
	return summary, socks, nil
}

func UniqueSOCKSInbound(path string) (SOCKSInbound, error) {
	_, found, err := Load(path)
	if err != nil {
		return SOCKSInbound{}, err
	}
	switch len(found) {
	case 0:
		return SOCKSInbound{}, ErrNoSOCKSInbound
	case 1:
		return found[0], nil
	default:
		return SOCKSInbound{}, ErrAmbiguousSOCKS
	}
}

func parseSOCKSCredentials(raw json.RawMessage) (SOCKSInbound, error) {
	var settings struct {
		Auth     string `json:"auth"`
		Accounts []struct {
			User string `json:"user"`
			Pass string `json:"pass"`
		} `json:"accounts"`
	}
	if len(raw) == 0 {
		return SOCKSInbound{}, nil // Xray SOCKS default is no authentication.
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return SOCKSInbound{}, ErrUnsupportedSOCKSAuth
	}
	switch strings.ToLower(strings.TrimSpace(settings.Auth)) {
	case "", "noauth":
		return SOCKSInbound{}, nil
	case "password":
		if len(settings.Accounts) != 1 || settings.Accounts[0].User == "" || settings.Accounts[0].Pass == "" {
			return SOCKSInbound{}, ErrUnsupportedSOCKSAuth
		}
		if len(settings.Accounts[0].User) > 255 || len(settings.Accounts[0].Pass) > 255 {
			return SOCKSInbound{}, ErrUnsupportedSOCKSAuth
		}
		return SOCKSInbound{Username: settings.Accounts[0].User, Password: settings.Accounts[0].Pass}, nil
	default:
		return SOCKSInbound{}, ErrUnsupportedSOCKSAuth
	}
}

func syntaxOffset(err error) int64 {
	switch typed := err.(type) {
	case *json.SyntaxError:
		return typed.Offset
	case *json.UnmarshalTypeError:
		return typed.Offset
	default:
		return 0
	}
}

func safeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "未知"
	}
	if len(value) > 32 {
		return "已隐藏"
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "已隐藏"
		}
	}
	return value
}

func safeListen(value string) string {
	value = strings.TrimSpace(value)
	if value == "localhost" {
		return value
	}
	ip := net.ParseIP(value)
	if ip == nil || !ip.IsLoopback() {
		return "" // The local Xray proxy endpoint must not be an exposed network listener.
	}
	return ip.String()
}

func safePort(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return ""
	}
	port, err := strconv.Atoi(number.String())
	if err != nil || port < 1 || port > 65535 {
		return ""
	}
	return strconv.Itoa(port)
}

// IsXrayConfigPath returns true only for the expected i.json beside the app.
func IsXrayConfigPath(programDir string) string {
	return filepath.Join(programDir, "i.json")
}
