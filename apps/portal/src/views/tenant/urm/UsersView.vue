<!--
  租户端终端用户列表:搜索 + 表格 + 分页 + 创建/充值弹窗。
  重构:迁移至 DsUI 一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡),el-table → DsTable,el-tag → DsTag,空态 DsEmpty,
       分页始终渲染;创建/充值弹窗仍为 element-plus,业务逻辑与请求参数不变。
-->
<template>
  <div class="page-container users-page">
    <PortalPagePanel
      :icon="Users"
      :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '终端用户' }]"
      description="管理属于本租户的所有终端用户"
    >
      <template #actions>
        <GuideHelpLink to="/help/urm/create-users" />
        <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建用户</el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input
              v-model="keyword"
              placeholder="搜索用户名 / 邮箱 / 手机"
              clearable
              class="users-search"
              @keyup.enter="handleSearch"
              @clear="handleSearch"
            >
              <template #prefix><Search class="users-search__icon" /></template>
            </el-input>
          </DsFilterField>
          <template #actions>
            <el-button type="primary" @click="handleSearch">搜索</el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="userList"
        row-key="userId"
        :loading="loading"
      >
        <template #empty>
          <DsEmpty title="暂无终端用户" description="还没有终端用户,先创建一个吧">
            <template #action>
              <el-button type="primary" @click="openCreateDialog">创建用户</el-button>
            </template>
          </DsEmpty>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="row.status === 1 ? 'positive' : 'danger'">
            {{ row.status === 1 ? '正常' : '禁用' }}
          </DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="users-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button type="primary" link @click="openOverview(row)">详情</el-button>
          <el-button v-if="row.status === 1" type="danger" link @click="handleDisable(row)">停用</el-button>
          <el-button v-else type="success" link @click="handleEnable(row)">启用</el-button>
          <el-button type="warning" link @click="handleResetPassword(row)">重置密码</el-button>
          <el-button type="primary" link @click="openRecharge(row)">充值</el-button>
          <el-button type="danger" link @click="handleDelete(row)">删除</el-button>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handleSizeChange"
        />
      </template>
    </PortalPagePanel>

    <!-- 创建用户弹窗 -->
    <el-dialog
      v-model="showCreateDialog"
      title="创建终端用户"
      width="520px"
      :close-on-click-modal="false"
      :append-to-body="true"
      @close="resetCreateForm"
    >
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="90px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="createForm.username" placeholder="请输入用户名" maxlength="50" show-word-limit />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="createForm.email" placeholder="请输入邮箱（可选）" />
        </el-form-item>
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="createForm.phone" placeholder="请输入手机号（可选）" />
        </el-form-item>
        <el-alert
          title="默认密码为 123456，创建后请通知终端用户登录后自行修改。"
          type="warning"
          :closable="false"
          show-icon
        />
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" :loading="createLoading" @click="submitCreateUser">创建</el-button>
      </template>
    </el-dialog>

    <!-- 充值弹窗 -->
    <el-dialog
      v-model="showRechargeDialog"
      title="给用户充值"
      width="560px"
      :close-on-click-modal="false"
      :append-to-body="true"
      @close="resetRechargeForm"
    >
      <el-form ref="rechargeFormRef" :model="rechargeForm" :rules="rechargeRules" label-width="90px">
        <el-form-item label="用户">
          <div class="flex min-w-0 flex-col gap-1 leading-tight">
            <span class="users-recharge-name">{{ rechargeTarget?.username }}</span>
            <span class="users-recharge-identity">{{ rechargeTargetIdentity }}</span>
          </div>
        </el-form-item>

        <el-form-item label="实付金额" prop="paidAmountYuan">
          <el-input-number
            v-model="rechargeForm.paidAmountYuan"
            :min="0"
            :precision="2"
            :step="100"
            :controls="false"
            style="width: 100%"
            @change="isCreditAutoCalc = true"
          />
          <div class="flex gap-2 mt-2">
            <el-tag
              v-for="q in [100, 500, 1000]"
              :key="q"
              class="users-quick-pick"
              @click="handleAmountQuickPick(q)"
            >¥{{ q }}</el-tag>
          </div>
          <p class="users-field-hint">输入金额（元），自动计算到账积分（1元=100积分）</p>
        </el-form-item>

        <el-form-item label="到账积分" prop="creditAmount">
          <el-input-number
            v-model="rechargeForm.creditAmount"
            :min="1"
            :precision="0"
            :step="1000"
            :controls="false"
            style="width: 100%"
            @change="isCreditAutoCalc = false"
          />
          <div class="flex gap-2 mt-2">
            <el-tag
              v-for="q in [10000, 50000, 100000]"
              :key="q"
              class="users-quick-pick"
              @click="handleCreditQuickPick(q)"
            >+{{ q.toLocaleString() }}</el-tag>
          </div>
          <p class="users-field-hint">
            <span v-if="isCreditAutoCalc && (rechargeForm.paidAmountYuan ?? 0) > 0" class="users-field-hint--accent">
              已自动按 1元=100积分 计算，可手动修改
            </span>
            <span v-else>默认按 1元 = 100积分 自动换算，可手动修改</span>
          </p>
        </el-form-item>

        <el-form-item label="有效期">
          <el-date-picker
            v-model="rechargeForm.expireDate"
            type="datetime"
            placeholder="永久有效"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 100%"
            :disabled-date="(d: Date) => d.getTime() < Date.now()"
          />
          <div class="flex gap-2 mt-2 flex-wrap">
            <el-tag
              v-for="days in [7, 30, 90, 180, 365]"
              :key="days"
              class="users-quick-pick"
              @click="setExpireDays(days)"
            >{{ days }}天</el-tag>
            <el-tag
              class="users-quick-pick users-quick-pick--plain"
              @click="clearExpire"
            >永久有效</el-tag>
          </div>
          <p class="users-field-hint">按北京时间填写，留空则永久有效。</p>
          <p class="users-field-hint users-field-hint--accent font-bold" v-if="rechargeForm.expireDate">
            该笔积分到期后自动失效；消费时系统会优先扣减更早到期的有效积分，永久积分后扣。
          </p>
        </el-form-item>

        <el-form-item label="备注" prop="reason">
          <el-input
            v-model="rechargeForm.reason"
            type="textarea"
            :rows="4"
            :placeholder="isZeroAmount ? '实付金额为0，请详细说明免费充值原因（必填）' : '充值备注（可选）'"
            maxlength="500"
            show-word-limit
          />
          <p v-if="isZeroAmount" class="users-field-hint users-field-hint--danger font-bold">实付金额为 ¥0 时，备注说明为必填项</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRechargeDialog = false">取消</el-button>
        <el-button type="primary" :loading="rechargeLoading" @click="submitRecharge">确认充值</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { Plus, Search } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { FormInstance, FormRules } from "element-plus";
