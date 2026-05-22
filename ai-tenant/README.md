# ai-tenant

Uni AI API 租户门户。

## 启动

```bash
bun install
bun run dev
# http://localhost:13012
```

## 环境变量

- `VITE_APP_TITLE`
- `VITE_API_BASE_URL`
- `VITE_URM_MANIFEST_URL`
- `VITE_SSO_AUTHORIZE_URL`
- `VITE_SSO_CLIENT_ID`
- `VITE_SSO_CLIENT_TYPE=tenant`

## 主要功能

- ✅ AI 路由浏览
- ✅ API Key 管理
- ✅ 模型授权查看
- ✅ 用量与消费统计
- ✅ 余额查询
- ✅ 访问日志
- ✅ SSO 授权登录
- ✅ Token 自动刷新

## 技术栈

- Vue 3 + Vite
- Element Plus
- Tailwind CSS
- Pinia
- Vue Router
- Axios
