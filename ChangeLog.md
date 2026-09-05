# ChangeLog

## Unreleased

- 将运行目标迁移至 GitHub Actions 的 Ubuntu 环境。
- 配置和生成的 `clash.yaml` 均改为仓库根目录文件，不再使用用户目录。
- 移除 `config.yaml` 中的 Mihomo 与 GeoIP 路径；部署工作流从官方 `MetaCubeX/mihomo` GitHub release 下载 Linux AMD64 核心。
- 增加推送/PR 的 Go 检查与二进制构建 CI，以及手动触发的 GitHub Pages 订阅部署 CD。
- 修复 benchmark 切换临时工作目录后无法找到 Mihomo 核心的问题。
- 恢复 `snakem982 proxypool` 的官方回退订阅源，并支持配置中的内联 fallback URL 列表。
- 使用完整节点配置去重，保留同一 `server:port` 但凭据或传输参数不同的节点。
- 改用完整 YAML 解析上游 Clash 配置，避免嵌套节点字段导致候选节点丢失。
- 将源抓取与延迟探测超时分离，允许较大的候选集完成质量检测。
- `AUTO-FAST` 改为包含所有已发布节点并继续自动测速选优；`AI-POOL` 保持前 10 个节点的范围。

## 0.01.00

- 建立 Go 命令行项目骨架和版本输出。
- 增加用户目录、配置模板、配置校验和 Mihomo 路径检查。
- 增加源抓取、大小限制、常见 JSON/YAML 解析、协议过滤和节点指纹去重。
- 增加跨源 round-robin 合并及重名节点处理。
- 增加 Mihomo controller 延迟探测核心、多轮中位延迟、抖动和质量门槛计算。
- 增加 Mihomo benchmark 临时目录、端口分配、配置预校验、启动等待、日志上限和清理逻辑。
- 保留受控的 UUID、密码、TLS、SNI、传输和认证字段，过滤上游证书放宽控制。
- 完善 HK/JP/US 区域代理组、AI 域名规则、Fallback 链路和订阅引用校验。
- 增加生成摘要，报告源失败数、候选数、发布数、区域数和输出路径。
- 增加常见 `ss://`、`trojan://`、`socks5://` 和 HTTP 代理 URI 解析。
- 扩展 `vless://`、`vmess://`、`hysteria2://`、`tuic://` URI 参数解析，保留 UUID、密码、TLS、SNI 和传输类型。
- 增加标准 VMess Base64 JSON 解码及服务器、端口、UUID、alterId、网络和 TLS 字段映射。
- 增加单节点响应体积限制，超大节点在进入 benchmark 前过滤。
- 增加平台相关的 Mihomo 文件检查：Windows 要求 `.exe`，类 Unix 要求执行权限。
- 探测请求超时与发布延迟门槛分离：单次探测允许 5 秒，发布门槛仍为 800ms。
- 无论是否使用 `--config`，初始化 `.pangolin` 时都会创建缺失的带注释 `config.yaml` 模板。