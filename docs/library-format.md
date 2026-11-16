# 手工维护素材库

[项目介绍](../README.md) · [使用指南](usage.md)

每个合集对应 `library/` 下的一个目录，目录名就是合集标识。一个 `meta.json` 同时保存合集主图标和旗下产品资料，不需要给每个产品再建子目录。完整字段结构见 [meta.schema.json](../library/meta.schema.json)。

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
| `category` | 使用 [categories.json](../library/categories.json) 中的分类名；不确定就用“其他”，不要随意新增近义分类。分类可在后台设置中维护。 |
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

`published` 和 `deleted` 可省略，默认正常上架。需要下架时在合集或产品对象中设置 `"published": false`；`"deleted": true` 兼容旧回收站状态，不等于删除 SVG 文件。后台“永久删除”会直接移除文件及元数据，删除前会提示确认。

### 添加或修改后的操作

1. 保存为 UTF-8 编码的标准 JSON，不写注释、不使用尾随逗号；布尔值用 `true` / `false`，不是 `1` / `0` 或字符串。
2. 把对应 SVG 放进同一个合集目录。新产品先在 `products` 中声明，再放入符合命名的文件。
3. 在后台设置中点击“扫描变更”，或等待定期扫描。已有图标组中新加的规范命名 SVG 会自动登记；新建合集仍需要先有 `meta.json`。
4. 如扫描报错，检查 JSON、文件名和默认版本；扫描失败会保留上次可用索引。扫描时不要同时在后台编辑同一份资料。
