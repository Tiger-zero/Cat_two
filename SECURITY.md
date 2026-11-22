# Cat_two 安全说明

## 运行版本

部署前用 `/api/instance/profile` 核对应用版本和提交，确认镜像含所需修复。通过 HTTPS 代理访问，先在受限网络完成管理员初始化。保留应用限流，并用反向代理限制公网流量和保护后端端口。 [公网配置](README.md#在公网部署)

## 外部访问

Web 登录、REST 个人访问令牌和 MCP 连接令牌分别管理。笔记、ZIP 归档和附件读取核对作者权限。各集成使用可单独撤销的令牌。企业微信由回调签名、消息加密及配置的发件人共同验证，Telegram 限于指定用户与 Bot 的私聊。 [外部接口和凭据](.agents/skills/cat-two-development/references/external-api.md)

## 安全问题报告

通过私有渠道联系 Cat_two 仓库维护者。附上受影响镜像或提交、部署方式、影响范围和验证信息。发送前移除令牌、Cookie、回调密钥及笔记内容。
