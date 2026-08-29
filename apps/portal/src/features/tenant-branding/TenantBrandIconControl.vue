<script setup lang="ts">
import { Delete, Upload } from "@element-plus/icons-vue";
import { useTemplateRef } from "vue";

import { FAVICON_ACCEPT } from "./normalizeFavicon";

const props = defineProps<{
  faviconUrl?: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  choose: [file: File];
  remove: [];
}>();

const fileInput = useTemplateRef<HTMLInputElement>("fileInput");

function openFilePicker() {
  fileInput.value?.click();
}

function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (file) emit("choose", file);
}
</script>

<template>
  <div class="tenant-brand-icon-control">
    <div class="tenant-brand-icon-control__preview" :class="{ 'is-empty': !props.faviconUrl }">
      <img v-if="props.faviconUrl" :src="props.faviconUrl" alt="当前用户门户小图标" />
      <span v-else>豆</span>
    </div>

    <div class="tenant-brand-icon-control__copy">
      <strong>用户门户小图标</strong>
      <p>支持 JPG / PNG / WebP，上传后自动裁剪为正方形并压缩。它会显示在用户门户顶栏和浏览器标签页。</p>
      <div class="tenant-brand-icon-control__actions">
        <el-button :icon="Upload" :loading="props.loading" @click="openFilePicker">上传图标</el-button>
        <el-button v-if="props.faviconUrl" :icon="Delete" type="danger" plain :loading="props.loading" @click="emit('remove')">
          恢复默认
        </el-button>
      </div>
    </div>

    <input ref="fileInput" class="tenant-brand-icon-control__input" type="file" :accept="FAVICON_ACCEPT" @change="onFileSelected" />
  </div>
</template>

<style scoped>
.tenant-brand-icon-control {
  display: flex;
  align-items: center;
  gap: 16px;
}

.tenant-brand-icon-control__preview {
  display: grid;
  width: 72px;
  height: 72px;
  flex: 0 0 72px;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
  font-size: 28px;
  font-weight: 700;
}

.tenant-brand-icon-control__preview.is-empty {
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.tenant-brand-icon-control__preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.tenant-brand-icon-control__copy {
  min-width: 0;
}

.tenant-brand-icon-control__copy strong {
  color: var(--ds-ink);
  font-size: 14px;
}

.tenant-brand-icon-control__copy p {
  margin: 5px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.55;
}

.tenant-brand-icon-control__actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}

.tenant-brand-icon-control__input {
  display: none;
}

@media (max-width: 640px) {
  .tenant-brand-icon-control {
    align-items: flex-start;
  }
}
</style>
