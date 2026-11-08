# 开发状态与验收说明

项目需求基线仍是 README.md。此文件记录实现状态与外部验证边界。

## 已实现的 Go 核心

- 单一 Go 命令入口：--help、--version、diagnose、serve、iget、iput。
- Windows CMD 包装器调用同目录 _ichuansu.exe 并返回退出码；Linux 使用原生 Go 二进制。
- 只读解析同目录 i.json，摘要显示入站/出站协议和路由数量；不打印节点地址、域名、UUID、密码或密钥。
- 只使用配置中明确声明的唯一 loopback SOCKS 入站；不猜端口，不把 Xray outbound 节点地址当成远端电脑。
- SOCKS5 CONNECT 拨号器支持配置指定的认证方式；生产 HTTP 客户端必须经此拨号器，不含普通 TCP 直连回退路径。
- HTTP 健康检查、文件下载和上传 API；使用 256-bit 应用 Bearer token。
- 上传/下载流式传输、进度、SHA-256、ZIP 文件夹打包、路径/文件名安全检查。
- 接收端先写 .part 临时文件，校验长度和 SHA-256 后以同目录原子且不覆盖的方式安装。
- 默认拒绝同名文件；不自动解压，不执行收到的文件。
- Go 标准库实现，无第三方模块依赖。

## 明确阻塞（不是代码测试失败）

目前没有两台真实测试电脑的网络环境、已运行的 Xray 实例和经脱敏确认的双机部署映射。因此不能验证：
- A 到 B、B 到 A 的应用连通性；
- 两端 ichuansu 服务各自应该监听/连接的确切 IP:port；
- 文件数据确实经过用户现有 Xray 路径。

常规 Xray i.json 中的 outbound 节点地址只标识 Xray 基础设施，不足以唯一确定另一台个人电脑的 ichuansu 服务地址。当前 ResolvePeerEndpoint 和 ResolveListenEndpoint 因而故意 fail-closed；serve、iget、iput 在该条件未被验证前会报告阻塞，不会猜地址、开放公网监听、修改 i.json 或绕过 Xray。将同一个 .ichuansu-token 安全地部署到两端也是启用接收服务前的必要条件。

所以当前状态是：文件传输核心实现完成；双机 Xray 网络验收 BLOCKED / NOT VERIFIED；不能宣称 MVP 验收完成。

## 本地验证与构建

本地检查命令：

    gofmt -w .
    go test -race ./...
    go vet ./...
    go build -o ichuansu .
    GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/ichuansu .
    GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/_ichuansu.exe .

Windows 发布包需将 ichuansu.cmd 与 _ichuansu.exe 放在同一目录。两端将程序放入各自 Xray 程序目录，并确保同目录存在原有 i.json。
