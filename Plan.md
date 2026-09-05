# Pangolin GitHub Actions 运行方案

## 运行约定

Pangolin 运行在 GitHub-hosted Ubuntu runner，不再维护 Windows 本地用户目录。仓库根目录保存唯一配置 `config.yaml`，手动部署时生成根目录 `clash.yaml`。

`config.yaml` 只定义订阅源。Mihomo core 不是用户配置：部署工作流从官方 [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo) release 获取最新 Linux AMD64 gzip 资产，解压至 `tools/mihomo/mihomo` 并赋予执行权限。

## 持续集成

`.github/workflows/ci.yml` 在 push 与 pull request 时执行：

1. `go vet ./...`
2. `go test ./...`
3. 构建 `dist/pangolin-linux-amd64`
4. 上传二进制为 GitHub Actions artifact

## 持续部署

`.github/workflows/deploy.yml` 仅由 `workflow_dispatch` 手动触发：

1. 检出用户推送的代码和 `config.yaml`。
2. 下载官方 Mihomo core。
3. 执行 Pangolin，得到 `clash.yaml`。
4. 将该文件作为 GitHub Pages artifact 部署。

首次部署前，仓库 Settings > Pages 的 Source 必须选择 **GitHub Actions**。部署后订阅地址为 `https://<owner>.github.io/<repository>/clash.yaml`。
