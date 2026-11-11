# iPerf3 GUI

Go + Wails 的 Windows 桌面 iPerf3 测试台。前端沿用 `iperf3-gui-demo.html` 的视觉与三视图结构，后端负责随包引擎、进程生命周期、JSON 流解析、SQLite 历史和导出。

GitHub：https://github.com/gzx19990101/iperf3-gui

## 功能

- 随包 iPerf3 3.22（Cygwin x64 构建），启动时探测版本与输出能力
- TCP/UDP 上传、下载、双向测试；UDP 双向需要服务端支持多端口，客户端对兼容服务端可用，本机 Cygwin 服务端存在已知限制
- 实时区间指标、按方向汇总、原始输出与失败诊断
- 本机服务端管理：监听端口、绑定地址、单次连接；连接记录包含协议、方向、上传/下载吞吐、开始/结束时间和状态，并提供实时吞吐曲线、区间明细与服务日志
- SQLite 历史：查询、方向/协议筛选、删除选中、清空全部
- JSON/CSV 导出，路径通过系统保存对话框选择
- 按天诊断日志，包含引擎状态、测试生命周期与服务端输出

## 停止行为

手动停止客户端或服务端时，应用通过 `-I` PID 文件取得 iperf3 的 Cygwin PID，调用随包 `kill.exe -INT` 发送 SIGINT，让 iperf3 输出结束汇总并自然退出；3 秒内未退出才回退到强制结束。停止客户端不会停止本机服务端。

## 环境

- Windows 10/11 x64，WebView2 Runtime
- Go 1.27+
- Wails CLI v2.17.1
- Node.js 仅用于可选的构建工具链；当前前端直接嵌入，无需 npm 构建

## 构建

```powershell
go test ./...
wails build -s -clean
```

`-s` 表示跳过前端构建，`frontend/` 会直接嵌入应用。

产物：`build/bin/iperf3-gui.exe`

## 测试

```powershell
go test ./...
go test -tags integration ./internal/session -timeout 120s
```

集成测试会在本机启动真实 iPerf3 服务端与客户端，覆盖 TCP 上传/下载/双向、UDP 上传/下载，以及手动停止客户端后服务端保持监听。

## 数据目录

| 内容 | 路径 |
| --- | --- |
| SQLite 历史 | `%LOCALAPPDATA%\Iperf3GUI\iperf3.db` |
| 随包引擎 | `%LOCALAPPDATA%\Iperf3GUI\engine\3.22\` |
| 诊断日志 | `%LOCALAPPDATA%\Iperf3GUI\logs\app-YYYYMMDD.log` |
| WebView2 数据 | `%LOCALAPPDATA%\Iperf3GUI\webview\` |

## 打包说明

当前 `internal/engine/assets/` 中的 iPerf3 是 Cygwin 构建，包含 `iperf3.exe`、`kill.exe`、`cygwin1.dll`、`cygcrypto-3.dll`、`cygz.dll`。`kill.exe` 用于在手动停止时向 iperf3 发送 `SIGINT`，让测试自然收尾；Cygwin 服务端在 UDP 双向/多流场景存在已知上游问题（esnet/iperf#1992），客户端模式可正常对兼容服务端使用。分发时请遵守 `THIRD_PARTY_NOTICES.md` 与 `licenses/` 中的许可证和源码提供义务；更稳妥的方式是替换为原生 Windows x64 构建。

## 许可证

本项目源代码采用 MIT 许可证，见 [LICENSE](LICENSE)。随包分发的 iPerf3、Cygwin、OpenSSL、zlib 二进制保留各自许可证，详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 与 [licenses/](licenses/)；Cygwin 运行时的对应源码获取方式见该文件。
