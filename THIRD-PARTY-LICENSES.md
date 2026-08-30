# Third-Party Licenses

This file summarizes direct third-party dependencies used by distributed
builds of D-AI. Keep it with Docker images, standalone binaries and frontend
bundles.

Scope: direct dependencies from `go.mod` and `package.json`. Transitive
dependencies and bundled assets must be audited before each external binary
release. The license files shipped by each dependency are authoritative.

## Backend

| Scope | Dependency | Version | License |
|---|---|---:|---|
| test | `github.com/alicebob/miniredis/v2` | `v2.38.0` | MIT |
| production | `github.com/cloudflare/ahocorasick` | `v0.0.0-20240916140611-054963ec9396` | BSD-3-Clause |
| production | `github.com/danielgtaylor/huma/v2` | `v2.38.0` | MIT |
| production | `github.com/gen2brain/webp` | `v0.6.4` | MIT |
| production | `github.com/go-chi/chi/v5` | `v5.3.0` | MIT |
| production | `github.com/go-chi/cors` | `v1.2.2` | MIT |
| production | `github.com/golang-jwt/jwt/v5` | `v5.3.1` | MIT |
| production | `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause |
| production | `github.com/jackc/pgx/v5` | `v5.10.0` | MIT |
| production | `github.com/mozillazg/go-pinyin` | `v0.21.0` | MIT |
| production | `github.com/prometheus/client_golang` | `v1.23.2` | Apache-2.0 |
| production | `github.com/redis/go-redis/v9` | `v9.18.0` | BSD-2-Clause |
| production | `github.com/spf13/viper` | `v1.21.0` | MIT |
| production | `github.com/wechatpay-apiv3/wechatpay-go` | `v0.2.21` | Apache-2.0 |
| production | `go.opentelemetry.io/otel` | `v1.43.0` | Apache-2.0 |
| production | `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | `v1.43.0` | Apache-2.0 |
| production | `go.opentelemetry.io/otel/sdk` | `v1.43.0` | Apache-2.0 |
| production | `go.opentelemetry.io/otel/trace` | `v1.43.0` | Apache-2.0 |
| production | `go.uber.org/zap` | `v1.28.0` | MIT |
| production | `golang.org/x/crypto` | `v0.50.0` | BSD-3-Clause |
| production | `golang.org/x/net` | `v0.53.0` | BSD-3-Clause |
| production | `golang.org/x/sync` | `v0.20.0` | BSD-3-Clause |
| production | `golang.org/x/text` | `v0.36.0` | BSD-3-Clause |
| production | `gopkg.in/natefinch/lumberjack.v2` | `v2.2.1` | MIT |

## Portal

| Scope | Dependency | Resolved version | License |
|---|---|---:|---|
| development | `@axe-core/playwright` | `4.10.2` | MPL-2.0 |
| development | `@playwright/test` | `1.55.1` | Apache-2.0 |
| production | `@element-plus/icons-vue` | `2.3.2` | MIT |
| production | `dompurify` | `3.4.13` | Apache-2.0 OR MPL-2.0 |
| production | `echarts` | `6.1.0` | Apache-2.0 |
| production | `element-plus` | `2.14.3` | MIT |
| production | `lucide-vue-next` | `1.0.0` | ISC |
| production | `marked` | `18.0.9` | MIT |
| production | `pinia` | `3.0.4` | MIT |
| production | `qrcode` | `1.5.4` | MIT |
| production | `tailwindcss` | `4.3.3` | MIT |
| production | `vue` | `3.5.40` | MIT |
| production | `vue-router` | `5.2.0` | MIT |
| production | `vue-sonner` | `2.0.9` | MIT |
| development | `@tailwindcss/vite` | `4.3.3` | MIT |
| development | `@types/node` | `24.13.3` | MIT |
| development | `@types/qrcode` | `1.5.6` | MIT |
| development | `@vitejs/plugin-vue` | `6.0.8` | MIT |
| development | `@vue/compiler-sfc` | `3.5.39` | MIT |
| development | `@vue/test-utils` | `2.4.6` | MIT |
| development | `@vue/tsconfig` | `0.7.0` | MIT |
| development | `happy-dom` | `20.8.9` | MIT |
| development | `openapi-typescript` | `7.10.1` | MIT |
| development | `postcss` | `8.5.25` | MIT |
| development | `typescript` | `6.0.3` | Apache-2.0 |
| development | `unplugin-vue-components` | `32.1.0` | MIT |
| development | `vite` | `8.2.0` | MIT |
| development | `vitest` | `4.1.0` | MIT |
| development | `vue-tsc` | `3.3.9` | MIT |

## Distribution Notes

- DOMPurify is available under either Apache-2.0 or MPL-2.0; D-AI distributions
  may rely on the Apache-2.0 option.
- Frontend build tooling includes MPL-2.0 components such as Lightning CSS.
  They are not D-AI code and retain their own file-level license terms.
- Copyright, attribution and NOTICE requirements from dependencies must be
  retained when their code is redistributed in source or object form.
