package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/henankongming/ichuansu/internal/diagnostics"
)

// Run dispatches the single ichuansu command-line application.
// It returns the process exit code instead of calling os.Exit so it can be tested.
func Run(args []string, version string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch strings.ToLower(args[0]) {
	case "--help", "-h", "help":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "[ichuansu] 帮助命令不接受额外参数。")
			return 2
		}
		printHelp(stdout)
		return 0
	case "--version", "version":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "[ichuansu] 版本命令不接受额外参数。")
			return 2
		}
		fmt.Fprintf(stdout, "ichuansu %s\n", version)
		return 0
	case "diagnose":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "[ichuansu] diagnose 不接受额外参数。")
			return 2
		}
		programDir, err := diagnostics.ExecutableDir()
		if err != nil {
			fmt.Fprintln(stderr, "[ichuansu] 无法确定程序所在目录。")
			return 3
		}
		report := diagnostics.Collect(programDir)
		diagnostics.Print(stdout, report)
		if !report.LocalChecksPassed() {
			return 3
		}
		return 0
	case "serve", "iget", "iput":
		fmt.Fprintf(stderr, "[ichuansu] 命令 %q 尚未实现；当前版本仅完成阶段 1 的项目骨架和本机诊断。\n", args[0])
		fmt.Fprintln(stderr, "[ichuansu] 请先运行 ichuansu diagnose。网络连通性验证完成前，不会启用文件传输。")
		return 1
	default:
		fmt.Fprintf(stderr, "[ichuansu] 未知命令：%s\n", args[0])
		fmt.Fprintln(stderr, "[ichuansu] 使用 ichuansu --help 查看用法。")
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "ichuansu - 跨 Windows/Linux 的命令行文件传输工具")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "用法：")
	fmt.Fprintln(w, "  ichuansu serve                         启动本机文件接收服务（阶段 2）")
	fmt.Fprintln(w, "  ichuansu iget <远端文件或文件夹路径>    下载到本机程序目录（阶段 3）")
	fmt.Fprintln(w, "  ichuansu iput <本地文件或文件夹路径>    上传到对端程序目录（阶段 3）")
	fmt.Fprintln(w, "  ichuansu diagnose                      只读检查本机 Xray 和 i.json")
	fmt.Fprintln(w, "  ichuansu --help                        显示帮助")
	fmt.Fprintln(w, "  ichuansu --version                     显示版本")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "说明：Windows 通过同目录 ichuansu.cmd 调用内部 _ichuansu.exe；Linux 使用原生 ichuansu。")
	fmt.Fprintln(w, "网络路径尚未经过两台真实电脑验证；程序不会修改 i.json，也不会绕过 Xray。")
}
