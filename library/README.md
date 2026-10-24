# 图标素材库

每个合集直接一个目录：`library/<slug>/`，其中保存 SVG 和 `meta.json`。分类不确定时使用“其他”。这是实际运行素材库，后台修改直接落盘，GitHub 修改在扫描后生效。

- 根级 `icons` 保存主图标版本，`products` 保存产品和网站图标组。
- `category` 为分类；全局分类列表在 `library/categories.json`。
- `tags` / `aliases` / `domains` 用于检索，`parent_brand` 可关联上级合集。
- `is_default` 指定默认版本；每个有活动图标的组恰好一个。
- 文件名为 `<slug>[-<product>][-<version>].svg`，版本 0 省略编号。
- `next_version` 只增加，不因移除文件而回收；新文件的校验值和字节数由扫描重新计算。
- 合集的 `collection_id`、组和图标的 `id` 由程序生成，请保留以稳定详情链接。
- `published: false` 为下架；`deleted: true` 为回收站状态，素材仍保留在目录中。
- 新增合集需同时提供有效 meta.json；已有合集可直接添加符合命名规则的 SVG，扫描自动登记。
- 删除文件会在扫描后移除索引，删除 meta.json 而保留目录会报错。无效扫描不会破坏已有索引。

启动、默认每 5 分钟或后台手动刷新时扫描。不要在 Git 拉取时同时编辑后台；出现外部修改提示后先刷新。

保留 [licenses](licenses/README.md) 中的第三方许可。元数据结构见 [meta.schema.json](meta.schema.json)。运行锁、恢复日志、临时文件不会提交 Git。

普通扫描重读 meta.json，未变化 SVG 复用已校验内容；设置中的“完整扫描”绕过缓存。使用保留修改时间的同步工具后可完整扫描。扫描失败保留上次可用索引，错误与最近成功时间在设置页可见。
