<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { ElMessage } from "element-plus";

import {
  audiencePresetToSelections,
  audienceRulesToEditorState,
  type AnnouncementAudiencePreset
} from "./audience";
import { renderAnnouncementMarkdown } from "./markdown";
import type {
  AnnouncementAudienceKind,
  AnnouncementDraftPayload,
  AnnouncementTenantLoader,
  AnnouncementTenantOption,
  ManagedAnnouncement
} from "./types";

const props = defineProps<{
  visible: boolean;
  mode: "platform" | "tenant";
  item: ManagedAnnouncement | null;
  saving: boolean;
  loadTenants?: AnnouncementTenantLoader;
}>();

const emit = defineEmits<{ close: []; save: [payload: AnnouncementDraftPayload] }>();
const formRef = ref<FormInstance>();
const activeTab = ref("edit");
const tenantLoading = ref(false);
const tenantOptions = ref<AnnouncementTenantOption[]>([]);

const form = reactive({
  title: "",
  contentMarkdown: "",
  category: "general" as AnnouncementDraftPayload["category"],
  severity: "info" as AnnouncementDraftPayload["severity"],
  displayMode: "inbox" as AnnouncementDraftPayload["displayMode"],
  startsAt: null as Date | null,
  endsAt: null as Date | null,
  audiencePreset: "all" as AnnouncementAudiencePreset,
  tenantIds: [] as string[],
  selectedKinds: ["end_user"] as Array<Exclude<AnnouncementAudienceKind, "admin">>
});

const rules: FormRules = {
  title: [{ required: true, message: "请输入公告标题", trigger: "blur" }],
  contentMarkdown: [{ required: true, message: "请输入公告内容", trigger: "blur" }],
  category: [{ required: true, message: "请选择公告分类", trigger: "change" }],
  severity: [{ required: true, message: "请选择重要程度", trigger: "change" }],
  displayMode: [{ required: true, message: "请选择展示方式", trigger: "change" }]
};

const renderedContent = computed(() => renderAnnouncementMarkdown(form.contentMarkdown));
const dialogTitle = computed(() => props.item ? "编辑公告草稿" : "新建公告");

watch(
  () => [props.visible, props.item] as const,
  ([visible, item]) => {
    if (!visible) return;
    activeTab.value = "edit";
    form.title = item?.title ?? "";
    form.contentMarkdown = item?.contentMarkdown ?? "";
    form.category = item?.category ?? "general";
    form.severity = item?.severity ?? "info";
    form.displayMode = item?.displayMode ?? "inbox";
    form.startsAt = item?.startsAt ? new Date(item.startsAt) : null;
    form.endsAt = item?.endsAt ? new Date(item.endsAt) : null;
    const audience = audienceRulesToEditorState(item?.audiences ?? []);
    form.audiencePreset = item ? audience.preset : "all";
    form.tenantIds = audience.tenantIds;
    form.selectedKinds = audience.selectedKinds.length ? audience.selectedKinds : ["end_user"];
    tenantOptions.value = audience.tenantIds.map((tenantId) => ({ tenantId, tenantName: tenantId }));
    formRef.value?.clearValidate();
  },
  { immediate: true }
);

