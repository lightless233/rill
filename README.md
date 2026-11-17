[简体中文](./README.md) | [English](./README-EN.md)

# Rill

轻量、自托管的 RSS 阅读器，基于 [Fusion](https://github.com/0x2E/fusion) 继续开发。保留简洁的阅读体验，并提供 AI 翻译、摘要和更丰富的阅读设置。

## 功能

- 订阅管理：RSS/Atom、订阅发现、分组和 OPML 导入导出。
- 日常阅读：未读管理、收藏、搜索、快捷键，以及读完当前订阅后继续浏览下一个未读订阅。
- AI 辅助：文章翻译、列表标题与摘要翻译、文章总结，使用自行配置的 OpenAI 兼容接口。
- 正文提取：为每个订阅配置 CSS 正文选择器和排除选择器，预览结果，并在拉取新文章时自动提取正文。
- 文章分享：生成带有效期的文章快照链接，可设置访问码。
- 阅读外观：多套浅色和深色配色，以及正文、中文和代码字体设置，包含 OPPO Sans。
- 实时未读数：后台抓取到新文章后，自动更新侧栏未读数。
- 多端使用：响应式界面、PWA、多语言，以及供第三方阅读客户端使用的 Fever API。
- 自托管：Go + SQLite，前端嵌入单个可执行文件，也可使用 Docker 部署。

## 安装

### 从源码构建

准备 Go、Node.js 和 pnpm。Go 与 Node.js 的版本要求分别见 [backend/go.mod](./backend/go.mod) 和 [frontend/package.json](./frontend/package.json)。以下命令使用 POSIX shell：

```sh
git clone https://github.com/lightless233/rill.git
cd rill
./scripts.sh build-frontend
./scripts.sh build-backend
FUSION_PASSWORD='replace-with-your-password' FUSION_LISTEN_HOST=127.0.0.1 ./build/fusion
```

打开 <http://localhost:8080>。将示例密码替换为自己的密码。

### 使用 Docker

安装 Docker，在仓库根目录构建镜像：

```sh
./scripts.sh build-frontend
./scripts.sh build-backend linux amd64 ./build/fusion-linux-amd64
docker build --platform linux/amd64 -t rill:local .
docker run -d --name rill --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -v "$PWD/data:/data" \
  -e FUSION_DB_PATH=/data/fusion.db \
  -e FUSION_PASSWORD='replace-with-your-password' \
  rill:local
```

打开 <http://localhost:8080>。订阅和文章数据保存在宿主机的 `data/` 目录中。示例构建目标为 `linux/amd64`；其他架构需要同时调整后端构建参数和 Docker 的 `--platform`。

已有 Docker Compose 和反向代理环境可参考 [deploy/compose.yaml](./deploy/compose.yaml)；该文件使用外部代理网络，需要按自己的环境配置。远程部署脚本及连接配置示例分别见 [deploy/deploy.sh](./deploy/deploy.sh) 和 [deploy/.env.example](./deploy/.env.example)。

## 配置与使用

- **服务配置**：环境变量参考 [`.env.example`](./.env.example)。项目目前保留 `FUSION_*` 环境变量名和 `fusion` 可执行文件名。
- **本地访问**：直接运行二进制时，可用 `FUSION_LISTEN_HOST=127.0.0.1` 限制为本机访问。只有在可信的本地环境中，才使用 `FUSION_ALLOW_EMPTY_PASSWORD=true` 配合空密码关闭密码登录。
- **公网访问**：通过反向代理提供 HTTPS，并按部署环境设置可信代理和 CORS。Docker 容器内不要将监听地址设为回环地址，否则代理无法连接。
- **AI 功能**：在「设置 → AI」配置接口地址、API Key 和模型，可测试连接并设置翻译展示方式。
- **正文提取**：在订阅编辑窗口配置规则并预览，保存后用于新拉取的文章，不回填已有文章；提取失败时保留 RSS 原有内容。
- **分享阅读**：分享页展示创建分享时的文章快照，无需登录；分享页不提供翻译或 AI 总结操作。
- **第三方客户端**：配置方法见 [Fever API 文档](./docs/fever-api.md)。SSO/OIDC 等其他服务配置见 [`.env.example`](./.env.example)。

## 开发与版本

本地开发、检查命令和提交前敏感信息扫描见 [CONTRIBUTING.md](./CONTRIBUTING.md)，构建入口见 [scripts.sh](./scripts.sh)。

Git tag 和构建版本约定见 [版本说明](./CONTRIBUTING.md#5-release-versions)。可在设置的「关于」页面查看运行版本，或执行：

```sh
./build/fusion --version
```

## 文档与反馈

- [文档索引](./docs/README.md)
- [API 约定](./docs/api-conventions.md)
- [后端设计](./docs/backend-design.md)
- [前端设计](./docs/frontend-design.md)
- [问题反馈与功能建议](https://github.com/lightless233/rill/issues)

## 致谢与许可

Rill 基于 [Fusion](https://github.com/0x2E/fusion) 开发，感谢原作者与贡献者。Feed 解析由 [gofeed](https://github.com/mmcdole/gofeed) 提供。

采用 [MIT License](./LICENSE)，保留上游版权声明。
