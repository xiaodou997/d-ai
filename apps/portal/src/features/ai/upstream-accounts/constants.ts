// 上游账号自身的 Provider 端点元数据。

export interface EndpointProtocolOption {
  label: string;
  value: string;
}

export function defaultBindingProtocolForProviderFamily(providerFamily?: string) {
  switch (providerFamily) {
    case "anthropic":
      return "anthropic_messages";
    case "gemini":
      return "gemini_generate";
    default:
      return "openai_responses";
  }
}

export const endpointProtocolOptions: EndpointProtocolOption[] = [
  { label: "OpenAI", value: "openai_compatible" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Gemini", value: "gemini" }
];
