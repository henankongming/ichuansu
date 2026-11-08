package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const maxConfigBytes = 10 << 20 // 10 MiB is far larger than a normal Xray JSON config.

// Report contains only non-secret local environment information. It deliberately
// omits outbound server addresses, user IDs, credentials, private keys and domains.
type Report struct {
	Platform            string
	ProgramDir          string
	XrayExecutablePath  string
	XrayExecutableFound bool
	ConfigPath          string
	ConfigFound         bool
	ConfigValid         bool
	ConfigProblem       string
	Inbounds            []InboundSummary
	OutboundProtocols   []string
	RoutingRuleCount    int
	XrayProcessCount    int
	XrayProcessKnown    bool
	XrayProcessProblem  string
}

// InboundSummary exposes only protocol and an IP/localhost bind endpoint.
type InboundSummary struct {
	Protocol string
	Listen   string
	Port     string
}

// ExecutableDir returns the directory containing the currently running binary.
// It intentionally does not use the caller's working directory.
func ExecutableDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve executable path")
	}
	return filepath.Dir(absolute), nil
}

// Collect inspects only the expected Xray executable and i.json beside ichuansu.
// It never searches the disk and never writes to either file.
func Collect(programDir string) Report {
	report := Report{
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		ProgramDir: programDir,
	}

	xrayName := "xray"
	if runtime.GOOS == "windows" {
		xrayName = "xray.exe"
	}
	report.XrayExecutablePath = filepath.Join(programDir, xrayName)
	if info, err := os.Stat(report.XrayExecutablePath); err == nil && info.Mode().IsRegular() {
		report.XrayExecutableFound = true
	}

	report.ConfigPath = filepath.Join(programDir, "i.json")
	if _, err := os.Stat(report.ConfigPath); err == nil {
		report.ConfigFound = true
		config, problem := inspectConfig(report.ConfigPath)
		if problem != "" {
			report.ConfigProblem = problem
		} else {
			report.ConfigValid = true
			report.Inbounds = config.Inbounds
			report.OutboundProtocols = config.OutboundProtocols
			report.RoutingRuleCount = config.RoutingRuleCount
		}
	} else if !os.IsNotExist(err) {
		report.ConfigProblem = "无法读取配置文件元数据"
	}

	count, err := countXrayProcesses()
	if err == nil {
		report.XrayProcessKnown = true
		report.XrayProcessCount = count
	} else {
		report.XrayProcessProblem = "当前系统无法可靠检查 Xray 进程"
	}

	return report
}

// LocalChecksPassed reports whether the expected local files and a running Xray
// process were detected. It says nothing about remote reachability.
func (r Report) LocalChecksPassed() bool {
	return r.XrayExecutableFound && r.ConfigFound && r.ConfigValid && r.XrayProcessKnown && r.XrayProcessCount > 0
}

