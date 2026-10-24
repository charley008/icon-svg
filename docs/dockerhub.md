# icon-svg

轻量的 SVG 图标管理与展示网站，使用 Go 和 HTML 模板构建。收录彩色品牌、App、网站及知名厂商 Logo，支持自部署。

## 功能

- 按分类、合集浏览，支持名称、别名、域名和标签检索。
- 多版本预览、默认图标、收藏、SVG / PNG 下载。
- 后台维护合集、产品、分类、标签和图标，支持回收站及导出备份。
- 前后台浅色 / 深色模式。
- SVG 和 meta.json 保存到 library，自动扫描并建立索引；账号、日志和统计保存在 data。
- 支持 Linux AMD64 和 ARM64。

## 快速启动

```sh
docker run -d \
  --name icon-svg \
  --restart unless-stopped \
  -p 127.0.0.1:8088:8000 \
  -v icon-svg-library:/app/library \
  -v icon-svg-data:/app/data \
  charley008/icon-svg:latest
```

访问 http://127.0.0.1:8088/ ，后台为 /admin。需要局域网访问时，将端口映射改为 `8088:8000`，并根据需要配置防火墙或反向代理。

Docker 命名卷在首次创建时会自动复制镜像内的初始素材。更新镜像后继续保留已有素材、账号及运行数据。不要用空的宿主目录覆盖 /app/library；使用目录绑定时，请先准备完整的 library 文件夹。

首次随机生成的管理员凭据：

```sh
docker exec icon-svg cat /app/data/initial-admin.txt
```

登录后请修改密码。管理员密码最少 8 位，不使用通用默认密码。

## Docker Compose

下面配置可直接用于全新部署，使用命名卷自动初始化素材：

```yaml
services:
  icon-svg:
    image: charley008/icon-svg:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8088:8000"
    volumes:
      - icon-svg-library:/app/library
      - icon-svg-data:/app/data
volumes:
  icon-svg-library:
  icon-svg-data:
```

```sh
docker compose up -d
docker compose exec icon-svg cat /app/data/initial-admin.txt
```

如已拥有完整 library，可将素材挂载换成 `./library:/app/library`，数据挂载换成 `./data:/app/data`。请确保容器用户对目录有读写权限；可通过 PUID / PGID 指定用户。

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

- `1.0.0`：固定版本。
- `latest`：最新正式发布版本。

使用 latest 时执行 `docker compose pull`，然后执行 `docker compose up -d`。固定版本则先修改 image 标签。保留原有素材卷和数据卷，更新镜像不会自动覆盖已有素材。修改素材后可等待自动扫描，或在后台设置中刷新素材库。

## 许可说明

图标与商标归相应权利人所有。各素材来源和许可信息保存在 meta.json 及 /app/library/licenses 中；第三方程序依赖许可见 /app/THIRD_PARTY_NOTICES.txt。
