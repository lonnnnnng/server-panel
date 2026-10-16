# Server Panel · 服务器面板

面向 Linux 服务器的轻量级、只读监控面板。使用 Go 标准库采集系统指标，通过 SSE 实时刷新，不依赖 Node.js、Python、数据库或前端 CDN。

默认监听 **`0.0.0.0:8888`**。源码已从原 `monitor/go` 平铺整理到项目根目录，不再保留多语言目录层级。

## 功能

- CPU：总使用率、各核使用率、负载、频率、用户/系统/IO 等待/steal/idle 分项与温度。
- 内存：物理内存、Swap、缓存、slab、Active/Inactive、Dirty、匿名页、页表、Commit 等明细。
- 磁盘：根分区、全部文件系统、整盘读写吞吐、IOPS、忙碌度及设备明细。
- 网络：网卡状态、速率、双工、MTU、驱动、IPv4 地址、累计流量、实时吞吐、包速率、错误与丢包。
- 硬件：主板/BIOS、CPU、内存页与 NUMA、硬盘、显卡、网卡型号。
- 进程与端口：Top CPU/内存进程、线程、RSS/VSZ、TCP 状态、TCP/UDP 监听端口与所属进程。
- 系统：主机、OS、内核、架构、开机时间、时区、虚拟化/容器、PSI、温度、文件句柄与历史趋势。
- 部署：静态资源内嵌二进制、Linux amd64/arm64 Release、校验和、systemd 管理、开机自启与异常退出重启。

## 使用边界

面板**没有登录鉴权**，会显示主机、进程名称、命令行和网络信息。仅在可信局域网使用，不要直接开放到公网；需要远程访问时请绑定 `127.0.0.1` 并使用 SSH 隧道，或在反向代理层添加 TLS 与鉴权。只读监控不提供上传、删除文件或执行系统命令的 API。

服务建议以普通用户运行。受 Linux 权限限制，部分其他用户进程、端口归属或硬件信息可能不可见，页面按实际可读取结果展示。数据保存在内存中，重启服务会清空历史趋势；程序只负责监控，不负责代理网络或自动修复断网。

## 项目结构

```text
server-panel/
├── main.go                    # 原有 Linux 指标采集实现
├── server.go                  # HTTP/SSE、内嵌前端、版本与配置校验
├── server_test.go             # 配置、API、SSE 与采集辅助函数测试
├── public/                    # 无外部依赖的前端源码
├── config/server-panel.env.example
├── deploy/server-panel.service.in
├── scripts/build.sh           # 本机构建
├── scripts/release.sh         # 构建 Linux Release 与校验和
├── scripts/service.sh         # systemd 服务管理
├── VERSION
└── CHANGELOG.md
```

Release 压缩包在以上目录基础上包含 `bin/server-panel`；无需在服务器安装 Go。前端已经编入二进制，直接改 `public/` 不会影响正在运行的版本，需重新构建发布。

## 从 Release 安装（推荐）

以下以 Linux x86_64 / amd64、已有普通用户 `lee` 为例。ARM64 设备将包名里的 `amd64` 替换为 `arm64`，并修改校验文件筛选项。

安装和服务管理需要 Bash、systemd、curl、tar、sha256sum，以及系统自带的 awk、sed、install、mktemp、id 工具。

```bash
release_tag=v0.1.0
release_url="https://github.com/lonnnnnng/server-panel/releases/download/$release_tag"
mkdir -p "$HOME/server-panel-download"
cd "$HOME/server-panel-download"
curl -fL --retry 3 -O "$release_url/server-panel_0.1.0_linux_amd64.tar.gz"
curl -fL --retry 3 -O "$release_url/SHA256SUMS"
grep '  server-panel_0.1.0_linux_amd64.tar.gz$' SHA256SUMS | sha256sum -c -

# 首次安装要求目标目录不存在，避免覆盖其他项目。
test ! -e "$HOME/server-panel"
tar -xzf server-panel_0.1.0_linux_amd64.tar.gz -C "$HOME"
cd "$HOME/server-panel"
sudo ./scripts/service.sh install --user lee
./scripts/service.sh health
```

在浏览器访问 `http://服务器局域网IP:8888/`。安装命令同时注册 systemd、立即启动服务并启用开机自启；不需要登录后再运行启动命令。不自动更改防火墙，不建立公网端口映射。

## 服务管理

在安装目录执行：

```bash
sudo ./scripts/service.sh start       # 启动并等待健康检查
sudo ./scripts/service.sh stop        # 停止
sudo ./scripts/service.sh restart     # 重启并等待健康检查
sudo ./scripts/service.sh status      # 状态和开机自启状态
sudo ./scripts/service.sh logs        # 最近 100 条 journal 日志
sudo ./scripts/service.sh enable      # 启用开机自启，不改变当前运行状态
sudo ./scripts/service.sh disable     # 关闭开机自启，不停止当前进程
./scripts/service.sh health           # 健康、版本、提交与构建时间
sudo ./scripts/service.sh uninstall   # 停止并取消服务注册，保留源码和配置
```

