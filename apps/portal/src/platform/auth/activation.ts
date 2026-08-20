import { ElMessage, ElMessageBox } from "element-plus";

export interface ActivationCredential {
  activationToken: string;
  activationExpiresIn: number;
}

export function buildActivationUrl(token: string): string {
  const basePath = import.meta.env.BASE_URL.replace(/\/$/, "");
  const url = new URL(`${basePath}/activate`, window.location.origin);
  url.hash = new URLSearchParams({ token }).toString();
  return url.toString();
}

export async function showActivationCredential(
  credential: ActivationCredential,
  accountName: string
): Promise<void> {
  const url = buildActivationUrl(credential.activationToken);
  const expiresInHours = Math.max(1, Math.ceil(credential.activationExpiresIn / 3600));

  try {
    await ElMessageBox.prompt(
      `该链接仅在此处展示一次，并将在 ${expiresInHours} 小时后失效。`,
      `${accountName}的激活链接`,
      {
        inputValue: url,
        inputType: "textarea",
        confirmButtonText: "复制并关闭",
        showCancelButton: false,
        showClose: false,
        closeOnClickModal: false,
        closeOnPressEscape: false,
        inputValidator: () => true
      }
    );
  } catch {
    return;
  }

  try {
    await navigator.clipboard.writeText(url);
    ElMessage.success("激活链接已复制");
  } catch {
    ElMessage.warning("无法自动复制，请手动复制激活链接");
    await ElMessageBox.prompt("请手动复制下方链接后关闭。", `${accountName}的激活链接`, {
      inputValue: url,
      inputType: "textarea",
      confirmButtonText: "已复制，关闭",
      showCancelButton: false,
      showClose: false,
      closeOnClickModal: false,
      closeOnPressEscape: false,
      inputValidator: () => true
    });
  }
}