func Print(w io.Writer, r Report) {
	fmt.Fprintln(w, "[ichuansu] 阶段 1：本机环境诊断（只读）")
	fmt.Fprintf(w, "平台：%s\n", r.Platform)
	fmt.Fprintf(w, "程序目录：%s\n", r.ProgramDir)

	if r.XrayExecutableFound {
		fmt.Fprintf(w, "Xray 程序：已找到（%s）\n", r.XrayExecutablePath)
	} else {
		fmt.Fprintf(w, "Xray 程序：未在程序目录找到（预期文件：%s）\n", r.XrayExecutablePath)
	}

	switch {
	case r.ConfigValid:
		fmt.Fprintf(w, "i.json：存在且 JSON 格式有效（%s）\n", r.ConfigPath)
		fmt.Fprintf(w, "Xray 入站：%d\n", len(r.Inbounds))
		for i, inbound := range r.Inbounds {
			endpoint := "未指定"
			if inbound.Listen != "" && inbound.Port != "" {
				endpoint = net.JoinHostPort(inbound.Listen, inbound.Port)
			} else if inbound.Listen != "" {
				endpoint = inbound.Listen
			}
			fmt.Fprintf(w, "  - inbound %d：protocol=%s listen=%s\n", i+1, displayOrUnknown(inbound.Protocol), endpoint)
		}
		fmt.Fprintf(w, "Xray 出站：%d（协议：%s）\n", len(r.OutboundProtocols), joinOrNone(r.OutboundProtocols))
		fmt.Fprintf(w, "路由规则：%d 条（未输出域名、IP 或规则内容）\n", r.RoutingRuleCount)
	case r.ConfigFound:
		fmt.Fprintf(w, "i.json：存在，但无法确认是有效 JSON（%s）\n", r.ConfigPath)
		if r.ConfigProblem != "" {
			fmt.Fprintf(w, "配置诊断：%s\n", r.ConfigProblem)
		}
	default:
		fmt.Fprintf(w, "i.json：程序目录中未找到（预期位置：%s）\n", r.ConfigPath)
	}

	if r.XrayProcessKnown {
		fmt.Fprintf(w, "Xray 进程：检测到 %d 个\n", r.XrayProcessCount)
	} else {
		fmt.Fprintf(w, "Xray 进程：无法判断（%s）\n", displayOrUnknown(r.XrayProcessProblem))
	}

	fmt.Fprintln(w, "网络路径：未验证。必须在两台真实电脑上分别测试双向应用连通性，并确认流量经过现有 Xray 路径。")
	fmt.Fprintln(w, "安全：未输出 UUID、密码、私钥、服务器地址或完整配置；i.json 仅以只读方式读取。")
	if r.LocalChecksPassed() {
		fmt.Fprintln(w, "本机检查：通过（不代表对端可达）。")
	} else {
		fmt.Fprintln(w, "本机检查：未通过或信息不完整；请按上述提示检查 Xray 程序、i.json 和运行状态。")
	}
}

type configSummary struct {
	Inbounds          []InboundSummary
	OutboundProtocols []string
	RoutingRuleCount  int
}

func inspectConfig(path string) (configSummary, string) {
	info, err := os.Stat(path)
	if err != nil {
		return configSummary{}, "无法读取配置文件"
	}
	if !info.Mode().IsRegular() {
		return configSummary{}, "配置路径不是普通文件"
	}
	if info.Size() > maxConfigBytes {
		return configSummary{}, "配置文件超过 10 MiB 安全读取上限"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return configSummary{}, "无法读取配置文件（请检查权限）"
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		// Do not include the parser's raw message because it may echo source content.
		return configSummary{}, fmt.Sprintf("JSON 格式无效（错误位置约在字节 %d）", jsonErrorOffset(err))
	}

	summary := configSummary{Inbounds: make([]InboundSummary, 0)}
	if values, ok := root["inbounds"].([]any); ok {
		for _, value := range values {
			item, ok := value.(map[string]any)
			if !ok {
				continue
			}
			inbound := InboundSummary{
				Protocol: safeToken(asString(item["protocol"])),
				Listen:   safeListen(asString(item["listen"])),
				Port:     safePort(item["port"]),
			}
			summary.Inbounds = append(summary.Inbounds, inbound)
		}
	}
	if values, ok := root["outbounds"].([]any); ok {
		for _, value := range values {
			item, ok := value.(map[string]any)
			if !ok {
				continue
			}
			summary.OutboundProtocols = append(summary.OutboundProtocols, safeToken(asString(item["protocol"])))
		}
	}
	if routing, ok := root["routing"].(map[string]any); ok {
		if rules, ok := routing["rules"].([]any); ok {
			summary.RoutingRuleCount = len(rules)
		}
	}
	return summary, ""
}

func jsonErrorOffset(err error) int64 {
	switch value := err.(type) {
	case *json.SyntaxError:
		return value.Offset
	case *json.UnmarshalTypeError:
		return value.Offset
	default:
		return 0
	}
}

func asString(value any) string {
	text, _ := value.(string)
	return text
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
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}

func safePort(value any) string {
	var port int64
	switch number := value.(type) {
	case float64:
		port = int64(number)
	case json.Number:
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil {
			return ""
		}
		port = parsed
	default:
		return ""
	}
	if port < 1 || port > 65535 {
		return ""
	}
	return strconv.FormatInt(port, 10)
}

func displayOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "未知"
	}
	return value
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "无或未识别"
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		clean = append(clean, displayOrUnknown(value))
	}
	return strings.Join(clean, ", ")
}
