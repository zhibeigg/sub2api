# 同步官方更新 & 发布自己的版本

本仓库是 `Wei-Shaw/sub2api` 的定制 fork（`zhibeigg/sub2api`）。**更新检测源已改为本 fork**
（`backend/internal/service/update_service.go` 的 `githubRepo`），所以后台「更新」按钮只会跟踪
你自己发布的 Release，官方发新版**不会**覆盖你的定制代码。

线上部署使用 CI 构建的镜像 `ghcr.io/zhibeigg/sub2api:latest`（打 tag 时自动构建并推送）。

## remote 约定
- `upstream` → 官方 `Wei-Shaw/sub2api`（拉官方更新用）
- `fork` → 你的 `zhibeigg/sub2api`（推代码 / 发版用）

```
git remote -v   # 确认已有 upstream 和 fork
```

## 一、同步官方更新（想合并官方新功能时）
```
# 1. 拉官方最新
git fetch upstream

# 2. 合并到你的定制分支（解决冲突；你的定制文件冲突时以你的为准，
#    但要吸收官方对同一文件的改动）
git merge upstream/main
#   或用 rebase： git rebase upstream/main

# 3. 冲突解决后，递增版本号
#    编辑 backend/cmd/server/VERSION（如 0.2.0 -> 0.3.0）

# 4. 本地构建验证（可选）
#    cd backend && go build -tags embed ./...
#    cd frontend && pnpm typecheck && pnpm lint:check && pnpm build
```
> 冲突高发文件：`update_service.go`（保留 `githubRepo = "zhibeigg/sub2api"`）、
> 你改过的设置链路、前端定制视图。合并时逐一确认你的定制没被官方版覆盖。

## 二、发布新版本（把改动发成 Release + 镜像）
```
# 推代码到 fork
git push fork main

# 打 tag（vX.Y.Z 要与 VERSION 一致）
git tag -a v0.3.0 -m "说明"
git push fork v0.3.0
```

CI（`.github/workflows/release.yml`）会自动：
- 构建各平台二进制 + `checksums.txt`，发布到 fork 的 Releases
- 构建并推送 `ghcr.io/zhibeigg/sub2api:{版本}` 和 `:latest` 镜像

> ⚠️ fork 首次可能不会因 tag push 自动触发 CI，需要手动跑一次：
> `gh workflow run Release --repo zhibeigg/sub2api -f tag=v0.3.0 -f simple_release=false`
> 之后 tag push 会自动触发。

## 三、部署到服务器
```
# 服务器上（/home/docker/sub2api）
docker compose pull sub2api          # 拉 ghcr.io/zhibeigg/sub2api:latest
docker compose up -d sub2api
```
或后台点「更新」按钮（它会检测到 fork 的新 Release）——但 Docker 部署下推荐用
`docker compose pull` + `up -d`，因为容器重启会用镜像里的二进制，最干净可靠。

## 版本号规则（沿用项目约定 A.B.C）
- A：不兼容的公开接口变更
- B：新增功能
- C：Bug 修复 / 兼容性修复
