# icon-svg

本项目由 OpenAI Codex 开发。

使用 Go 和 HTML 模板构建的图标库。`library/` 中的 SVG 和 `meta.json` 是素材的唯一来源；SQLite 仅作为内存查询索引，以及账号、日志、下载统计的持久化存储。页面与静态文件从外部 `web/` 读取。

- 按分类或合集浏览，支持名称、别名、域名及标签检索。
- 图标版本预览、默认版本、收藏、SVG / PNG 下载和批量打包。
- 后台维护合集、产品、分类、标签、上传、回收站及默认图标。
- 设置按需展示素材库管理、数据库备份、账号安全、分类管理和操作日志。
- 素材库 ZIP 导入/导出：追加去重、保留默认图标、导入后立即生效，无需重新拉取镜像。
- 页面底部显示程序版本，右上角提供项目 GitHub 链接。
- 前后台深色/浅色模式，英文在前、中文按拼音排序。

## 初始数据

1.0.3 的 library 包含 **2723 个合集、425 个产品、5132 个 SVG、23 个分类**（含回收站记录），保留当前已上架内容、默认版本、分类、标签和版本序号。

同一品牌的文字、深浅色等图形合并为版本；相关服务归入品牌下的产品。合并后的旧详情链接会永久跳转，原默认版本及素材 ID 保留。

启动时扫描文件库并重建内存索引，之后默认每 5 分钟扫描一次；设置中也可手动刷新。后台修改直接更新素材文件和索引。全新安装的操作日志、下载次数和会话为空，管理员账号独立生成；仓库不包含数据库或凭据。

## Windows