async function remoteTenantSearch(keyword: string) {
  if (!props.loadTenants) return;
  tenantLoading.value = true;
  try {
    const loaded = await props.loadTenants(keyword);
    const selected = tenantOptions.value.filter((option) => form.tenantIds.includes(option.tenantId));
    tenantOptions.value = [...new Map([...selected, ...loaded].map((option) => [option.tenantId, option])).values()];
  } catch {
    ElMessage.error("加载租户列表失败");
  } finally {
    tenantLoading.value = false;
  }
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;
  if (form.startsAt && form.endsAt && form.endsAt.getTime() <= form.startsAt.getTime()) {
    ElMessage.warning("结束时间必须晚于开始时间");
    return;
  }
  if (props.mode === "platform" && form.audiencePreset === "selected") {
    if (form.tenantIds.length === 0 || form.selectedKinds.length === 0) {
      ElMessage.warning("请选择租户和接收用户类型");
      return;
    }
  }

  emit("save", {
    title: form.title.trim(),
    contentMarkdown: form.contentMarkdown.trim(),
    category: form.category,
    severity: form.severity,
    displayMode: form.displayMode,
    startsAt: form.startsAt?.getTime(),
    endsAt: form.endsAt?.getTime(),
    audiences: props.mode === "platform"
      ? audiencePresetToSelections(form.audiencePreset, form.tenantIds, form.selectedKinds)
      : undefined
  });
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="dialogTitle"
    width="min(820px, 96vw)"
    append-to-body
    destroy-on-close
    @close="emit('close')"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="announcement-editor">
      <el-form-item label="标题" prop="title">
        <el-input v-model="form.title" maxlength="200" show-word-limit placeholder="输入公告标题" />
      </el-form-item>

      <div class="announcement-editor__grid">
        <el-form-item label="分类" prop="category">
          <el-select v-model="form.category">
            <el-option label="系统公告" value="general" />
            <el-option label="维护通知" value="maintenance" />
            <el-option label="升级通知" value="upgrade" />
            <el-option label="费率变更" value="pricing" />
            <el-option label="安全通知" value="security" />
          </el-select>
        </el-form-item>
        <el-form-item label="重要程度" prop="severity">
          <el-select v-model="form.severity">
            <el-option label="普通" value="info" />
            <el-option label="重要" value="important" />
            <el-option label="紧急" value="critical" />
          </el-select>
        </el-form-item>
        <el-form-item label="展示方式" prop="displayMode">
          <el-radio-group v-model="form.displayMode">
            <el-radio-button value="inbox">公告中心</el-radio-button>
            <el-radio-button value="popup">强提醒</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </div>

      <div class="announcement-editor__grid announcement-editor__grid--time">
        <el-form-item label="生效时间">
          <el-date-picker v-model="form.startsAt" type="datetime" placeholder="发布后立即生效" />
        </el-form-item>
        <el-form-item label="结束时间">
          <el-date-picker v-model="form.endsAt" type="datetime" placeholder="不自动结束" />
        </el-form-item>
      </div>

      <el-form-item label="通知范围">
        <template v-if="mode === 'tenant'">
          <el-alert title="本租户全部终端用户" type="info" :closable="false" show-icon />
        </template>
        <template v-else>
          <el-radio-group v-model="form.audiencePreset" class="announcement-editor__audiences">
            <el-radio-button value="all">全体</el-radio-button>
            <el-radio-button value="admins">管理员</el-radio-button>
            <el-radio-button value="tenant_users">租户用户</el-radio-button>
            <el-radio-button value="end_users">终端用户</el-radio-button>
            <el-radio-button value="selected">指定租户</el-radio-button>
          </el-radio-group>
          <div v-if="form.audiencePreset === 'selected'" class="announcement-editor__selected-audience">
            <el-select
              v-model="form.tenantIds"
              multiple
              filterable
              remote
              reserve-keyword
              :remote-method="remoteTenantSearch"
              :loading="tenantLoading"
              placeholder="搜索并选择租户"
              @visible-change="(open: boolean) => open && remoteTenantSearch('')"
            >
              <el-option
                v-for="option in tenantOptions"
                :key="option.tenantId"
                :label="option.tenantName"
                :value="option.tenantId"
              />
            </el-select>
            <el-checkbox-group v-model="form.selectedKinds">
              <el-checkbox value="tenant_user">租户用户</el-checkbox>
              <el-checkbox value="end_user">终端用户</el-checkbox>
            </el-checkbox-group>
          </div>
        </template>
      </el-form-item>

      <el-form-item label="公告内容" prop="contentMarkdown">
        <el-tabs v-model="activeTab" class="announcement-editor__content">
          <el-tab-pane label="编辑" name="edit">
            <el-input
              v-model="form.contentMarkdown"
              type="textarea"
              :rows="11"
              maxlength="50000"
              show-word-limit
              placeholder="支持 Markdown 格式"
            />
          </el-tab-pane>
          <el-tab-pane label="预览" name="preview">
            <div v-if="form.contentMarkdown" class="announcement-editor__preview" v-html="renderedContent" />
            <el-empty v-else description="输入内容后可预览" :image-size="64" />
          </el-tab-pane>
        </el-tabs>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button :disabled="saving" @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submit">保存草稿</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.announcement-editor__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
}

.announcement-editor__grid--time {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.announcement-editor :deep(.el-select),
.announcement-editor :deep(.el-date-editor) {
  width: 100%;
}

.announcement-editor__audiences {
  display: flex;
  flex-wrap: wrap;
}

.announcement-editor__selected-audience {
  display: grid;
  width: 100%;
  grid-template-columns: minmax(260px, 1fr) auto;
  align-items: center;
  gap: 16px;
  margin-top: 12px;
}

.announcement-editor__content {
  width: 100%;
}

.announcement-editor__preview {
  min-height: 220px;
  max-height: 360px;
  overflow: auto;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-sm);
  padding: 14px;
  color: var(--ds-ink);
  line-height: 1.7;
  overflow-wrap: anywhere;
}

@media (max-width: 680px) {
  .announcement-editor__grid,
  .announcement-editor__grid--time,
  .announcement-editor__selected-audience {
    grid-template-columns: 1fr;
  }
}
</style>
