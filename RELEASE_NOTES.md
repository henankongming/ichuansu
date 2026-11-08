## ichuansu v0.1.0-alpha（预览版）

这是首个可下载的 Go 原生程序预览包，提供 Windows 和 Linux 可执行构建及本机 Xray 配置诊断入口。

### 下载资产

- Windows x86-64：`ichuansu-v0.1.0-alpha-windows-amd64.zip`
- Windows ARM64：`ichuansu-v0.1.0-alpha-windows-arm64.zip`
- Linux x86-64：`ichuansu-v0.1.0-alpha-linux-amd64.tar.gz`
- Linux ARM64：`ichuansu-v0.1.0-alpha-linux-arm64.tar.gz`
- `SHA256SUMS.txt`：所有安装包的 SHA-256 校验值

Windows 包包含 `ichuansu.cmd` 和内部 `_ichuansu.exe`；Linux 包包含原生 `ichuansu` 可执行文件。普通用户不需要安装 Go 运行环境。

### 当前可用功能

- `ichuansu --help`
- `ichuansu --version`
- `ichuansu diagnose`（本机只读诊断，敏感配置字段脱敏）

### 重要：文件传输尚未开放

**本版本不是已完成验收的双机文件传输 MVP。** 两台真实电脑之间的 Xray 路由、服务监听地址和远端 ichuansu 应用端点还没有实测确认。为避免连接错目标或绕过既定 Xray 通道，`serve`、`iget`、`iput` 会 fail-closed 并报告缺少的端点条件。

本版本不会修改 `i.json`，不会把 Xray 节点地址擅自当成对端电脑地址，不会新增 VPS/中转，也不会在 Xray 路径不通时回退到公网直连。

下一阶段必须在 A、B 两台真实机器上完成双向连通性与文件哈希验收，之后才能发布宣称文件传输可用的稳定版。
