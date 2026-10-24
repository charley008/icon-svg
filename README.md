# icon-svg

使用 Go 和 HTML 模板构建的图标库。`library/` 中的 SVG 和 `meta.json` 是素材的唯一来源；SQLite 仅作为内存查询索引，以及账号、日志、下载统计的持久化存储。页面与静态文件从外部 `web/` 读取。

- 按分类或合集浏览，支持名称、别名、域名及标签检索。
- 图标版本预览、默认版本、收藏、SVG / PNG 下载和批量打包。
- 后台维护合集、产品、分类、标签、上传、回收站及默认图标。
- 设置按需展示数据备份、账号安全、分类管理和操作日志。
- 前后台深色/浅色模式，英文在前、中文按拼音排序。

## 初始数据

当前 library 包含 **641 个合集、227 个产品、1824 个 SVG、23 个分类**，保留当前已上架内容、默认版本、分类、标签和版本序号。

启动时扫描文件库并重建内存索引，之后默认每 5 分钟扫描一次；设置中也可手动刷新。后台修改直接更新素材文件和索引。全新安装的操作日志、下载次数和会话为空，管理员账号独立生成；仓库不包含数据库或凭据。

## Windows

当前分支为尚未发布的文件素材库版本，可按下文从源码编译。后续下载对应版本的 [Release 完整 ZIP](https://github.com/charley008/icon-svg/releases)，完整解压到可写目录后双击 `icon-svg.exe`。无需安装 Go、Python、Node.js；请保留 `library` 和 `web`，EXE 不包含这些文件。

```text
icon-svg/
├── icon-svg.exe
├── library/
│   ├── google/                 SVG 与 meta.json
│   ├── categories.json         分类列表
│   └── licenses/
├── web/
│   ├── templates/
│   └── static/
├── data/                       初始仅有说明文件，启动后生成数据库
├── README.md
└── THIRD_PARTY_NOTICES.txt
```

默认打开浏览器；保持控制台窗口运行，按 Ctrl+C 退出。路径以程序位置为准，不受启动时的工作目录影响。修改模板后重启程序生效，静态资源可直接更新后刷新；修改 library 后等待扫描，或在后台设置中点击“刷新素材库”。

自行编译需要 Go 1.26.3 或更高版本：

```powershell
.\build.cmd
.\bin\icon-svg.exe
```

编译后也可直接双击 `bin/icon-svg.exe`，默认打开 <http://127.0.0.1:8088/>，后台地址 `/admin`。保持控制台窗口运行，按 Ctrl+C 退出。

首次生成的管理员凭据位于 `data/initial-admin.txt`，用户名与密码各占一行。修改密码后该文件会删除。密码最少 8 位，不预设通用密码。

源码编译输出到 `bin/` 时，从父目录读取 `library/`、`web/`，并使用父目录的 `data/`。分发部署请使用完整 ZIP，不要只复制 EXE。

运行 `icon-svg.exe -version` 可查看版本号。

开发时也可在项目根目录运行 `go run ./cmd/icon-svg`。版本号统一保存在 `cmd/icon-svg/VERSION`。

可将 `.env.example` 复制为 `.env` 设置地址、端口等。环境变量优先，命令行参数可覆盖：

```powershell
.\bin\icon-svg.exe -listen 127.0.0.1:8090 -open=false
.\bin\icon-svg.exe -data D:\icon-svg-data -open=false
```

## Docker Compose

```sh
docker compose pull
docker compose up -d
docker compose ps
docker compose logs --tail 50 icon-svg
```

默认拉取 [charley008/icon-svg](https://hub.docker.com/r/charley008/icon-svg) 的 `latest` 镜像，无需本地编译。可在 `.env` 中设置 `IMAGE_TAG=1.0.0` 固定版本。Compose 默认地址为 <http://127.0.0.1:1089/>。素材使用名为 `icon-svg-library` 的 Docker 命名卷挂载到 `/app/library`，首次创建时自动复制镜像中的初始素材；账号与运行数据仍通过宿主机 `./data` 挂载到 `/app/data`。容器名为 `icon-svg`，重启策略为 `always`，Compose 默认每 60 分钟扫描素材。服务以非 root 用户运行，默认使用 library 挂载目录所有者的 UID/GID（UID 为 0 时使用 10001）；也可设置 `PUID` / `PGID`。请确保该用户对素材目录有读写权限。入口仅调整 data 根目录归属，不递归修改素材目录。模板和静态文件位于镜像内的 `/app/web`；如需直接维护，也可添加 `./web:/app/web:ro` 映射。

查看首次凭据：

```sh
docker compose exec icon-svg cat /app/data/initial-admin.txt
```

更新镜像时运行 `docker compose pull`，再执行 `docker compose up -d`。`docker compose down` 保留素材卷及宿主机的 `./data`。不要执行 `docker compose down -v`，它会删除素材卷。更新时保留素材卷和 `data`；素材变动在扫描后生效，不需重新初始化数据库。

首次部署只需 `docker-compose.yaml`，无需手动创建 `library` 或下载初始素材；Docker 会创建素材卷及 `./data`。再次启动或更新镜像会保留卷内已有素材，不会重新覆盖。

**如果已经用 `./library:/app/library` 部署过，请继续使用原目录绑定**，或先从后台导出素材、改用命名卷后再导入；直接切换为新卷不会自动迁移旧目录。已有管理员数据请继续挂载原来的 `./data`。后台提供素材导出和完整备份。

反向代理后的 HTTPS 部署请在 `.env` 中设置 `PUBLIC_ORIGIN=https://你的域名`、`COOKIE_SECURE=true`。默认端口仅监听宿主机回环地址。

如需自行从源码构建：

```sh
docker compose -f docker-compose.yaml -f docker-compose.build.yaml up -d --build
```

支持在 WSL2 中运行 Docker Engine 和 Compose。在项目目录内执行上面的 Compose 命令即可。

不下载源码或初始素材时，也可直接运行镜像，并使用 Docker 命名卷自动填充初始素材：

```sh
docker run -d --name icon-svg --restart unless-stopped -p 127.0.0.1:8088:8000 -v icon-svg-library:/app/library -v icon-svg-data:/app/data charley008/icon-svg:latest
```

这里使用 Docker 命名卷，首次创建素材卷时会复制镜像内的初始素材，后续更新镜像保留已有素材和账号数据。也可将完整源码或 ZIP 中的 `library` 目录绑定到 `/app/library`；不要用空目录覆盖镜像素材。`docker-compose.build.yaml` 提供可选的本地构建覆盖配置。


## 配置

详见 [.env.example](.env.example)。

| 配置 | 用途 |
| --- | --- |
| `LISTEN_ADDR` | 本地程序监听地址，默认 `127.0.0.1:8088` |
| `DATA_DIR` | 数据目录，可选绝对路径 |
| `OPEN_BROWSER` | 是否自动打开浏览器 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | 仅创建第一个管理员时使用；密码为空则自动生成 |
| `IMAGE_TAG` | Docker Hub 镜像版本，默认 `latest`，也可固定为 `1.0.0` |
| `BIND_ADDRESS` / `PORT` | Compose 宿主机地址和端口 |
| `PUBLIC_ORIGIN` | 反向代理后的完整外部地址 |
| `COOKIE_SECURE` | HTTPS 部署设为 `true` |
| `LIBRARY_DIR` | 素材库目录，默认程序旁的 `library/`，合集目录直接位于其下 |
| `SCAN_INTERVAL` | 定期扫描间隔，默认 `5m`；设为 `0` 关闭定期扫描，启动和手动扫描仍可用 |
| `PUID` / `PGID` | 可选 Docker 服务 UID/GID，默认匹配 library 挂载目录所有者 |
| `WEB_DIR` | 外部网页目录，默认程序旁的 `web/` |

## 数据维护

后台编辑合集、产品、分类、标签及默认版本，会直接保存到 `library/`。分类不确定时使用“其他”。修改展示资料不需要重新编译。

- **网站维护**：上传 SVG 后自动命名并更新 `meta.json`。回收站状态使用文件中的 `deleted` 标记，可恢复；下架状态使用 `published: false`。
- **GitHub 维护**：直接编辑 `library/<合集>/meta.json`，本地拉取后等待扫描或手动刷新。不要同时在后台编辑与执行 Git 拉取；检测到外部修改时，后台保存会拒绝覆盖并提示先刷新。
- **直接添加 SVG**：已有合集内可添加 `<合集>[-<产品>][-<序号>].svg`，扫描会登记并更新校验值。新合集需有 `meta.json`；独立产品需先在 `products` 中声明。删除 SVG 或整个合集目录后，扫描同步移除对应索引。
- **完整备份**：ZIP 包含 `library/` 和 `data/runtime.sqlite3`，保留素材、账号、日志和统计。恢复时先停止程序，再恢复这两个目录。
- **导出已上架素材**：只包含当前上架内容及许可，不包含账号与运行记录；可用于新安装或同步 GitHub。

扫描采用“发现文件变化 → 校验资料 → 发布索引”的流程。普通扫描每次重读元数据，SVG 通过文件身份、大小和修改时间判断是否复用缓存；没有变更时不重建索引。发现变更后统一校验，成功后更新内存索引；失败保留上次可用索引。后台设置显示扫描状态、时间、耗时和新增/修改/移除数量。

“完整扫描”绕过 SVG 缓存并重新读取所有素材，适用于同步工具保留原修改时间或需要重新检查全部内容的情况。普通重复扫描请求会合并；完整扫描会等待正在运行的扫描完成。后台保存前仍读取真实文件检查冲突，避免缓存掩盖外部修改。

素材 ID 用于保持详情链接稳定，由程序写入 `meta.json`，建议保留。索引可随时重建；不需要清空账号数据库。JSON 或 SVG 无效时保留上一份可用索引，并在控制台输出扫描错误。

默认图标由 `is_default` 决定，不依赖文件名。新版本编号递增，删除后不复用。方形 SVG 上传时自动添加矢量圆角，页面与下载使用相同素材。

## 开发与检查

GitHub Actions **仅在推送版本标签时运行**，普通提交、Pull Request 和网页编辑均不触发。标签采用 `v1.0.0` 格式，并须与 `cmd/icon-svg/VERSION` 一致。流程依次执行测试与 Docker 启动检查、Windows 完整 ZIP 构建和 Release 发布，最后推送 Docker Hub 镜像（`linux/amd64`、`linux/arm64`）。

首次发布前，在 GitHub 仓库 **Settings → Secrets and variables → Actions → Secrets** 设置：

| Secret | 内容 |
| --- | --- |
| `DOCKER_USERNAME` | Docker Hub 用户名 |
| `DOCKER_PASSWORD` | Docker Hub Access Token，需有目标镜像仓库的读取和写入权限 |

先在 Docker Hub 创建公开的 `icon-svg` 镜像仓库。默认推送到 `<DOCKER_USERNAME>/icon-svg`；如需其他名称，可在 GitHub Actions Variables 设置 `DOCKERHUB_REPOSITORY`（仅仓库名，不含用户名）。GitHub 源码仓库仍保持私有；公开镜像包含程序、网页和初始素材，不包含本地账号、日志或数据库。

例如发布 `v1.0.0` 会生成镜像标签 `1.0.0` 和 `latest`，以及 GitHub Release 的 Windows ZIP 和 SHA256 校验文件。发布操作：

```sh
git tag v1.0.0
git push origin main
git push origin v1.0.0
```

后续发布先修改 `cmd/icon-svg/VERSION` 并提交，再推送对应的新标签。

```sh
go test ./...
go vet ./...
node --check web/static/site.js
node --check web/static/theme.js
```

目录说明：

```text
cmd/icon-svg/        程序入口和版本号
internal/app/        应用、素材库、存储和对应测试
web/templates/       页面模板
web/static/          CSS、JavaScript 和站点标识
internal/svgstyle/   SVG 圆角处理
library/<slug>/      SVG 及 meta.json
library/licenses/    素材许可说明
bin/                 编译产物，不提交
data/                账号、日志、统计和首次凭据，不提交
```

图标、商标及素材许可见各 `meta.json` 和 [library/licenses](library/licenses/README.md)。第三方 Go 依赖许可见 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)。

## 文件写入安全

合集及产品标识只允许小写 ASCII 字母、数字、单个点或连字符分隔；拒绝路径分隔符、绝对路径、Windows 设备名和保留目录名。上传文件名不作为落盘路径。

所有素材操作通过 `os.Root` 限定在 library 内，扫描拒绝符号链接及特殊文件；写入使用随机排他临时文件、同步和原子替换，不截断目标文件。多文件保存使用恢复日志，进程中断后下次启动继续恢复。单个素材库使用操作系统文件锁，仅允许一个写入进程。

library 是受信任管理员维护的素材目录，请勿让不可信本机用户对它具有写入权限。此功能不把任意宿主机文件路径暴露为网站参数。
