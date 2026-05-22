# ai-admin

Uni AI API 管理员后台。

## 启动

```bash
bun install
bun run dev
# http://localhost:13011
```

## 环境变量

- `VITE_APP_TITLE`
- `VITE_API_BASE_URL`
- `VITE_URM_MANIFEST_URL`
- `VITE_SSO_AUTHORIZE_URL`
- `VITE_SSO_CLIENT_ID`
- `VITE_SSO_CLIENT_TYPE=admin`

## 主要功能

- ✅ AI 网关管理（供应商、端点、模型、部署）
- ✅ 模型路由与定价
- ✅ 凭证池管理
- ✅ 速率限制配置
- ✅ API Key 管理
- ✅ 租户模型授权
- ✅ 用量统计
- ✅ 访问审计
- ✅ SSO 授权登录
- ✅ Token 自动刷新

## 技术栈

- Vue 3 + Vite
- Element Plus
- Tailwind CSS
- Pinia
- Vue Router
- Axios
