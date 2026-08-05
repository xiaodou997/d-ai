<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from "vue";
import QRCode from "qrcode";

export interface QrPayPollResult {
  status: "created" | "paying" | "paid" | "closed" | "expired";
  creditAmount?: number;
  transactionId?: string;
}

const props = defineProps<{
  visible: boolean;
  orderId: string;
  codeUrl: string;
  amount: number; // 分
  creditAmount: number;
  expiresAt: number; // epoch ms
  poll: () => Promise<QrPayPollResult>;
}>();

const emit = defineEmits<{
  close: [];
  success: [result: QrPayPollResult];
}>();

const canvasRef = ref<HTMLCanvasElement | null>(null);
const remainingSeconds = shallowRef(0);
const phase = shallowRef<"pending" | "paying" | "success" | "timeout">("pending");

let pollTimer: ReturnType<typeof setInterval> | null = null;
let countdownTimer: ReturnType<typeof setInterval> | null = null;

function stopTimers() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
  if (countdownTimer) {
    clearInterval(countdownTimer);
    countdownTimer = null;
  }
}

function formatCountdown(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

const countdownLabel = computed(() => formatCountdown(Math.max(0, remainingSeconds.value)));
const amountYuan = computed(() => (props.amount / 100).toFixed(2));

async function drawQrCode() {
  await nextTick();
  if (!canvasRef.value || !props.codeUrl) return;
  try {
    await QRCode.toCanvas(canvasRef.value, props.codeUrl, { width: 220, margin: 1 });
  } catch {
    // 画码失败不阻断轮询，展示原始链接兜底
  }
}

async function tickPoll() {
  try {
    const result = await props.poll();
    if (result.status === "paid") {
      phase.value = "success";
      stopTimers();
      emit("success", result);
    } else if (result.status === "closed" || result.status === "expired") {
      phase.value = "timeout";
      stopTimers();
    } else if (result.status === "paying") {
      phase.value = "paying";
    }
  } catch {
    // 瞬时错误忽略，继续下一轮轮询
  }
}

function startPolling() {
  stopTimers();
  phase.value = "pending";
  remainingSeconds.value = Math.max(0, Math.round((props.expiresAt - Date.now()) / 1000));
  pollTimer = setInterval(() => void tickPoll(), 3000);
  countdownTimer = setInterval(() => {
    remainingSeconds.value = Math.max(0, remainingSeconds.value - 1);
    if (remainingSeconds.value === 0 && phase.value !== "success") {
      phase.value = "timeout";
      stopTimers();
    }
  }, 1000);
}

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      startPolling();
      void drawQrCode();
    } else {
      stopTimers();
    }
  },
  { immediate: true }
);

watch(() => props.codeUrl, () => void drawQrCode());

onBeforeUnmount(stopTimers);

function handleClose() {
  stopTimers();
  emit("close");
}
</script>

<template>
  <el-dialog :model-value="visible" title="微信扫码支付" width="380px" append-to-body @close="handleClose">
    <div class="qr-pay-body">
      <div class="qr-pay-amount">
        <span class="qr-pay-amount-yuan">¥{{ amountYuan }}</span>
        <span class="qr-pay-amount-credit">到账 {{ creditAmount }} 积分</span>
      </div>

      <div class="qr-pay-canvas-wrap">
        <canvas v-show="phase === 'pending' || phase === 'paying'" ref="canvasRef" class="qr-pay-canvas" />
        <div v-if="phase === 'success'" class="qr-pay-state qr-pay-state--success">
          <span class="qr-pay-state-icon">✓</span>
          <strong>支付成功</strong>
        </div>
        <div v-else-if="phase === 'timeout'" class="qr-pay-state qr-pay-state--timeout">
          <span class="qr-pay-state-icon">!</span>
          <strong>二维码已失效</strong>
          <p>请关闭后重新发起充值</p>
        </div>
      </div>

      <div v-if="phase === 'pending' || phase === 'paying'" class="qr-pay-hint">
        <p>请使用微信扫一扫完成支付</p>
        <p class="qr-pay-countdown">剩余 {{ countdownLabel }}{{ phase === "paying" ? "，支付确认中…" : "" }}</p>
      </div>
    </div>

    <template #footer>
      <el-button @click="handleClose">{{ phase === "success" ? "完成" : "关闭" }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.qr-pay-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 4px 0 12px;
}

.qr-pay-amount {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.qr-pay-amount-yuan {
  font-size: 28px;
  font-weight: 700;
  color: var(--ds-ink);
}

.qr-pay-amount-credit {
  font-size: 13px;
  color: var(--ds-muted);
}

.qr-pay-canvas-wrap {
  width: 220px;
  height: 220px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
}

.qr-pay-canvas {
  width: 220px;
  height: 220px;
}

.qr-pay-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--ds-muted);
  text-align: center;
}

.qr-pay-state-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  font-size: 24px;
  font-weight: 700;
}

.qr-pay-state--success .qr-pay-state-icon {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.qr-pay-state--success strong {
  color: var(--ds-positive);
  font-size: 16px;
}

.qr-pay-state--timeout .qr-pay-state-icon {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.qr-pay-state--timeout strong {
  color: var(--ds-danger);
  font-size: 16px;
}

.qr-pay-state--timeout p {
  margin: 0;
  font-size: 13px;
}

.qr-pay-hint {
  text-align: center;
  color: var(--ds-muted);
  font-size: 13px;
}

.qr-pay-hint p {
  margin: 4px 0;
}

.qr-pay-countdown {
  color: var(--ds-warning);
  font-weight: 600;
}
</style>
