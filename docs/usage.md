# 使用指南

[项目介绍](../README.md) · [素材格式与命名规则](library-format.md)

## Docker Compose 部署

下载 [docker-compose.yaml](../docker-compose.yaml)，在文件所在目录执行：

```sh
docker compose up -d
```

访问 [http://127.0.0.1:1089](http://127.0.0.1:1089)，后台地址为 `/admin`。首次启动会向空的 `library/` 填充初始素材，并自动生成管理员密码：

```sh
docker compose exec icon-svg cat /app/data/initial-admin.txt
```

凭据文件第一行为用户名，第二行为密码。登录后可在后台修改密码；自定义初始密码时使用下文的 `ADMIN_PASSWORD`。

Compose 会在同目录创建 `library/` 和 `data/`。已有素材在更新时保留，不会被镜像中的初始素材覆盖。容器以非 root 用户运行，请确保挂载目录可读写。

查看状态及日志：

```sh
docker compose ps
docker compose logs --tail 50 icon-svg
```

更新镜像：

```sh
docker compose pull
docker compose up -d
```

默认使用 `latest`，固定版本可将 Compose 中的镜像改为 `charley008/icon-svg:1.0.8`。更新和迁移前请备份素材与数据，保留原来的两个挂载目录。

需要从源码构建镜像时，在项目根目录执行：

```sh
docker compose -f docker-compose.yaml -f docker-compose.build.yaml up -d --build
```

### HTTPS 与端口

默认只监听宿主机回环地址。通过反向代理提供 HTTPS 时，在 Compose 同目录创建 `.env`：

```dotenv
PUBLIC_ORIGIN=https://icons.example.com
COOKIE_SECURE=true
```

修改后执行 `docker compose up -d`。将示例地址换成实际域名，反向代理转发到 `127.0.0.1:1089`。

修改宿主机端口时，编辑 Compose 的 `ports`，例如 `127.0.0.1:8090:8000`，容器内部端口保持 `8000`。需要局域网直接访问时可改为 `1089:8000`。

### 目录权限

服务默认使用宿主机 `library/` 所有者的 UID/GID；所有者为 root 时，服务 UID 使用 `10001`。已有非空素材目录不会被递归修改权限。

需要手工指定时，在 Compose 的 `environment` 中增加：

```yaml
      PUID: "1000"
      PGID: "1000"
```

请换成实际有权读写挂载目录的用户和组。曾使用命名卷部署的用户，应先导出旧素材和备份数据，再迁移到目录挂载。

## Windows 本地运行

需要 Go 1.26.3 或更高版本。在项目根目录执行：

```powershell
.\build.cmd
.\bin\icon-svg.exe
```

默认打开 [http://127.0.0.1:8000](http://127.0.0.1:8000)，后台地址为 `/admin`。保持控制台运行，按 Ctrl+C 退出。首次随机生成的管理员凭据保存在 `data/initial-admin.txt`。

编译输出位于 `bin/`，运行时使用项目根目录的 `library/`、`web/` 和 `data/`。单独分发 EXE 时，也需提供这三个目录；EXE 不包含素材和网页。正式发布不提供 Windows 预编译包。

开发时也可在项目根目录执行 `go run ./cmd/icon-svg`。

## 配置参数

### 本地配置文件

将 [.env.example](../.env.example) 复制为项目根目录的 `.env`。每行使用 `名称=值`，以 `#` 开头的行为注释：

```dotenv
LISTEN_ADDR=127.0.0.1:8090
OPEN_BROWSER=false
SCAN_INTERVAL=60m
```

本地程序按“命令行参数 → 系统环境变量 → `.env` → 默认值”的顺序取值。程序位于 `bin/` 时从其父目录读取 `.env`；使用 `go run` 时从当前项目目录读取。

### 环境变量

| 变量 | 默认值与写法 | 用途 |
| --- | --- | --- |
| `LISTEN_ADDR` | 本地 `127.0.0.1:8000`，Docker 内 `0.0.0.0:8000` | 程序监听地址；Docker 宿主机端口在 Compose 中修改 |
| `ADMIN_USERNAME` | `admin` | 首次创建管理员时使用 |
| `ADMIN_PASSWORD` | 留空自动生成；自定义为 8–256 位 | 仅首次创建管理员时使用，已有账号请在后台修改密码 |
| `PUBLIC_ORIGIN` | 留空；例如 `https://icons.example.com` | 反向代理后的完整外部地址 |
| `COOKIE_SECURE` | `false`；HTTPS 部署设为 `true` | 仅通过 HTTPS 发送登录 Cookie |
| `OPEN_BROWSER` | Windows 默认 `true`；设为 `false` 关闭 | 启动后自动打开浏览器；Docker 已关闭 |
| `DATA_DIR` | 本地 `data/`，Docker `/app/data` | 账号、设置与运行数据目录 |
| `LIBRARY_DIR` | 本地 `library/`，Docker `/app/library` | SVG 素材库目录 |
| `WEB_DIR` | 本地 `web/`，Docker `/app/web` | 模板和静态文件目录 |
| `SCAN_INTERVAL` | 程序默认 `5m`，示例配置与 Compose 为 `60m` | 扫描间隔，可写 `30m`、`1h`；`0` 关闭定期扫描 |
| `GOMEMLIMIT` | Docker 默认 `128MiB` | Go 软内存目标，不是容器硬上限；一般无需调整 |
| `PUID` / `PGID` | 自动匹配挂载目录所有者 | 仅 Docker 使用，指定服务用户和组 |

Docker Compose 的 `.env` 用于替换 Compose 文件中的变量。默认 Compose 已传入管理员、外部地址、Cookie、扫描间隔和内存配置；其他变量需在服务的 `environment` 中显式添加，仅写入宿主机 `.env` 不会自动传给容器。

### 命令行参数

| 参数 | 示例 | 用途 |
| --- | --- | --- |
| `-listen` | `-listen 127.0.0.1:8090` | 监听地址 |
| `-data` | `-data D:\icon-svg-data` | 数据目录，建议使用绝对路径 |
| `-library` | `-library D:\icon-library` | 素材库目录 |
| `-web` | `-web D:\icon-web` | 网页目录 |
| `-scan-interval` | `-scan-interval 1h` | 初始定期扫描间隔 |
| `-open` | `-open=false` | 是否启动浏览器 |
| `-version` | `-version` | 显示版本后退出 |
| `-healthcheck` | `-healthcheck` | 检查本机 `127.0.0.1:8000/healthz`，用于默认 Docker 健康检查 |

例如，临时调整端口并关闭浏览器：

```powershell
.\bin\icon-svg.exe -listen 127.0.0.1:8090 -open=false
```

后台保存过的扫描间隔优先于环境变量或命令行的初始值；后续请在后台调整。

## 后台操作

### 素材管理

在后台创建合集和产品，上传、替换 SVG，编辑分类、标签和版本说明，并选择默认版本。上传文件会自动命名；每组最多 10 个活动版本。方形 SVG 上传时自动添加圆角。

资料和素材保存在 `library/`，后台修改后立即生效。需要手工编辑时，参阅 [meta.json 字段与命名规则](library-format.md)。

### 永久删除

- 删除版本：移除对应 SVG 文件及资料。
- 删除产品：移除产品及其全部图标版本。
- 删除合集：移除合集目录及其产品、图标；独立子合集保留，并解除上级关联。

永久删除无法从回收站恢复。旧回收站内容仍可手动永久删除，升级不会自动清空。仅需暂时隐藏时，请使用合集或产品的“公开上架”选项。

### 素材扫描

在“设置 → 素材库管理”中设置扫描间隔，单位为分钟，`0` 表示关闭定期扫描。保存后立即生效，重启后保留。关闭定期扫描仍可手动扫描。

外部修改文件后，可点击“扫描变更”。未识别到内容变化或需要重新校验全部素材时，选择“完整扫描”。新产品需先在 `meta.json` 的 `products` 中声明。扫描报错时，修正 JSON、文件名或 SVG 后重新扫描。

不要同时在后台编辑素材与执行 Git 拉取。修改网页模板后需重启程序，更新静态文件后刷新页面即可。

### 顶部导航

在“设置 → 顶部导航”中添加链接，可设置名称、显示状态、打开方式和顺序，最多 12 条。支持完整 HTTP(S) 地址和 `/tools.html` 形式的站内路径，保存后立即生效。

站内链接指向的文件需由反向代理或其他服务提供。

## 导入导出与备份

| 操作 | 包含内容 | 用途 |
| --- | --- | --- |
| 素材 ZIP 导入 | 追加素材并去重，保留已有默认图标 | 增加素材，无需更新镜像 |
| 导出已上架素材 | 公开素材及来源许可 | 分享素材或导入另一安装 |
| 完整备份 | `library/` 和 `data/runtime.sqlite3` | 保留素材、账号、设置、日志和统计 |
| 数据库备份 | 仅运行数据库，不含 SVG | 单独备份账号与运行数据 |

恢复完整备份时，先停止程序，将 `library/` 和 `data/runtime.sqlite3` 还原到对应目录，再启动。Docker 部署可先执行 `docker compose stop`，还原后执行 `docker compose up -d`。

只恢复数据库不会恢复 SVG 文件；只导入素材也不会恢复管理员账号和运行记录。
