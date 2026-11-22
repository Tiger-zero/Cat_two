# Cat_two

Cat_two 是面向个人的自托管笔记应用。用网页记录 Markdown 笔记和附件，按日期、标签或关键词找回内容。笔记仅限创建者登录后访问。 [产品文档](.agents/skills/cat-two-product-development/references/product-guide.md)

## 部署服务端

将 `compose.yaml` 保存到装有 Docker Compose 的机器上，在文件所在目录运行以下命令。默认从 Docker Hub 使用 `tiger2896/cat_two:latest`。 [部署配置](compose.yaml)

```sh
docker compose up -d
```

打开 `http://服务器地址:5230`，按提示创建管理员账号。默认用 SQLite，数据库、笔记和附件保存在 `cat_two_data` 命名卷。

## 在 NAS 上部署

在 NAS 的 Docker 应用中导入同一份 `compose.yaml`。如需使用 NAS 文件夹，先创建数据目录，再替换 `cat-two` 服务的挂载。

```yaml
volumes:
  - /volume1/docker/cat2/data:/var/opt/memos
```

按目录读写权限调整 `PUID` 和 `PGID`，默认是 `1000` 和 `10`。已有部署在替换配置前先备份并沿用原数据挂载。 [部署与维护指南](.agents/skills/cat-two-product-development/references/deployment.md)

## 项目来源与许可

Cat_two 基于 Memos 开发，采用 MIT 许可证。 [Memos](https://github.com/usememos/memos) · [MIT 许可证](LICENSE)
