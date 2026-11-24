# Cat_two 安全说明

## 运行版本

部署前用 `/api/instance/profile` 核对应用版本和提交，确认镜像含所需修复。通过 HTTPS 代理访问，先在受限网络完成管理员初始化。保留应用限流，并用反向代理限制公网流量和保护后端端口。 [公网配置](.agents/skills/cat-two-product-development/references/deployment.md#在公网部署)

## 外部访问

Web 登录、REST 个人访问令牌和 MCP 连接令牌分别管理。退出、撤销或因 45 分钟无编辑而到期的浏览器会话，其访问和刷新凭据均失效。网页由 CSP 限制脚本来源，旧实例的自定义 JavaScript 已停用。MCP 可选择只读或读写令牌，使用前会核对权限及到期时间。笔记、ZIP 归档和附件读取核对作者权限。企业微信由回调签名、消息加密及配置的发件人共同验证，Telegram 限于指定用户与 Bot 的私聊。 [外部接口和凭据](.agents/skills/cat-two-product-development/references/external-api.md#凭据和可见性)

连续 6 次密码验证失败会持久封禁来源 IP 的新登录。管理员在安全设置中解除，部署主机也可解除单个地址。反向代理应先正确配置真实客户端 IP，再开放公网登录。 [登录防护](.agents/skills/cat-two-product-development/references/deployment.md#登录防护和-ip-解封)

## 安全问题报告

仓库目前未启用 GitHub 私密漏洞报告，也未公布其他私密报告入口。已有维护者私密联系方式时，通过该渠道提交。漏洞详情请保留在私密沟通中。 [Cat_two Security](https://github.com/Tiger-zero/Cat_two/security)

报告请附上受影响的镜像标签或提交、部署方式、影响范围和验证信息。发送前移除令牌、Cookie、回调密钥及笔记内容。
