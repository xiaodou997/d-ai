import { onMounted, shallowRef } from "vue";

import { platformPublicApi } from "@/api/platformPublic";
import type { PasswordPolicy } from "@/api/types/platformPublic";

let policyRequest: Promise<PasswordPolicy> | undefined;

export function loadPasswordPolicy(): Promise<PasswordPolicy> {
  policyRequest ??= platformPublicApi.getPasswordPolicy().catch((error) => {
    policyRequest = undefined;
    throw error;
  });
  return policyRequest;
}

export function usePasswordPolicy() {
  const policy = shallowRef<PasswordPolicy | null>(null);
  onMounted(async () => {
    try {
      policy.value = await loadPasswordPolicy();
    } catch {
      policy.value = null;
    }
  });
  return policy;
}

export function validatePasswordAgainstPolicy(
  password: string,
  username: string,
  policy: PasswordPolicy
): string {
  if (Array.from(password).length < policy.minLength) return policy.description;
  if (new TextEncoder().encode(password).length > policy.maxBytes) {
    return `密码不能超过 ${policy.maxBytes} 字节。`;
  }

  let lower = false;
  let upper = false;
  let digit = false;
  let symbol = false;
  for (const character of password) {
    if (/\p{Ll}/u.test(character)) lower = true;
    else if (/\p{Lu}/u.test(character)) upper = true;
    else if (/\p{Nd}/u.test(character)) digit = true;
    else symbol = true;
  }
  if ([lower, upper, digit, symbol].filter(Boolean).length < policy.requiredCharacterClasses) {
    return policy.description;
  }

  const normalizedUsername = username.trim().toLocaleLowerCase();
  if (
    Array.from(normalizedUsername).length >= 4 &&
    password.toLocaleLowerCase().includes(normalizedUsername)
  ) {
    return "密码不能包含用户名。";
  }
  return "";
}
