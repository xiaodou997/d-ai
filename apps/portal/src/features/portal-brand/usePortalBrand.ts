import { onMounted, shallowRef } from "vue";

import { platformCustomerApi } from "@/api/platformCustomer";
import type { CustomerPortalBrand } from "@/api/types/platformCustomer";

/**
 * 终端用户门户的品牌信息（站点名 + favicon）。
 *
 * 请求和状态放在这里而不是布局视图里：视图只做 feature 组合，API/DTO 依赖
 * 下沉到 composable（见 docs/frontend-feature-architecture-debt.md）。
 *
 * 品牌拿不到不该挡住整个门户渲染，所以失败时回退为 null，调用方自己兜底成
 * 租户名。
 */
export function usePortalBrand() {
  const portalBrand = shallowRef<CustomerPortalBrand | null>(null);

  async function load() {
    try {
      portalBrand.value = await platformCustomerApi.getPortalBrand();
    } catch {
      portalBrand.value = null;
    }
  }

  onMounted(() => {
    void load();
  });

  return { portalBrand };
}
