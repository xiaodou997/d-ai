// AI 网关页面共享常量。绑定选择器的定义归属于 upstream-model-bindings feature。

import type { CapabilityOption } from "@/features/ai/upstream-model-bindings/constants";

export type {
  BindingFormatGroup,
  BindingFormatOption,
  CapabilityOption
} from "@/features/ai/upstream-model-bindings/constants";
export {
  DEFAULT_OPENAI_BINDING_PROTOCOL,
  OTHER_CAPABILITIES_VALUE,
  OTHER_CAPABILITY_TYPES,
  bindingFormatGroups,
  bindingFormatValue,
  capabilityOptions,
  protocolOptions,
  statusOptions
} from "@/features/ai/upstream-model-bindings/constants";
export {
  defaultBindingProtocolForProviderFamily,
  endpointProtocolOptions
} from "@/features/ai/upstream-accounts/constants";

const STATUS_LABEL_MAP: Record<string, string> = {
  active: "启用",
  disabled: "停用"
};

export function statusLabel(value: string): string {
  if (!value) return "-";
  return STATUS_LABEL_MAP[value] || value;
}
