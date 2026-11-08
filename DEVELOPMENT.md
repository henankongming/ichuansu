# 开发进度

项目需求基线见 README.md。本文件只记录实现阶段，不替代需求规格。

## 当前阶段：阶段 1（骨架与本机诊断）

已实现：

- 单一 Go 命令入口，以及 --help、--version。
- 从可执行文件路径确定程序目录，不依赖启动时的当前工作目录。
- 只检查程序目录内的 xray / xray.exe 和 i.json，不扫描磁盘。
- 只读解析 i.json，显示入站协议/本地监听地址、出站协议数量和路由规则数量。
- 不输出服务器地址、域名、UUID、密码、私钥或完整配置。
- 检测本机 Xray 进程；Windows 通过系统 tasklist，Linux 读取 /proc。
- 提供 Windows CMD 启动器 ichuansu.cmd，调用同目录 _ichuansu.exe 并传回退出码。

尚未实现或验证：

- serve、iget、iput 尚未实现。
- 尚未发现或推断远端 ichuansu 应用端点。
- 尚未验证 A→B、B→A 的应用连通性，也未证明数据会经过既有 Xray 路径。
- 尚未实现文件传输、ZIP、SHA-256 或发布包。

## 本地开发检查

go test ./...
go build -o ichuansu .

Linux 目标：
GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/ichuansu .

Windows 目标：
GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/_ichuansu.exe .
将 ichuansu.cmd 一同复制到 dist/windows-amd64/。

将对应程序放入现有 Xray 程序目录，并确保同目录存在用户自己的 i.json。diagnose 只提供本机检查；真实网络路径仍需在两台测试电脑上验证。
