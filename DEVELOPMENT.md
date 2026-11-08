# 开发状态与验收说明

项目需求基线仍是 README.md。此文件记录实现状态与外部验证边界。

## 已实现的 Go 核心

- 单一 Go 命令入口、`--help`、`--version`、`diagnose`。
- 通过可执行文件路径确定程序目录，不依赖 shell 当前目录。
- 只读解析程序目录内的 `i.json`，不扫描磁盘、不写回配置。
- 诊断仅显示入站协议/显式本地监听、出站协议名称、路由规则数量；不输出节点地址、域名、UUID、密码、私钥或完整配置。
- 识别唯一、显式配置的本地 SOCKS 入站（不猜 Xray 默认地址/端口）；可在内存中读取单一 SOCKS 用户名/密码供本地代理握手使用，但绝不打印。
- Linux `/proc` 与 Windows `tasklist` 本机 Xray 进程检测。
- HTTP 文件 API：健康检查、远端下载、上传；应用层 Bearer 密钥认证。
- 流式传输、SHA-256、长度核验、`.part` 临时文件、原子 no-override 安诅。
普通文件直接传输；目录生成 ZIP，保留顶层目录，跳过符号链接，不自动解压。
- Windows CMD 启动器。
- SOCKS5 CONNECT取用使用服务地址 XRay SOCKS inbound使用，所有 http 请希置回作绝，并用运你图的线下渠道复制；不要把密钥贴到聊天或日志）。

未完成上述验证前，MVP 的网络验收状态为 **BLOCKED / NOT VERIFIED**。没有自动创建 VPS、中继节点、代理配置，也没有修改 `i.json`。

## 本地验证与构建

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o ichuansu .
GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/ichuansu .
GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/_ichuansu.exe .
```

将 Windows `ichuansu.cmd` 与 `_ichuansu.exe` 放在同一目录。两端把程序放入 Xray 程序目录，并确保同目录存在原有 `i.json`。
