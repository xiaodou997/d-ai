<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";

interface TocItem {
  id: string;
  label: string;
}

const route = useRoute();
const tocItems = ref<TocItem[]>([]);
const activeTocId = ref("");
const contentRef = ref<HTMLElement | null>(null);
let observer: IntersectionObserver | null = null;

function buildToc() {
  if (!contentRef.value) return;

  const cards = contentRef.value.querySelectorAll(".portal-content-card");
  const items: TocItem[] = [];

  cards.forEach((card, index) => {
    const titleEl = card.querySelector(".portal-content-card__title");
    if (!titleEl || !titleEl.textContent) return;

    const id = `guide-section-${index}`;
    card.setAttribute("id", id);
    (card as HTMLElement).style.scrollMarginTop = "76px";
    items.push({ id, label: titleEl.textContent.trim() });
  });

  tocItems.value = items;

  if (observer) observer.disconnect();

  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          activeTocId.value = entry.target.id;
        }
      }
    },
    { rootMargin: "-80px 0px -60% 0px", threshold: 0 }
  );

  cards.forEach((card) => {
    if (card.getAttribute("id")) observer?.observe(card);
  });
}

function scrollToSection(id: string) {
  const el = document.getElementById(id);
  if (el) {
    el.scrollIntoView({ behavior: "smooth", block: "start" });
    activeTocId.value = id;
  }
}

watch(
  () => route.path,
  () => {
    tocItems.value = [];
    activeTocId.value = "";
    nextTick(() => setTimeout(() => buildToc(), 100));
  }
);

onMounted(() => {
  nextTick(() => setTimeout(() => buildToc(), 100));
});

onUnmounted(() => {
  observer?.disconnect();
});
</script>

<template>
  <div class="guide-layout">
    <div ref="contentRef" class="guide-content">
      <RouterView />
    </div>

    <aside v-if="tocItems.length > 1" class="guide-toc">
      <p class="guide-toc__label">本页目录</p>
      <ul class="guide-toc__list">
        <li v-for="item in tocItems" :key="item.id">
          <button
            type="button"
            class="guide-toc__link"
            :class="{ 'is-active': activeTocId === item.id }"
            @click="scrollToSection(item.id)"
          >
            {{ item.label }}
          </button>
        </li>
      </ul>
    </aside>
  </div>
</template>

<style scoped>
.guide-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 208px;
  gap: 24px;
}

.guide-content {
  min-width: 0;
  align-self: start;
}

/*
 * 与 DsSidebar 完全相同的 sticky 模式：
 *   position: sticky + align-self: start 直接在 grid item 上。
 * grid item 不拉伸（align-self: start），但 sticky 的 containing block
 * 是 grid area（整行高度 = 文章内容高度），所以有足够空间粘住。
 *
 * 视口滚动时 sticky 参考系是视口。DsTopbar 高 56px (z-index:30)，
 * top 必须 > 56px 否则被顶栏盖住。
 */
.guide-toc {
  position: sticky;
  top: 76px;
  align-self: start;
  max-height: calc(100vh - 100px);
  overflow-y: auto;
  z-index: 20;
  padding: 0 0 16px;
}

