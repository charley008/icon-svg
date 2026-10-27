# icon-svg

轻量的 SVG 图标管理与展示网站，使用 Go 和 HTML 模板构建。收录彩色品牌、App、网站及知名厂商 Logo，支持自部署。

## 功能

- 按分类、合集浏览，支持名称、别名、域名和标签检索。
- 多版本预览、默认图标、收藏、SVG / PNG 下载。
- 后台维护合集、产品、分类、标签和图标，支持回收站及导出备份。
- 前后台浅色 / 深色模式。
- SVG 和 meta.json 保存到 library，自动扫描并建立索引；账号、日志和统计保存在 data。
- 支持 Linux AMD64 和 ARM64。
- 1.0.3 提供素材 ZIP 导入/导出，追加素材无需更新镜像；页面显示版本号及 GitHub 链接。

## 快速启动

```sh
docker run -d \
  --name icon-svg \
  --restart unless-stopped \
  -p 127.0.0.1:1089:8000 \
  -v "$(pwd)/library:/app/library" \
  -v "$(pwd)/data:/app/data" \
  charley008/icon-svg:latest
```

访问 http://127.0.0.1:1089/ ，后台为 /admin。需要局域网访问时，将端口映射改为 `1089:8000`，并根据需要配置防火墙或反向代理。

从 1.0.1 起，首次启动会自动创建宿主机当前目录下的 library 和 data，并向空 library 复制初始素材。之后重启或更新镜像保留已有内容，不覆盖修改或恢复已删除素材；初次复制中断可重试。无需到 Docker 内部卷目录查找文件。1.0.0 不具备空目录初始化功能，请先拉取 latest。

首次随机生成的管理员凭据：

```sh
docker exec icon-svg cat /app/data/initial-admin.txt
```

登录后请修改密码。管理员密码最少 8 位，不使用通用默认密码。

## Docker Compose

下面配置可直接用于全新部署，在 Compose 文件旁自动创建并初始化素材目录：

```yaml
services:
  icon-svg:
    image: charley008/icon-svg:latest
    container_name: icon-svg
    restart: always
    ports:
      - "127.0.0.1:1089:8000"
    volumes:
      - ./library:/app/library
      - ./data:/app/data
```

```sh
docker compose up -d
docker compose exec icon-svg cat /app/data/initial-admin.txt
```

已有 library 和 data 目录会继续沿用；如果曾使用命名卷，请先导出旧素材后迁移到目录绑定，不要直接删除旧卷。请确保容器用户对目录有读写权限；可通过 PUID / PGID 指定用户。

## 常用环境变量

| 变量 | 说明 |
| --- | --- |
| ADMIN_USERNAME | 首次管理员用户名，默认 admin |
| ADMIN_PASSWORD | 首次管理员密码；留空自动生成 |
| SCAN_INTERVAL | 素材定期扫描间隔，默认 5m，0 表示关闭定期扫描 |
| PUBLIC_ORIGIN | 反向代理后的外部访问地址 |
| COOKIE_SECURE | 使用 HTTPS 时设为 true |
| PUID / PGID | 可选容器用户 UID / GID |

## 版本与更新

- `1.0.1`：支持宿主机空目录初始化的修正版。
- `latest`：最新正式发布版本。

使用 latest 时执行 `docker compose pull`，然后执行 `docker compose up -d`。固定版本则先修改 image 标签。保留原有 library 和 data 目录，更新镜像不会自动覆盖已有素材。修改素材后可等待自动扫描，或在后台设置中刷新素材库。

## 许可说明

图标与商标归相应权利人所有。各素材来源和许可信息保存在 meta.json 及 /app/library/licenses 中；第三方程序依赖许可见 /app/THIRD_PARTY_NOTICES.txt。

## 1.0.3 素材导入导出

升级程序到 1.0.3 后，在后台“设置 → 素材库管理”导入或导出 ZIP。后续仅添加素材无需更新镜像。已有非空 library 不会被镜像初始素材覆盖；可从 GitHub 对应 Release 下载 `icon-svg-1.0.3-materials.zip`，通过导入补全。已有默认图标、资料和上下架状态保留，重复内容及相同 HTTPS 来源跳过。ZIP 最大 64 MB，仅包含素材和许可文件，不包含账号或数据库。

分类整理包还可补全已有条目的空分类/“其他”，导入时保持对应选项勾选即可。已经设置的具体分类不会被覆盖。注意源码提交不等于已发布镜像，非空的 library 挂载目录也不会随镜像更新自动覆盖；请确认访问域名连接的实例以及其素材目录。
