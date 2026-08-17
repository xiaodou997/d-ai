const onlineRechargeOrderTypes = new Set([
  "online_user_topup",
  "online_tenant_topup",
  "user_topup_income"
]);

const manualRechargeOrderTypes = new Set([
  "platform_to_tenant",
  "tenant_to_user"
]);

export function rechargeMethodText(orderType: string): string {
  if (onlineRechargeOrderTypes.has(orderType)) return "在线充值";
  if (manualRechargeOrderTypes.has(orderType)) return "手动充值";
  return "—";
}

export function isOnlineRecharge(orderType: string): boolean {
  return onlineRechargeOrderTypes.has(orderType);
}
