# icon-svg

使用 Go 构建的自托管 SVG 图标库，由 OpenAI Codex 开发。

## 功能

- 按分类、合集浏览，支持名称、别名、域名和标签搜索。
- 多版本图标预览、默认版本管理与收藏。
- SVG / PNG 下载，支持批量打包。
- 后台管理合集、产品、分类、标签和图标版本。
- SVG 上传、替换和永久删除，支持素材 ZIP 导入导出及数据备份。
- 自定义顶部导航，支持深色和浅色模式。
- 支持 Docker 部署和 Windows 本地运行。

## 文档

- [使用指南](docs/usage.md)：安装部署、配置参数、后台操作和备份恢复。
- [素材格式](docs/library-format.md)：`meta.json` 写法、字段说明和文件命名规则。
- [版本记录](docs/releases/)。

## 许可证

原创代码采用 [MIT License](LICENSE)，第三方依赖见 [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)。图标、Logo 和商标归各自权利人所有，来源及授权见各素材的 `meta.json` 和 [素材许可说明](library/licenses/README.md)。
