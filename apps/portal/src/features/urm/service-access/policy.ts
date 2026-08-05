export type ServiceAccessWrite = {
  mode: "all" | "selected";
  serviceIds: string[];
};

export function editablePolicyForActor(
  policy: ServiceAccessWrite,
  userType: number | null,
  enabledClientIds: string[]
): ServiceAccessWrite {
  if (userType === 1) {
    return { mode: policy.mode, serviceIds: [...policy.serviceIds] };
  }

  const allowed = new Set(enabledClientIds);
  return {
    mode: "selected",
    serviceIds: policy.mode === "all"
      ? [...allowed]
      : policy.serviceIds.filter((id) => allowed.has(id))
  };
}
