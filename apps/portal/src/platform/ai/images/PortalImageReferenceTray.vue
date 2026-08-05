<script setup lang="ts">
import { ArrowLeft, ArrowRight, Delete, Plus, ZoomIn } from "@element-plus/icons-vue";

import type { PortalImageReference, PortalImageReferenceMove } from "./usePortalImageReferences";

defineProps<{
  references: readonly PortalImageReference[];
}>();

const emit = defineEmits<{
  add: [files: File[]];
  remove: [id: string];
  move: [payload: { id: string; direction: PortalImageReferenceMove }];
  preview: [src: string];
  insertReference: [index: number];
}>();

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  if (files.length > 0) emit("add", files);
  input.value = "";
}
</script>

<template>
  <section class="reference-tray" aria-label="参考图">
    <div class="reference-tray__heading">
      <span>参考图</span>
      <small v-if="references.length">{{ references.length }} 张</small>
    </div>

    <div class="reference-tray__scroller">
      <article v-for="(reference, index) in references" :key="reference.id" class="reference-item">
        <button class="reference-item__preview" type="button" :aria-label="`预览图片 ${index + 1}`" @click="emit('preview', reference.previewUrl)">
          <img :src="reference.previewUrl" :alt="`图片 ${index + 1}`" />
          <span class="reference-item__zoom"><el-icon><ZoomIn /></el-icon></span>
        </button>

        <div class="reference-item__tools">
          <el-tooltip content="左移" placement="top">
            <span>
              <el-button
                circle
                text
                size="small"
                :icon="ArrowLeft"
                :disabled="index === 0"
                :aria-label="`左移图片 ${index + 1}`"
                @click="emit('move', { id: reference.id, direction: -1 })"
              />
            </span>
          </el-tooltip>
          <el-tooltip content="右移" placement="top">
            <span>
              <el-button
                circle
                text
                size="small"
                :icon="ArrowRight"
                :disabled="index === references.length - 1"
                :aria-label="`右移图片 ${index + 1}`"
                @click="emit('move', { id: reference.id, direction: 1 })"
              />
            </span>
          </el-tooltip>
          <el-tooltip content="移除" placement="top">
            <span>
              <el-button
                circle
                text
                size="small"
                class="reference-item__delete"
                :icon="Delete"
                :aria-label="`移除图片 ${index + 1}`"
                @click="emit('remove', reference.id)"
              />
            </span>
          </el-tooltip>
        </div>

        <button class="reference-item__label" type="button" :aria-label="`引用图片 ${index + 1}`" @click="emit('insertReference', index)">
          @图片{{ index + 1 }}
        </button>
      </article>

      <label class="reference-add">
        <input type="file" accept="image/*" multiple @change="onFileChange" />
        <el-icon><Plus /></el-icon>
        <span>添加</span>
      </label>
    </div>
  </section>
</template>

<style scoped>
.reference-tray {
  min-width: 0;
}

.reference-tray__heading {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 7px;
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.reference-tray__heading small {
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 600;
}

.reference-tray__scroller {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding: 1px 1px 5px;
  scrollbar-width: thin;
}

.reference-item,
.reference-add {
  position: relative;
  flex: 0 0 82px;
  width: 82px;
  height: 82px;
  border-radius: 7px;
}

.reference-item {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  background: var(--ds-paper);
}

.reference-item__preview {
  display: block;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: zoom-in;
}

.reference-item__preview img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.reference-item__zoom {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: rgb(0 0 0 / 32%);
  color: #fff;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.reference-item:hover .reference-item__zoom,
.reference-item:focus-within .reference-item__zoom {
  opacity: 1;
}

.reference-item__tools {
  position: absolute;
  top: 3px;
  right: 3px;
  z-index: 2;
  display: flex;
  gap: 1px;
  padding: 2px;
  border-radius: 6px;
  background: rgb(20 22 25 / 78%);
  opacity: 0;
  transition: opacity 0.15s ease;
}

.reference-item:hover .reference-item__tools,
.reference-item:focus-within .reference-item__tools {
  opacity: 1;
}

.reference-item__tools :deep(.el-button) {
  width: 22px;
  min-width: 22px;
  height: 22px;
  margin: 0;
  color: #fff;
}

.reference-item__tools :deep(.el-button.is-disabled) {
  color: rgb(255 255 255 / 30%);
}

.reference-item__delete :deep(.el-icon) {
  color: #ff8c8c;
}

.reference-item__label {
  position: absolute;
  right: 4px;
  bottom: 4px;
  left: 4px;
  z-index: 2;
  overflow: hidden;
  padding: 3px 5px;
  border: 0;
  border-radius: 5px;
  background: rgb(18 20 23 / 78%);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.reference-add {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border: 1px dashed var(--ds-line-strong);
  background: color-mix(in srgb, var(--ds-paper) 76%, var(--ds-panel));
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease, color 0.15s ease;
}

.reference-add:hover,
.reference-add:focus-within {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.reference-add input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}

.reference-add :deep(.el-icon) {
  font-size: 20px;
}

@media (max-width: 560px) {
  .reference-item,
  .reference-add {
    flex-basis: 72px;
    width: 72px;
    height: 72px;
  }

  .reference-item__tools {
    opacity: 1;
  }
}
</style>