import { Users } from "lucide-vue-next";
import { PortalPagePanel } from "@dai/app-core";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@dai/ui";

import { urmTenantApi } from "../../api/urmTenant";
import type { EndUserItem } from "../../types/urmTenant";
import { GuideHelpLink } from "@dai/app-core/guide";

const columns: DsTableColumn[] = [
  { key: "userId", title: "用户 ID", width: 110, mono: true },
  { key: "username", title: "用户名", width: 130 },
  { key: "email", title: "邮箱" },
  { key: "status", title: "状态", width: 80 },
  { key: "createdTime", title: "注册时间", width: 170 },
  { key: "actions", title: "操作", width: 285 }
];

const router = useRouter();
const keyword = ref("");
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const loading = ref(false);
const userList = ref<EndUserItem[]>([]);

const showCreateDialog = ref(false);
const createLoading = ref(false);
const createFormRef = ref<FormInstance>();
const createForm = reactive({ username: "", email: "", phone: "" });
const createRules: FormRules = {
  username: [{ required: true, message: "请输入用户名", trigger: "blur" }]
};

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

async function fetchUsers() {
  loading.value = true;
  try {
    const res = await urmTenantApi.getUsers({
      keyword: keyword.value || undefined,
      page: page.value,
      size: pageSize.value
    });
    userList.value = res?.items ?? [];
    total.value = res?.total ?? userList.value.length;
  } catch (e) {
    console.error("获取用户列表失败:", e);
    userList.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

function handleSearch() {
  page.value = 1;
  fetchUsers();
}

function handlePageChange(value: number) {
  page.value = value;
  fetchUsers();
}

function handleSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
  fetchUsers();
}

function openCreateDialog() {
  createForm.username = "";
  createForm.email = "";
  createForm.phone = "";
  showCreateDialog.value = true;
}

function resetCreateForm() {
  createForm.username = "";
  createForm.email = "";
  createForm.phone = "";
  createFormRef.value?.clearValidate();
}

async function submitCreateUser() {
  const valid = await createFormRef.value?.validate().catch(() => false);
  if (!valid) return;
  createLoading.value = true;
  try {
    const data = await urmTenantApi.createEndUser({
      username: createForm.username.trim(),
      email: createForm.email?.trim() || undefined,
      phone: createForm.phone?.trim() || undefined
    });
    ElMessage.success(`用户创建成功，默认密码 ${data?.defaultPassword || "123456"}`);
    showCreateDialog.value = false;
    fetchUsers();
  } catch (e: any) {
    ElMessage.error(e?.message || "创建失败");
  } finally {
    createLoading.value = false;
  }
}

async function handleToggleStatus(row: EndUserItem, status: "active" | "disabled") {
  const action = status === "active" ? "启用" : "停用";
  const isDisable = status === "disabled";
  try {
    await ElMessageBox.confirm(
      `确定要${action}用户「${row.username}」吗？${isDisable ? "停用后该用户将立即无法登录。" : ""}`,
      `确认${action}`,
      {
        confirmButtonText: `确认${action}`,
        cancelButtonText: "取消",
        type: isDisable ? "warning" : "info",
        confirmButtonClass: isDisable ? "el-button--danger" : ""
      }
    );
    await urmTenantApi.updateUserStatus(row.userId, status);
    ElMessage.success(`用户已${action}`);
    fetchUsers();
  } catch (e: any) {
    if (e !== "cancel") ElMessage.error(e?.message || "操作失败");
  }
}

const handleDisable = (row: EndUserItem) => handleToggleStatus(row, "disabled");
const handleEnable = (row: EndUserItem) => handleToggleStatus(row, "active");

async function handleDelete(row: EndUserItem) {
  try {
    await ElMessageBox.confirm(
      `确定要删除用户「${row.username}」吗？账号将立即无法登录，且会从用户列表移除；历史账务与用量记录仍会保留。`,
      "确认删除用户",
      {
        confirmButtonText: "确认删除",
        cancelButtonText: "取消",
        type: "warning",
        confirmButtonClass: "el-button--danger"
      }
    );
    await urmTenantApi.deleteEndUser(row.userId);
    ElMessage.success("用户已删除");
    if (userList.value.length === 1 && page.value > 1) {
      page.value -= 1;
    }
    fetchUsers();
  } catch (e: any) {
    if (e !== "cancel") ElMessage.error(e?.message || "删除失败");
  }
}

function openOverview(row: EndUserItem) {
  void router.push(`/users/${encodeURIComponent(row.userId)}`);
}

async function handleResetPassword(row: EndUserItem) {
  try {
    await ElMessageBox.confirm(`确定要将用户「${row.username}」的密码重置为 123456 吗？`, "确认重置密码", {
      confirmButtonText: "确认重置",
      cancelButtonText: "取消",
      type: "warning"
    });
    await urmTenantApi.resetUserPassword(row.userId);
    ElMessage.success("密码已重置为 123456");
  } catch (e: any) {
    if (e !== "cancel") ElMessage.error(e?.message || "操作失败");
  }
}

// --- 充值 ---
const showRechargeDialog = ref(false);
const rechargeLoading = ref(false);
const rechargeTarget = ref<EndUserItem | null>(null);
const rechargeFormRef = ref<FormInstance>();
const isCreditAutoCalc = ref(true);
const rechargeForm = reactive<{
  paidAmountYuan: number | null;
  creditAmount: number | null;
  expireDate: string | null;
  reason: string;
}>({ paidAmountYuan: null, creditAmount: null, expireDate: null, reason: "" });

const isZeroAmount = computed(() => rechargeForm.paidAmountYuan === 0);
const rechargeTargetIdentity = computed(() => {
  const target = rechargeTarget.value;
  if (!target) return "";
  if (target.phone?.trim()) return `手机号 ${target.phone.trim()}`;
  if (target.email?.trim()) return `邮箱 ${target.email.trim()}`;
  return `UID ${target.userId}`;
});

const UTC8_TIME_ZONE = "Asia/Shanghai";
const utc8DateTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  timeZone: UTC8_TIME_ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23"
});

