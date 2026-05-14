# Frontend Auth

All three frontend apps use the same browser SSO flow:

1. The login page builds an authorize URL from `VITE_SSO_AUTHORIZE_URL`, `VITE_SSO_CLIENT_ID`, and `VITE_SSO_CLIENT_TYPE`.
2. The browser is redirected to the URM authorize endpoint.
3. URM completes login or reuses the existing SSO session.
4. URM redirects back to `/oauth/callback` with `code` and `state`.
5. The callback page exchanges the code for tokens through the backend URM exchange API.
6. The frontend stores tokens, fetches `/urm/oauth2/userinfo`, and then enters the app.

## Required Environment Variables

- `VITE_SSO_AUTHORIZE_URL`
- `VITE_SSO_CLIENT_ID`
- `VITE_SSO_CLIENT_TYPE`

Each app keeps its own API base URL and UI title settings, but SSO variables follow the same naming and behavior.

## Rules

- Do not use the old `/api/auth/callback` flow.
- Do not introduce `VITE_URM_*` aliases again.
- Login pages should redirect the browser to URM directly instead of opening an embedded login form.
- Refresh token requests must continue sending the client identifier expected by URM.
- Logout clears local auth state first, then calls the URM logout endpoint when configured.

## Storage Model

Each app stores its auth state under its own namespace so admin, tenant, and customer sessions do not overwrite each other:

- `admin_*`
- `tenant_*`
- `customer_*`