.guide-toc__label {
  margin: 0 0 10px;
  padding: 0 14px;
  font-size: 11px;
  font-weight: 700;
  color: var(--ds-faint);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.guide-toc__list {
  list-style: none;
  margin: 0;
  padding: 0;
  border-left: 2px solid var(--ds-line);
}

.guide-toc__link {
  display: block;
  width: 100%;
  padding: 7px 14px;
  border: 0;
  border-radius: var(--ds-radius-none) var(--ds-radius-sm) var(--ds-radius-sm) var(--ds-radius-none);
  background: transparent;
  text-align: left;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.45;
  color: var(--ds-muted);
  cursor: pointer;
  border-left: 2px solid transparent;
  margin-left: -2px;
  overflow-wrap: anywhere;
  transition: color 0.15s ease, background 0.15s ease;
}

.guide-toc__link:hover {
  color: var(--ds-ink-soft);
  background: var(--ds-panel-muted);
}

.guide-toc__link.is-active {
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
  border-left-color: var(--ds-accent);
  font-weight: 700;
}

.guide-toc__link:focus-visible {
  outline: 2px solid var(--ds-accent);
  outline-offset: 2px;
}

@media (max-width: 1000px) {
  .guide-layout {
    grid-template-columns: 1fr;
  }

  .guide-toc {
    display: none;
  }
}
</style>

<!-- 使用说明文章共享样式（非 scoped，子路由文章组件可直接使用 .guide-* 类名） -->
<style>
.guide-list {
  margin: 0;
  padding-left: 18px;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.8;
}

.guide-list li + li {
  margin-top: 6px;
}

.guide-list strong {
  color: var(--ds-ink);
}

.guide-steps {
  margin: 0;
  padding: 0;
  list-style: none;
  counter-reset: guide-step;
}

.guide-steps > li {
  position: relative;
  padding-left: 44px;
  padding-bottom: 24px;
  border-left: 2px solid var(--ds-line);
  margin-left: 16px;
}

.guide-steps > li:last-child {
  border-left-color: transparent;
  padding-bottom: 0;
}

.guide-step__head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.guide-step__num {
  position: absolute;
  left: -16px;
  top: 0;
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--ds-radius-circle);
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
  font-size: 13px;
  font-weight: 700;
  flex-shrink: 0;
}

.guide-step__title {
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 700;
}

.guide-step__desc {
  margin: 0 0 8px;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

.guide-step__link {
  display: inline-block;
  margin-right: 12px;
  padding: 4px 10px;
  border-radius: var(--ds-radius-control);
  background: color-mix(in srgb, var(--ds-accent) 8%, transparent);
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  text-decoration: none;
  transition: background 0.15s ease;
}

.guide-step__link:hover {
  background: color-mix(in srgb, var(--ds-accent) 16%, transparent);
}

.guide-step__link--alt {
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.guide-step__link--alt:hover {
  background: var(--ds-line);
}

.guide-code {
  padding: 1px 6px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
}

.guide-concepts {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.guide-concept {
  padding: 12px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.guide-concept dt {
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 4px;
}

.guide-concept dd {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

.guide-next-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.guide-next-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  text-decoration: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.guide-next-card:hover {
  border-color: color-mix(in srgb, var(--ds-accent) 30%, var(--ds-line));
  box-shadow: var(--ds-shadow-sm);
}

.guide-next-card__title {
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 700;
}

.guide-next-card__desc {
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.guide-screenshot {
  margin: 16px 0 0;
}

.guide-screenshot__image {
  display: block;
  width: 100%;
  max-height: 680px;
  object-fit: contain;
  object-position: top;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
}

.guide-screenshot figcaption {
  margin-top: 8px;
  text-align: center;
  color: var(--ds-faint);
  font-size: 12px;
}

.guide-tip {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

.guide-tip__head {
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
}

.guide-tip__body {
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

.guide-table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  overflow: hidden;
  font-size: 13px;
}

.guide-table th {
  padding: 10px 12px;
  background: var(--ds-panel-muted);
  border-bottom: 1px solid var(--ds-line);
  color: var(--ds-faint);
  text-align: left;
  font-size: 11px;
  font-weight: 700;
}

.guide-table td {
  padding: 11px 12px;
  border-bottom: 1px solid var(--ds-line);
  color: var(--ds-muted);
  vertical-align: top;
  line-height: 1.7;
}

.guide-table tr:last-child td {
  border-bottom: 0;
}

.guide-table strong {
  color: var(--ds-ink);
}

.guide-link-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
}

.guide-link {
  display: inline-block;
  padding: 6px 12px;
  border-radius: var(--ds-radius-control);
  background: color-mix(in srgb, var(--ds-accent) 8%, transparent);
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  text-decoration: none;
  transition: background 0.15s ease;
}

.guide-link:hover {
  background: color-mix(in srgb, var(--ds-accent) 16%, transparent);
}

.guide-link--alt {
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
}

.guide-link--alt:hover {
  background: var(--ds-line);
}

.guide-paragraph {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.7;
}

.guide-paragraph + .guide-paragraph {
  margin-top: 10px;
}

@media (max-width: 768px) {
  .guide-next-grid {
    grid-template-columns: 1fr;
  }
}
</style>