function formatUtc8DateTime(date: Date) {
  const parts = Object.fromEntries(
    utc8DateTimeFormatter
      .formatToParts(date)
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value])
  ) as Record<string, string>;

  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`;
}

function parseUtc8DateTime(value: string) {
  const match = value.trim().match(/^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/);
  if (!match) return Number.NaN;

  const [, year, month, day, hour, minute, second] = match;
  return Date.UTC(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour) - 8,
    Number(minute),
    Number(second)
  );
}

async function clearRechargeValidation(fields: Array<"paidAmountYuan" | "creditAmount" | "reason">) {
  await nextTick();
  rechargeFormRef.value?.clearValidate(fields);
}

const rechargeRules = computed<FormRules>(() => ({
  paidAmountYuan: [{ required: true, type: "number", min: 0, message: "请填写实付金额（元）", trigger: ["blur", "change"] }],
  creditAmount: [{ required: true, type: "number", min: 1, message: "到账积分至少为 1", trigger: ["blur", "change"] }],
  reason: isZeroAmount.value
    ? [
        { required: true, message: "实付金额为0时，备注说明必填", trigger: ["blur", "change"] },
        { min: 5, message: "至少5个字符", trigger: ["blur", "change"] }
      ]
    : []
}));

watch(
  () => rechargeForm.paidAmountYuan,
  (val) => {
    if (isCreditAutoCalc.value && val != null && val >= 0) {
      rechargeForm.creditAmount = Math.round(val * 100);
    }
  }
);

watch(isZeroAmount, (value) => {
  if (!value) {
    void clearRechargeValidation(["reason"]);
  }
});

function openRecharge(row: EndUserItem) {
  rechargeTarget.value = row;
  showRechargeDialog.value = true;
}

function setExpireDays(days: number) {
  rechargeForm.expireDate = formatUtc8DateTime(new Date(Date.now() + days * 24 * 60 * 60 * 1000));
}
function clearExpire() {
  rechargeForm.expireDate = null;
}

function handleAmountQuickPick(amount: number) {
  rechargeForm.paidAmountYuan = amount;
  isCreditAutoCalc.value = true;
  void clearRechargeValidation(["paidAmountYuan", "creditAmount", "reason"]);
}
function handleCreditQuickPick(amount: number) {
  rechargeForm.creditAmount = amount;
  isCreditAutoCalc.value = false;
  void clearRechargeValidation(["creditAmount"]);
}

function resetRechargeForm() {
  rechargeForm.paidAmountYuan = null;
  rechargeForm.creditAmount = null;
  rechargeForm.expireDate = null;
  rechargeForm.reason = "";
  isCreditAutoCalc.value = true;
  rechargeFormRef.value?.clearValidate();
}

async function submitRecharge() {
  const valid = await rechargeFormRef.value?.validate().catch(() => false);
  if (!valid || !rechargeTarget.value) return;
  if (rechargeForm.paidAmountYuan === 0) {
    try {
      await ElMessageBox.confirm(
        `实付金额为 ¥0，确认对「${rechargeTarget.value.username}」执行免费充值？`,
        "金额为零确认",
        { confirmButtonText: "确认免费充值", cancelButtonText: "取消", roundButton: true, type: "warning" }
      );
    } catch {
      return;
    }
  }
  rechargeLoading.value = true;
  try {
    await urmTenantApi.rechargeUser({
      userId: rechargeTarget.value.userId,
      paidAmount: Math.round((rechargeForm.paidAmountYuan ?? 0) * 100),
      creditAmount: rechargeForm.creditAmount ?? 0,
      note: rechargeForm.reason || undefined,
      expireTime: rechargeForm.expireDate ? parseUtc8DateTime(rechargeForm.expireDate) : null
    });
    ElMessage.success(`已成功为「${rechargeTarget.value.username}」充值 ${rechargeForm.creditAmount} 积分`);
    showRechargeDialog.value = false;
    fetchUsers();
  } catch (e: any) {
    ElMessage.error(e?.message || "充值失败");
  } finally {
    rechargeLoading.value = false;
  }
}

onMounted(() => {
  fetchUsers();
});
</script>

<style scoped>
.users-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.users-search {
  width: 260px;
}

.users-search__icon {
  width: 16px;
  height: 16px;
  color: var(--ds-faint);
}

.users-time {
  font-size: 12px;
  color: var(--ds-faint);
}

.users-recharge-name {
  font-weight: 600;
  color: var(--ds-ink-soft);
}

.users-recharge-identity {
  font-size: 12px;
  color: var(--ds-faint);
}

.users-quick-pick {
  cursor: pointer;
  border: none;
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  transition: all 0.15s ease;
}

.users-quick-pick:hover {
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
}

.users-quick-pick--plain:hover {
  background: var(--ds-line-strong);
  color: var(--ds-accent-contrast);
}

.users-field-hint {
  margin: 4px 0 0;
  font-size: 10px;
  color: var(--ds-faint);
}

.users-field-hint--accent {
  font-weight: 500;
  color: var(--ds-accent);
}

.users-field-hint--danger {
  color: var(--ds-danger);
}
</style>
