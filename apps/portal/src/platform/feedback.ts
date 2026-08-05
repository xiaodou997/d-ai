import { ElMessage, ElMessageBox } from "element-plus";

export interface ConfirmDialogOptions {
  title: string;
  message: string;
  confirmButtonText?: string;
  cancelButtonText?: string;
  type?: "success" | "warning" | "info" | "error";
  confirmButtonClass?: string;
}

export interface NamedConfirmDialogOptions extends Omit<ConfirmDialogOptions, "message"> {
  message: (name: string) => string;
}

export function notifySuccess(message: string) {
  ElMessage.success(message);
}

export function notifyError(message: string) {
  ElMessage.error(message);
}

export function notifyWarning(message: string) {
  ElMessage.warning(message);
}

export async function confirmDialog(options: ConfirmDialogOptions): Promise<boolean> {
  try {
    await ElMessageBox.confirm(options.message, options.title, {
      confirmButtonText: options.confirmButtonText,
      cancelButtonText: options.cancelButtonText,
      type: options.type,
      confirmButtonClass: options.confirmButtonClass
    });
    return true;
  } catch {
    return false;
  }
}

export function createNamedConfirmDialog(options: NamedConfirmDialogOptions) {
  return (name: string) =>
    confirmDialog({
      ...options,
      message: options.message(name)
    });
}
