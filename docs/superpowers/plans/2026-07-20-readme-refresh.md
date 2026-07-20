# README Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stale root README with a user-focused guide that accurately presents current SuperFlare features, deployment paths, persistent data, and operating guidance.

**Architecture:** Keep `README.md` as the concise repository entry point and link to the existing platform guides for detailed Docker, Linux, and fnOS procedures. Derive every feature, field, route, and command from the current code or repository scripts; do not change application behavior.

**Tech Stack:** Markdown, Go 1.26, PowerShell, Git

## Global Constraints

- Primary audience: people evaluating, deploying, upgrading, or operating SuperFlare.
- Root README must stay user-focused; platform-specific detail remains in `tools/docker/README.md`, `tools/linux/README.md`, and `fnapp/README.md`.
- Document only fields and behavior already supported by the current repository.
- State that `config.yml`, `apps.yml`, `bookmarks.yml`, `ports.yaml`, optional `.env`, and `var/` are persistent runtime data.
- State that favorites apply to ordinary bookmarks, while private visibility applies to both applications and bookmarks.
- Preserve the user's uncommitted `fnapp/superflare/manifest` and `tools/superflare-icon.zip` changes.

---

### Task 1: Refresh The Root README

**Files:**
- Modify: `README.md`
- Reference: `config/defaults/config.yml`
- Reference: `config/defaults/apps.yml`
- Reference: `config/defaults/bookmarks.yml`
- Reference: `config/model/application.go`
- Reference: `config/model/bookmark.go`
- Reference: `tools/docker/README.md`
- Reference: `tools/linux/README.md`
- Reference: `fnapp/README.md`

**Interfaces:**
- Consumes: current configuration fields, route paths, platform scripts, and persistence guarantees from the referenced files.
- Produces: a self-contained GitHub landing document with valid relative links and runnable commands.

- [ ] **Step 1: Replace the stale README structure**

Write `README.md` in Chinese with these exact top-level sections and responsibilities:

```markdown
# SuperFlare

## 功能亮点
## 部署方式
## 快速开始
## 登录与管理入口
## 运行数据与升级
## 配置示例
## 图标与网络访问
## 开发与校验
## 仓库结构
## 致谢
```

The introduction must identify SuperFlare as a lightweight self-hosted navigation homepage derived from `soulteary/docker-flare`. The feature section must cover applications/bookmarks, private items, ordinary-bookmark favorites, description tooltips, application subdirectory modals, search, appearance, online editing, backup/restore, public-link checks, port discovery, multilingual UI, and resilient favicon fallback without claiming that external icon fetches always succeed.

- [ ] **Step 2: Add deployment and first-run instructions**

Use a Markdown table to distinguish these supported paths:

```markdown
| 方式 | 适用场景 | 入口 |
| --- | --- | --- |
| 源码运行 | 本地体验与开发 | 本 README 的“快速开始” |
| Docker | 容器化部署 | [`tools/docker/README.md`](tools/docker/README.md) |
| Linux 原生 | 二进制与 systemd | [`tools/linux/README.md`](tools/linux/README.md) |
| Windows | PowerShell 构建与后台运行 | `tools/windows/restart-superflare.ps1` |
| fnOS | NAS 原生 FPK | [`fnapp/README.md`](fnapp/README.md) |
```

Include these runnable source commands and the default URL:

```bash
go run ./build/build.go
go run .
```

```text
http://127.0.0.1:3636/
```

Document the default `admin` / `admin` credentials, immediate credential rotation, stable random `FLARE_COOKIE_SECRET` guidance, and the `/settings`, `/editor`, and `/settings/ports` routes. State that changing credentials invalidates the current session and requires a new login.

- [ ] **Step 3: Document persistent data and supported configuration**

List the four YAML files, optional `.env`, and `var/`. Explain that native deployments resolve them from the process working directory, Docker must persist `/data`, and fnOS upgrades preserve existing configuration while only filling missing files or keys.

Include concise examples grounded in the current schema:

```yaml
# apps.yml
links:
  - name: NAS
    link: https://nas.example.com
    local_link: http://192.168.1.10:5000
    subdir: 存储服务
    private: true
```

```yaml
# bookmarks.yml
categories:
  - id: "1"
    title: 开发
links:
  - name: SuperFlare
    link: https://github.com/junfuchang/superflare
    category: "1"
    desc: 项目仓库
    favorite: true
    private: false
```

Explain `local_link`, `subdir`, `desc`, `favorite`, and `private` in plain language. Make it explicit that a disabled login mode is treated as trusted access, so private items remain visible.

- [ ] **Step 4: Verify Markdown facts, links, and formatting**

Run:

```powershell
$required = @(
  'tools/docker/README.md',
  'tools/linux/README.md',
  'fnapp/README.md',
  'tools/windows/restart-superflare.ps1'
)
$missing = $required | Where-Object { -not (Test-Path $_ -PathType Leaf) }
if ($missing) { throw "Missing README target(s): $($missing -join ', ')" }

$readme = Get-Content -Encoding UTF8 -Raw README.md
@('favorite: true', 'private: true', 'FLARE_COOKIE_SECRET', '/settings', '/editor', '/data') |
  ForEach-Object { if (-not $readme.Contains($_)) { throw "README is missing: $_" } }

git diff --check -- README.md
```

Expected: all paths exist, all required facts are present, and `git diff --check` exits with code 0 and no output.

- [ ] **Step 5: Run repository verification**

Run:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
powershell -ExecutionPolicy Bypass -File .\tools\test-all-scripts.ps1
```

Expected: the Go test suite and build exit with code 0; repository script syntax, format, lifecycle preservation, and optional Docker checks report success or an explicit environment-based skip.

- [ ] **Step 6: Review and commit only the documentation change**

Run:

```powershell
git diff -- README.md
git status --short
git add -- README.md docs/superpowers/plans/2026-07-20-readme-refresh.md
git diff --cached --check
git commit -m "docs: refresh README for current features"
```

Expected: the staged diff contains only `README.md` and this plan; `fnapp/superflare/manifest` and `tools/superflare-icon.zip` remain unstaged.

- [ ] **Step 7: Push and verify GitHub main**

Run:

```powershell
git push origin main
$head = git rev-parse HEAD
$tracking = git rev-parse origin/main
$remote = (git ls-remote --heads origin refs/heads/main).Split("`t")[0]
if ($head -ne $tracking -or $head -ne $remote) { throw 'Local and remote main refs differ.' }
```

Expected: Git reports `main -> main`, and all three commit IDs are identical.