可按下文从源码编译 1.0.3，或下载已经发布的对应版本的 [Release 完整 ZIP](https://github.com/charley008/icon-svg/releases)，完整解压到可写目录后双击 `icon-svg.exe`。无需安装 Go、Python、Node.js；请保留 `library` 和 `web`，EXE 不包含这些文件。

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

编译后也可直接双击 `bin/icon-svg.exe`，默认打开 <http://127.0.0.1:8000/>，后台地址 `/admin`。保持控制台窗口运行，按 Ctrl+C 退出。

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

默认拉取 [charley008/icon-svg](https://hub.docker.com/r/charley008/icon-svg) 的 `latest` 镜像，无需本地编译。需要固定版本时，直接修改 Compose 中的 `image` 标签。Compose 默认地址为 <http://127.0.0.1:1089/>。素材通过宿主机 `./library` 目录挂载到 `/app/library`，镜像入口会在首次启动时向空目录复制初始素材；账号与运行数据仍通过宿主机 `./data` 挂载到 `/app/data`。容器名为 `icon-svg`，重启策略为 `always`，Compose 默认每 60 分钟扫描素材。服务以非 root 用户运行，默认使用 library 挂载目录所有者的 UID/GID（UID 为 0 时使用 10001）；也可设置 `PUID` / `PGID`。请确保该用户对素材目录有读写权限。入口调整 data 根目录归属；初始化空素材目录时也会设置该目录归属，已有非空素材目录不会被递归改权限。模板和静态文件位于镜像内的 `/app/web`；如需直接维护，也可添加 `./web:/app/web:ro` 映射。

查看首次凭据：

```sh
docker compose exec icon-svg cat /app/data/initial-admin.txt
```

更新镜像时运行 `docker compose pull`，再执行 `docker compose up -d`。`docker compose down` 保留宿主机的 `./library` 和 `./data`。更新时保留这两个目录；素材变动在扫描后生效，不需重新初始化数据库。

首次部署只需 `docker-compose.yaml`，无需手动创建 `library` 或下载初始素材；Docker 会创建 Compose 旁的 `./library` 和 `./data`，入口自动填充空素材目录。再次启动或更新镜像会保留已有素材，不会重新覆盖。复制中断后下次启动会继续补齐初始文件。此功能从 1.0.1 起提供，请先拉取最新镜像。

**如果已经使用命名卷部署**，先在后台导出素材，切换为目录绑定后再导入；不要直接删除旧卷。原本使用 `./library` 的站点继续沿用即可。已有管理员数据请继续挂载原来的 `./data`。

反向代理后的 HTTPS 部署请在 `.env` 中设置 `PUBLIC_ORIGIN=https://你的域名`、`COOKIE_SECURE=true`。默认端口仅监听宿主机回环地址。

如需自行从源码构建：

```sh
docker compose -f docker-compose.yaml -f docker-compose.build.yaml up -d --build
```

支持在 WSL2 中运行 Docker Engine 和 Compose。在项目目录内执行上面的 Compose 命令即可。

不下载源码或初始素材时，也可直接运行镜像，并在宿主机当前目录自动填充初始素材：

```sh
docker run -d --name icon-svg --restart unless-stopped -p 127.0.0.1:1089:8000 -v "$(pwd)/library:/app/library" -v "$(pwd)/data:/app/data" charley008/icon-svg:latest
```

首次启动时素材会复制到当前目录的 `library/`；后续更新镜像保留已有素材和账号数据。不要删除初始化标记文件（`.icon-svg-initialized`）。`docker-compose.build.yaml` 提供可选的本地构建覆盖配置。


## 配置

详见 [.env.example](.env.example)。

| 配置 | 用途 |
| --- | --- |
| `LISTEN_ADDR` | 本地程序监听地址，默认 `127.0.0.1:8000` |
| `DATA_DIR` | 数据目录，可选绝对路径 |
| `OPEN_BROWSER` | 是否自动打开浏览器 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | 仅创建第一个管理员时使用；密码为空则自动生成 |
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

## 后台素材库管理

设置中“素材库管理”与“数据库备份”独立：

- **扫描设置**：输入分钟数，`0` 关闭定期扫描，范围 0–43200（30 天）。保存后立即生效并写入运行数据库，重启后仍保留，优先于环境变量或启动参数的初始间隔。
- **手动扫描**：可扫描变更或完整扫描。复制 SVG 到已有合集后扫描即可登记，新产品仍需先在 `meta.json` 声明。
- **新增预览**：右侧显示最近一次扫描发现的新增 SVG，最多 60 个缩略图，点击可进入对应合集管理。没有变化的例行扫描保留上一批结果；下一批新增替换，重启程序后预览清空。扫描失败不发布错误素材。
- **素材导出**：只导出上架素材及来源许可。
- **数据库备份**：独立导出账号、日志、下载统计和扫描设置，不包含 SVG；恢复时需同时保留素材目录。

## 手工编写 meta.json

每个合集对应 `library/` 下的一个目录，目录名就是合集标识。一个 `meta.json` 同时保存合集主图标和旗下产品资料，不需要给每个产品再建子目录。完整字段结构见 [meta.schema.json](library/meta.schema.json)。

以 **Google 合集，两个主图标版本和一个 Gmail 产品图标** 为例：

```text
library/
└── google/
    ├── meta.json
    ├── google.svg          主图标，版本 0
    ├── google-1.svg        主图标，版本 1
    └── google-gmail.svg    Gmail 图标，版本 0
```

下面是这个示例的完整 `meta.json`。这是格式示例，不是现有 Google 资料的完整导出；请勿直接覆盖已有文件。

```json
{
  "schema_version": 1,
  "title": "Google",
  "url": "https://www.google.com/",
  "domains": ["google.com"],
  "description": "Google 搜索及旗下产品图标。",
  "category": "搜索导航",
  "tags": ["搜索", "谷歌"],
  "aliases": ["Google", "谷歌"],
  "notes": [],
  "icons": [
    {
      "file": "google.svg",
      "version": 0,
      "label": "彩色版",
      "is_default": true,
      "source": "",
      "license": "许可待核对，商标归相应权利人"
    },
    {
      "file": "google-1.svg",
      "version": 1,
      "label": "备选版",
      "is_default": false,
      "source": "",
      "license": "许可待核对，商标归相应权利人"
    }
  ],
  "next_version": 2,
  "products": [
    {
      "key": "gmail",
      "title": "Gmail",
      "url": "https://mail.google.com/",
      "domains": ["mail.google.com", "gmail.com"],
      "description": "Google 旗下的电子邮件服务。",
      "category": "办公效率",
      "tags": ["邮箱", "邮件", "Google"],
      "aliases": ["谷歌邮箱"],
      "notes": [],
      "icons": [
        {
          "file": "google-gmail.svg",
          "version": 0,
          "label": "彩色版",
          "is_default": true,
          "source": "",
          "license": "许可待核对，商标归相应权利人"
        }
      ],
      "next_version": 1
    }
  ]
}
```

### 字段说明

| 字段 | 写法与用途 |
| --- | --- |
| `schema_version` | 固定填写数字 `1`。 |
| `title` | 显示名称，可以用中文或英文。 |
| `url` / `domains` | `url` 为完整网站地址（`https://…`），没有时可填空字符串；`domains` 为域名数组，不带协议和路径，用于检索。 |
| `description` | 简短说明，没有时填写 `""`。 |
| `category` | 使用 [categories.json](library/categories.json) 中的分类名；不确定就用“其他”，不要随意新增近义分类。分类可在后台设置中维护。 |
| `tags` / `aliases` | 字符串数组：标签写用途、行业等关键词，别名写中文名、英文名或常用简称，方便检索；没有时填写 `[]`。 |
| `notes` | 备注字符串数组，没有时填写 `[]`。 |
| `icons` | 当前图标组的版本列表。根级表示合集主图标，产品内表示该产品的图标。 |
| `next_version` | 这个图标组下一次上传使用的编号，必须大于已有最大 `version`。只增加，不因删除版本而回退；从未有图标的组可填 `0`。 |
| `products` | 旗下产品或网站的数组。没有产品时直接填写 `[]`。产品有自己的名称、网址、分类、标签和图标版本，不会自动继承根级资料。 |
| 产品的 `key` | 产品标识，例如 `gmail`，在同一合集内唯一，用于组成 SVG 文件名。 |
| 图标的 `file` / `version` | 同目录下的 SVG 文件名与数字版本号，必须符合下方命名规则。不要填写绝对路径或子目录。 |
| 图标的 `label` | 版本说明，例如“彩色版”“深色背景版”，不是文件名。 |
| 图标的 `is_default` | 布尔值 `true` 或 `false`。每个有活动图标的组应恰好有一个默认版本。 |
| 图标的 `source` / `license` | 填写实际素材来源和使用许可。示例中的来源留空、许可待核对只是占位，请补充真实信息，不要推定所有 Logo 都可自由商用。 |

### 文件名和版本规则

- 合集目录名、产品 `key` 使用小写英文字母、数字，以单个 `-` 或 `.` 分隔，最多 80 个字符；例如 `google`、`app-store`。不能包含空格、中文、斜杠或 `..`，也不能使用 Windows 保留名（如 `con`、`nul`）。**中文名称放在 `title` 或 `aliases` 中。**
- 主图标：`google.svg` 对应 `version: 0`，`google-1.svg` 对应 `version: 1`，以此类推。
- 产品图标：`google-gmail.svg` 对应 Gmail 的 `version: 0`；`google-gmail-1.svg` 对应 Gmail 的 `version: 1`。
- 各图标组独立计数。示例中 Google 下次上传为 `google-2.svg`，Gmail 下次上传为 `google-gmail-1.svg`。
- 默认图标只看 `is_default`。可以让 `google-1.svg` 成为默认，将它设为 `true`、原默认设为 `false` 即可，无需改文件名。
- 根级没有主图标、只有产品时，可以使用 `icons: []` 和 `next_version: 0`；没有产品则使用 `products: []`。

### 哪些字段不用手填

新建资料时可以省略 `collection_id`、所有 `id`、图标的 `sha256` 和 `bytes`，程序扫描后会生成或计算。已有资料中的 ID 请保留，不要复制给新合集或新图标，以免重复或改变详情链接。

`parent_brand` 可选，用于关联另一个已存在的合集目录名，例如 QQ 合集中填写 `"parent_brand": "tencent"`；不能指向自己，也不能形成循环。没有上级合集就省略此字段。

`published` 和 `deleted` 可省略，默认正常上架。需要下架时在合集或产品对象中设置 `"published": false`；`"deleted": true` 表示回收站状态，不等于删除 SVG 文件。日常操作建议通过后台完成。

### 添加或修改后的操作

1. 保存为 UTF-8 编码的标准 JSON，不写注释、不使用尾随逗号；布尔值用 `true` / `false`，不是 `1` / `0` 或字符串。
2. 把对应 SVG 放进同一个合集目录。新产品先在 `products` 中声明，再放入符合命名的文件。
3. 在后台设置中点击“扫描变更”，或等待定期扫描。已有图标组中新加的规范命名 SVG 会自动登记；新建合集仍需要先有 `meta.json`。
4. 如扫描报错，检查 JSON、文件名和默认版本；扫描失败会保留上次可用索引。扫描时不要同时在后台编辑同一份资料。

## 文件写入安全

合集及产品标识只允许小写 ASCII 字母、数字、单个点或连字符分隔；拒绝路径分隔符、绝对路径、Windows 设备名和保留目录名。上传文件名不作为落盘路径。

所有素材操作通过 `os.Root` 限定在 library 内，扫描拒绝符号链接及特殊文件；写入使用随机排他临时文件、同步和原子替换，不截断目标文件。多文件保存使用恢复日志，进程中断后下次启动继续恢复。单个素材库使用操作系统文件锁，仅允许一个写入进程。

library 是受信任管理员维护的素材目录，请勿让不可信本机用户对它具有写入权限。此功能不把任意宿主机文件路径暴露为网站参数。

## 许可证与图标声明

项目原创代码采用 [MIT License](LICENSE)。第三方依赖仍遵循其各自许可证，详见 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)。

收录的图标、Logo 和商标归各自权利人所有，不因项目代码采用 MIT 而改变其授权范围。素材来源及许可见各 `meta.json` 和 [library/licenses](library/licenses/README.md)。

如有图标涉及侵权，请联系[项目维护者](https://github.com/charley008)，我们会及时核实并删除。

## 1.0.3：素材迁移与补全

后台 → 设置 → 素材库管理 → 导入与导出：

- **导出完整素材库**：包含 `library/<合集>/meta.json`、SVG、分类和许可文件；包含未上架及回收站素材，不包含账号、会话、密码和数据库。
- **导出已上架素材**：只包含当前可公开使用的素材，同样可以导入。
- **导入素材库**：选择本项目导出的 ZIP（最大 64 MB，解压总大小最大 256 MB）。支持带 `library/` 前缀或直接以合集目录开头的结构。先校验再写入，拒绝无效 SVG、目录穿越、符号链接、重复路径和数据库文件。
- 按合集和产品标识追加；相同内容或相同 HTTPS 来源跳过，文件名冲突时分配新版本；保留目标库已有资料、默认版本、ID、上下架及回收站状态。勾选“补全其他分类”时，仅把空分类或“其他”补成素材包提供的具体分类，已有具体分类保持不变。新导入的 ID 自动分配，避免与其他安装冲突。
- 许可文件一同保存；同名但内容不同的许可文件保留为带内容哈希后缀的副本。导入成功后刷新索引和新增预览，重启后仍然保留。导入属于合并追加，不是删除后完整还原；账号与数据库备份仍在单独的“数据管理”中。

Docker 继续挂载 `./library:/app/library` 和 `./data:/app/data`。首次升级程序到 1.0.3 后，后续仅添加素材可直接导入 ZIP，不需要再拉取镜像。已有非空素材目录不会被镜像内置素材覆盖，可下载对应 Release 的 `icon-svg-1.0.3-materials.zip`，通过导入补全。本地生成的素材包位于 `dist/icon-svg-1.0.3-materials.zip`。

本次核对 Dashboard Icons 上游完整 SVG 清单，新增 3212 个文件，跳过 304 个已有来源和 84 个重复内容；99 个非矢量、无效文件或不支持的内容未导入。原有默认图标保持不变。React 使用 `react` 合集并提供 `reactjs`、`react.js` 别名；操作系统放在“系统工具”，框架放在“开发编程”。详细来源、目标路径和排除原因见 [导入报告](docs/dashboard-icons-import.json)。

维护者可在本地完整 upstream checkout 上重跑：

```powershell
python .github/scripts/sync-dashboard-icons.py D:\path\to\dashboard-icons --report D:\path\to\import-report.json
```

脚本按来源、精确名称/别名和文件 SHA256 去重，不覆盖现有文件或默认图标。新增图形仅清除编辑器元数据、无用远程字体和非显示脚本；保留矢量几何与颜色。提交前必须通过 `go test ./...` 中的完整 SVG 校验。

## 分类整理与 Debian 检索说明

1.0.3 已将原“其他”中的 **1796 个条目**按用途归入现有 23 个分类，“其他”从 **1799 项降至 3 项**。余下 Gostiklogo、Iba、Quest 的原始元数据缺少明确用途，暂保留待核对。分类没有修改 SVG、默认版本、ID、上下架状态或其他资料。完整前后统计和逐条依据见 [分类报告](docs/library-classification-report.json)。

规则结合 Dashboard Icons 的 `metadata.json`、旧版 `meta/` 分类、明确的产品用途和同产品变体；大小写、横线及 wordmark/light/dark 等形式会统一识别。以后同步素材也使用同一套规则。需要复跑时：

```powershell
python .github/scripts/classify-library.py --upstream D:\path\to\dashboard-icons --report D:\path\to\classification-preview.json
python .github/scripts/classify-library.py --upstream D:\path\to\dashboard-icons --report D:\path\to\classification-report.json --apply --backup D:\path\to\metadata-backup
```

已有实例导入本次素材包时，可以勾选“用素材包补全已有条目的其他分类”。即使图标被判定为重复而跳过，空分类/“其他”仍会补全；你已设置的具体分类、默认图标和其他资料不变。

Debian 官方标志由提交 `6c0dcafe67216a0be01390e8c7a02070c37d54cf` 添加，仍是 `library/debian/debian.svg` 中的默认版本，可搜索 `debian`、`Debian GNU/Linux` 或 `蝶变`。该提交晚于 `v1.0.2` 标签，不能仅凭版本号推断部署实例已包含这次素材变更。程序版本、GitHub 源码和实例挂载的 library 内容各自独立；访问线上域名时看到的是线上实例。

## 2026-10-27 用户提供的操作系统 SVG

追加 11 个备选版本：Red Hat 2 个、Linux 2 个、Debian 7 个，均位于“系统工具”。三个合集原有默认图标不变，文件名按各合集现有序号继续分配；保留原始文件名和 SHA256 来源记录。Linux 单色 SVG 移除了外部 DOCTYPE 声明，一份 Debian 发行版图标移除了非显示的 Inkscape 元数据，图形、颜色、渐变、比例不变。其余 9 个 SVG 保留原始字节。

本地素材总数为 5048。新增素材专项包为 `dist/icon-svg-os-additions-20261027.zip`（包含这三个合集的完整元数据和全部版本，便于安全合并），完整素材包仍为 `dist/icon-svg-1.0.3-materials.zip`。版本说明和文件映射见 [添加记录](docs/user-os-assets-20261027.json)。

## 1.0.3 页面与 Copilot 整理

前后台共用同一个页脚，统一显示 logo、项目名、版本与许可链接；内容较少时页脚位于窗口底部，内容较长时跟随正文，手机端自动换行。已移除顶部“为你的下一个项目，找个好图标”和底部“每一个小图标，都值得好好挑选。”两句文案。

Microsoft Copilot 统一位于 Microsoft 合集的 `copilot` 产品下，保留原默认图标及 ID 361，Dashboard Icons 版本作为第 6 个备选，原素材 ID 3379 保留。独立的 `microsoft-copilot` 合集已合并移出素材目录，旧详情 `/detail/2048` 永久跳转至 `/detail/361`。合并记录见 [Copilot 整理报告](docs/copilot-merge-20261027.json)。同步脚本优先匹配已有的“合集-产品”完整标识。

维护者可在分组的 `redirect_from` 数组记录本实例退役的详情 ID；这些 ID 会参与冲突检查和新 ID 分配。它们只用于当前实例的旧链接兼容，素材包导入会丢弃外来跳转 ID，避免与目标实例的 ID 冲突。
