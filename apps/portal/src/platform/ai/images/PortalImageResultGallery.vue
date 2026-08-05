<script setup lang="ts">
defineProps<{
  images: Array<{ src: string; revisedPrompt?: string }>;
  emptyText: string;
}>()
</script>

<template>
  <div v-if="images.length" class="gallery-grid">
    <figure v-for="(image, index) in images" :key="`${image.src}-${index}`" class="gallery-card">
      <img :src="image.src" :alt="image.revisedPrompt || `image-${index}`" class="gallery-image" />
      <figcaption>{{ image.revisedPrompt || "无修订提示词" }}</figcaption>
    </figure>
  </div>
  <el-empty v-else :description="emptyText" :image-size="72" />
</template>

<style scoped>
.gallery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
}

.gallery-card {
  margin: 0;
  padding: 12px;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

.gallery-image {
  width: 100%;
  display: block;
  border-radius: var(--ds-radius-control);
  aspect-ratio: 1;
  object-fit: cover;
  background: var(--ds-line);
}

.gallery-card figcaption {
  margin-top: 10px;
  color: var(--ds-ink-soft);
  font-size: 13px;
  line-height: 1.5;
}
</style>
