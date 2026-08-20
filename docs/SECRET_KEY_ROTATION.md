# 敏感配置密钥管理

D-AI 使用 envelope-style 的进程内 AES-GCM 加密保护 JWT 私钥、Provider/OAuth 凭证、微信支付密钥、MFA 和代理密码。数据库只保存密文；新密文格式为：

```text
enc:v1:<key-id>:<nonce>:<ciphertext>
```

`key-id` 只表示版本，不包含密钥材料。`secret_master_key` 是当前 active key，`secret_master_key_id` 是版本号，`secret_master_key_previous` 保存轮换宽限期内的旧值，格式为 `old-id=old-key`，多个值可用逗号或空格分隔。

## 轮换流程

1. 生成新的 16/24/32 字节随机密钥（或其标准 Base64 表示），设置新的 `DAI_SECURITY_SECRET_MASTER_KEY_ID` 和 `DAI_SECURITY_SECRET_MASTER_KEY`。
2. 在同一发布中把旧的 `id=key` 放入 `DAI_SECURITY_SECRET_MASTER_KEY_PREVIOUS`，先滚动重启所有实例。
3. 新写入使用 active key；JWT 私钥、OAuth 凭证和微信支付配置在读取成功后会按 active key 做在线重新加密。
4. 观察解密失败、认证失败和凭证调用指标，确认旧密文已完成迁移后，从 `previous` 删除旧值并再次滚动重启。

应用启动会校验生产环境的 active/previous key ID、重复版本和密钥长度。缺少 active key 或无法构造 keyring 时直接拒绝启动。旧密钥删除过早会使仍未迁移的密文永久无法恢复，因此旧值必须在所有实例和数据迁移完成前保留。

## 兼容与恢复

升级期间仍可读取历史 `v1:` clientsecret 和 `aesgcm:v1:` Provider 密文，读取成功后会迁移到统一格式。JWT 历史明文 PEM 仅在启动加载时接受一次并立即覆盖为密文；无法解密或解析的签名密钥会阻止启动，避免静默使用不完整密钥集。

密钥材料只应通过受保护的环境变量、Secret Manager 或部署编排系统注入，不要提交到仓库、日志或管理接口。若 active 和所有 previous key 均丢失，数据库中的密文无法恢复，只能按业务流程重新配置对应凭证并重新生成 JWT 签名密钥。