对应系统服务为 `server-panel.service`，也可以直接使用 `systemctl` / `journalctl`。服务采用前台进程让 systemd 正确跟踪 PID，异常退出后 3 秒重启；正常手动停止不会自动拉起。服务日志写入 systemd journal，不再维护无上限增长的项目日志文件。journal 容量与保留期遵循系统设置。

## 配置

首次安装会复制 `config/server-panel.env.example` 为 `config/server-panel.env`；已有配置不会被安装脚本覆盖。编辑实际配置后重启服务生效。

| 配置 | 默认 | 允许范围/说明 |
| --- | --- | --- |
| `HOST` | `0.0.0.0` | 监听地址；仅本机访问时用 `127.0.0.1` |
| `PORT` | `8888` | 1–65535，避免与其他服务冲突 |
| `SAMPLE_INTERVAL_MS` | `1000` | 100–60000 毫秒 |
| `MAX_HISTORY` | `120` | 1–3600 个采样点，越大越占内存 |

配置使用示例中的不带引号 `KEY=value` 格式。文件由 systemd 作为 EnvironmentFile 读取，二进制不会自行加载配置文件；前台运行时请显式传入环境变量。错误的数值配置会阻止启动，不会偷偷使用其他端口。

## 源码运行与验证

Go 1.22 或更高版本；指标采集面向 Linux，macOS 可编译和运行 HTTP 单元测试，但不能替代 Linux 真机指标验收。

```bash
go vet ./...
go test -race ./...
./scripts/build.sh
./bin/server-panel --version
HOST=127.0.0.1 PORT=8888 ./bin/server-panel
```

验证接口：

```bash
curl -fsS http://127.0.0.1:8888/api/health
curl -fsS http://127.0.0.1:8888/api/metrics
curl -N http://127.0.0.1:8888/api/events
```

页面 `/`、资源 `/app.js` / `/styles.css` 和上述 API 均只接受 GET；未提供数据库、上传或写入接口。健康接口包含 `version`、`commit`、`buildTime`，可用于确认实际部署的 Release。

## 升级与发布

维护者先更新 `VERSION`、`CHANGELOG.md`，检查并提交代码，再执行：

```bash
./scripts/release.sh
git tag -a v0.1.0 -m '发布服务器面板 v0.1.0'
git push origin main v0.1.0
gh release create v0.1.0 dist/server-panel_0.1.0_linux_amd64.tar.gz \
  dist/server-panel_0.1.0_linux_arm64.tar.gz dist/SHA256SUMS \
  --verify-tag --title 'Server Panel v0.1.0' --notes-file CHANGELOG.md
```

发布脚本拒绝未提交的源码，二进制注入实际提交哈希；`SHA256SUMS` 对应两个 Release 压缩包。构建使用 `CGO_ENABLED=0`，避免依赖构建机的动态 libc。

服务器升级前先下载并校验对应架构的新包，将现有版本和实际配置备份到安装目录之外，然后 `sudo ./scripts/service.sh stop`，在安装目录的父目录解压新包，再执行 `sudo ./scripts/service.sh install --user lee`。实际 `config/server-panel.env` 不在 Release 包内，因此原配置保留。升级失败时恢复备份目录并重新 install；不要在运行中直接覆盖二进制。

## 指标采集口径

- Linux 系统信息主要来自 `/proc`、`/sys`；可选命令 `df`、`ip`、`lsblk`、`lspci`、`uname`、`systemd-detect-virt` 用于补充信息，缺失时相关字段可能为空。
- 网络累计值来自 `/proc/net/dev`，网卡或系统重启会重置；速率由前后采样差值计算。
- 磁盘明细包含物理设备和映射设备，汇总优先仅统计物理整盘，避免重复相加。
- 温度读取 `/sys/class/thermal/thermal_zone*`，保留 sysfs 链接；CPU 温度优先选择 `x86_pkg_temp`、`coretemp`、`k10temp` 等类型。
- 硬件参数缓存 30 秒，避免每次采样都执行外部硬件命令。
- 不展示 machine-id、产品 UUID、序列号、磁盘序列号和 MAC 地址等唯一标识；页面仍包含运维敏感信息，须遵守前述访问边界。

## 来源

监控与页面代码迁移自服务器已有的 `monitor` Go 版本，原仓库为 [MayLabPro/monitor](https://github.com/MayLabPro/monitor)。本项目在保留监控能力的基础上整理目录、部署、版本发布与服务管理。
