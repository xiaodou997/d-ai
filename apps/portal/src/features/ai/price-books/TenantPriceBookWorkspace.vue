<!--
  价格表 — 查看平台公共价格表（只读），维护租户私有价格表的模型 USD 售价（token 分档 / 图片 / 视频 / 音频）。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       汇率卡/价格表列表/条目表收进同卡 body 的 24px 容器）;条目表 el-table → DsTable
       （expandable 展开 token 分档）,el-tag → DsTag,空态 → DsEmpty;价格色值改用 --ds-* token,
       无硬编码 hex;业务逻辑与请求参数保持不变（平台表只读、私有表可写/复制）。
       ⚠ 条目接口无服务端分页,故不加 DsPagination。弹窗/表单仍为 element-plus。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, ref, shallowRef } from "vue";
import { CopyDocument, Delete, Edit, Plus, Refresh } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Tags } from "lucide-vue-next";
import { PortalContentCard, PortalPagePanel } from "@dai/app-core";
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from "@dai/ui";

import { aiTenantApi } from "../../../api/aiTenant";
import type { TenantAiPriceBook } from "../../../types/aiTenant";
import PriceBookEntryFormDialog from "./components/PriceBookEntryFormDialog.vue";
import {
  isTokenPricedCapability,
  validateTokenPriceTiers,
  type LiteLLMPriceModel,
  type PriceBookEntryForm,
  type PriceBookEntryRecord,
  type TokenPriceTier
} from "./pricingTypes";

const DEFAULT_CREDITS_PER_USD = 100;
const creditsPerUsd = shallowRef(DEFAULT_CREDITS_PER_USD);

const books = shallowRef<TenantAiPriceBook[]>([]);
const booksLoading = shallowRef(false);
const selectedBookId = shallowRef("");
const selectedBook = computed(() => books.value.find((book) => book.id === selectedBookId.value) || null);
const isWritable = computed(() => Boolean(selectedBook.value?.writable));

const entries = shallowRef<PriceBookEntryRecord[]>([]);
const entriesLoading = shallowRef(false);

// DsTable 列:model_code 为标识符用 mono;标准价格/规格覆盖/操作走 #cell-* 插槽;操作列仅私有表可写时出现
const entryColumns = computed<DsTableColumn[]>(() => {
  const columns: DsTableColumn[] = [
    { key: "model_code", title: "模型", width: 220, mono: true },
    { key: "capability_type", title: "能力", width: 90 },
    { key: "standardPrice", title: "标准价格", width: 150 },
    { key: "overrides", title: "规格覆盖" }
  ];
  if (isWritable.value) columns.push({ key: "actions", title: "操作", width: 120 });
  return columns;
});

const formatUsd2 = (value: unknown) => Number(value ?? 0).toFixed(2);
const formatUsd = (value: unknown) => `$${formatUsd2(value)}`;
const formatTierLimit = (limit: number | null) => limit === null ? "无上限" : limit.toLocaleString("zh-CN");
const imageTierLabel = (value: string) => ({ "1k": "1K", "2k": "2K", "4k": "4K" }[value] || value);

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

async function loadRate() {
  try {
    const response = await aiTenantApi.getCreditsPerUSD();
    creditsPerUsd.value = response?.credits_per_usd ?? DEFAULT_CREDITS_PER_USD;
  } catch {
    creditsPerUsd.value = DEFAULT_CREDITS_PER_USD;
  }
}

async function loadEntries() {
  if (!selectedBookId.value) {
    entries.value = [];
    return;
  }
  entriesLoading.value = true;
  try {
    const response = await aiTenantApi.listPriceBookEntries(selectedBookId.value);
    entries.value = response.items || [];
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "加载条目失败"));
  } finally {
    entriesLoading.value = false;
  }
}

async function loadBooks() {
  booksLoading.value = true;
  try {
    const response = await aiTenantApi.listPriceBooks();
    books.value = response.items || [];
    if (!books.value.some((book) => book.id === selectedBookId.value)) {
      selectedBookId.value = books.value[0]?.id || "";
    }
    await loadEntries();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "加载价格表失败"));
  } finally {
    booksLoading.value = false;
  }
}

function selectBook(id: string) {
  selectedBookId.value = id;
  void loadEntries();
}

const bookDialog = shallowRef(false);
const bookEditing = shallowRef(false);
const bookSaving = shallowRef(false);
const bookForm = reactive({ id: "", name: "", description: "", status: "active" as "active" | "disabled" });

function openCreateBook() {
  bookEditing.value = false;
  Object.assign(bookForm, { id: "", name: "", description: "", status: "active" });
  bookDialog.value = true;
}

function openEditBook() {
  if (!selectedBook.value?.writable) return;
  bookEditing.value = true;
  Object.assign(bookForm, {
    id: selectedBook.value.id,
    name: selectedBook.value.name,
    description: selectedBook.value.description || "",
    status: selectedBook.value.status || "active"
  });
  bookDialog.value = true;
}

async function submitBook() {
  if (!bookForm.name.trim()) {
    ElMessage.warning("请填写价格表名称");
    return;
  }
  bookSaving.value = true;
  try {
    if (bookEditing.value) {
      await aiTenantApi.updatePriceBook(bookForm.id, {
        name: bookForm.name.trim(),
        description: bookForm.description,
        status: bookForm.status
      });
      ElMessage.success("已保存");
    } else {
      const created = await aiTenantApi.createPriceBook({
        name: bookForm.name.trim(),
        description: bookForm.description
      });
      selectedBookId.value = created.id;
      ElMessage.success("已创建");
    }
    bookDialog.value = false;
    await loadBooks();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "保存失败"));
  } finally {
    bookSaving.value = false;
  }
}

async function copyBook() {
  if (!selectedBook.value) return;
  try {
    const { value } = await ElMessageBox.prompt(
      "复制后将生成一张可自行修改的租户私有价格表。",
      "复制价格表",
      {
        inputValue: `${selectedBook.value.name} - 副本`,
        inputPattern: /\S+/,
        inputErrorMessage: "请输入名称",
        confirmButtonText: "复制"
      }
    );
    const copied = await aiTenantApi.copyPriceBook(selectedBook.value.id, value.trim());
    selectedBookId.value = copied.id;
    await loadBooks();
    ElMessage.success("价格表已复制");
  } catch (error: unknown) {
    if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error, "复制失败"));
  }
}

async function removeBook() {
  if (!selectedBook.value?.writable) return;
  try {
    await ElMessageBox.confirm(
      `删除价格表「${selectedBook.value.name}」？其下所有条目将一并删除。`,
      "确认删除",
      { type: "warning" }
    );
    await aiTenantApi.deletePriceBook(selectedBook.value.id);
    selectedBookId.value = "";
    await loadBooks();
    ElMessage.success("已删除");
  } catch (error: unknown) {
    if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error, "删除失败"));
  }
}

const syncing = shallowRef(false);

async function syncCommon() {
  if (!selectedBook.value?.writable) return;
  try {
    await ElMessageBox.confirm(
      "自动获取一批常用模型（Claude / GPT / Gemini 等）的价格并填入本价格表？已手动编辑的条目不会被覆盖。",
      "同步常用模型",
      { type: "info" }
    );
  } catch {
    return;
  }
  syncing.value = true;
  try {
    const result = await aiTenantApi.syncCommonPriceModels(selectedBook.value.id);
    const missing = result.missing?.length ? `，未找到 ${result.missing.length} 个` : "";
    await loadEntries();
    ElMessage.success(`已同步 ${result.synced ?? 0} 个常用模型${missing}`);
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "同步失败"));
  } finally {
    syncing.value = false;
  }
}

const llmSearching = shallowRef(false);
const llmOptions = shallowRef<LiteLLMPriceModel[]>([]);

async function searchLiteLLM(query: string) {
  llmSearching.value = true;
  try {
    const response = await aiTenantApi.searchLiteLLMPriceModels(query || "", 50);
    llmOptions.value = response.items || [];
  } catch {
    llmOptions.value = [];
  } finally {
    llmSearching.value = false;
  }
}

const entryDialog = shallowRef(false);
const entryEditing = shallowRef(false);
const entrySaving = shallowRef(false);
const entryForm = ref<PriceBookEntryForm>(emptyEntryForm());

function emptyTokenTier(): TokenPriceTier {
  return {
    up_to_input_tokens: null,
    input_per_1m_usd: 0,
    output_per_1m_usd: 0,
    cache_write_per_1m_usd: 0,
    cache_read_per_1m_usd: 0
  };
}

function emptyEntryForm(): PriceBookEntryForm {
  return {
    model_code: "",
    capability_type: "chat",
    token_price_tiers: [emptyTokenTier()],
    audio_tts_per_1m_chars_usd: 0,
    audio_stt_per_minute_usd: 0,
    image_default_price_usd: 0,
    video_default_price_usd: 0,
    image_prices: [],
    video_prices: []
  };
}

function openCreateEntry() {
  if (!isWritable.value) return;
  entryEditing.value = false;
  entryForm.value = emptyEntryForm();
  llmOptions.value = [];
  entryDialog.value = true;
}

function openEditEntry(row: PriceBookEntryRecord) {
  if (!isWritable.value) return;
  entryEditing.value = true;
  entryForm.value = {
    model_code: row.model_code,
    capability_type: row.capability_type || "chat",
    token_price_tiers: (row.token_price_tiers?.length ? row.token_price_tiers : [emptyTokenTier()]).map((tier) => ({ ...tier })),
    audio_tts_per_1m_chars_usd: row.audio_tts_per_1m_chars_usd || 0,
    audio_stt_per_minute_usd: row.audio_stt_per_minute_usd || 0,
    image_default_price_usd: row.image_default_price_usd || 0,
    video_default_price_usd: row.video_default_price_usd || 0,
    image_prices: (row.image_prices || []).map((price) => ({ ...price })),
    video_prices: (row.video_prices || []).map((price) => ({ ...price }))
  };
  entryDialog.value = true;
}

function applyLiteLLM(modelCode: string) {
  const model = llmOptions.value.find((option) => option.model_code === modelCode);
  if (!model) return;
  entryForm.value.model_code = model.model_code;
  entryForm.value.capability_type = model.capability_type || "chat";
  entryForm.value.token_price_tiers = model.token_price_tiers.map((tier) => ({ ...tier }));
}

async function submitEntry() {
  if (!selectedBook.value?.writable) return;
  const form = entryForm.value;
  if (!form.model_code.trim()) {
    ElMessage.warning("请填写模型名（model_code）");
    return;
  }
  if (form.capability_type === "image" && !(form.image_default_price_usd > 0)) {
    ElMessage.warning("image 能力必须填写默认价（每张 $）");
    return;
  }
  if (form.capability_type === "video" && !(form.video_default_price_usd > 0)) {
    ElMessage.warning("video 能力必须填写默认价（每秒 $）");
    return;
  }
  const tierError = isTokenPricedCapability(form.capability_type)
    ? validateTokenPriceTiers(form.token_price_tiers)
    : "";
  if (tierError) {
    ElMessage.warning(tierError);
    return;
  }

  entrySaving.value = true;
  try {
    await aiTenantApi.upsertPriceBookEntry(selectedBook.value.id, form.model_code.trim(), {
      capability_type: form.capability_type,
      token_price_tiers: form.token_price_tiers,
      audio_tts_per_1m_chars_usd: form.audio_tts_per_1m_chars_usd,
      audio_stt_per_minute_usd: form.audio_stt_per_minute_usd,
      image_default_price_usd: form.image_default_price_usd,
      video_default_price_usd: form.video_default_price_usd,
      image_prices: form.image_prices.filter((price) => price.resolution),
      video_prices: form.video_prices.filter((price) => price.resolution)
    });
    entryDialog.value = false;
    await loadEntries();
    ElMessage.success("已保存");
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "保存失败"));
  } finally {
    entrySaving.value = false;
  }
}

async function removeEntry(row: PriceBookEntryRecord) {
  if (!selectedBook.value?.writable) return;
  try {
    await ElMessageBox.confirm(`删除条目「${row.model_code}」？`, "确认删除", { type: "warning" });
    await aiTenantApi.deletePriceBookEntry(selectedBook.value.id, row.model_code, row.capability_type);
    await loadEntries();
    ElMessage.success("已删除");
  } catch (error: unknown) {
    if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error, "删除失败"));
  }
}

onMounted(() => {
  void loadRate();
  void loadBooks();
});
</script>

<template>
  <div class="page-container pricing-page">
    <PortalPagePanel
      fill
      :icon="Tags"
      :breadcrumbs="[{ label: '智能服务' }, { label: '模型与定价' }, { label: '价格表' }]"
      description="平台公共价格表只读共享，可复制为租户私有价格表后自行维护条目。"
    >
      <!-- 主从布局:body 无内边距,用 24px 容器承载汇率卡 + 价格表/条目两栏 -->
      <div class="pricing-body">
        <!-- USD → credit rate（租户只读,由平台管理员统一维护） -->
        <PortalContentCard class="rate-card">
          <div class="rate-row">
            <div class="rate-copy">
              <p class="rate-title">USD → 积分 汇率</p>
              <p class="rate-desc">对外计费时把价格表的 USD 售价换算成积分。汇率由平台管理员统一维护。</p>
            </div>
            <div class="rate-controls">
              <span class="rate-value">1 USD = {{ creditsPerUsd }} 积分</span>
            </div>
          </div>
        </PortalContentCard>

        <div class="pricing-main">
          <!-- book list -->
          <PortalContentCard title="价格表" body-padding="none" class="book-list-card">
            <template #actions>
              <el-button size="small" type="primary" :icon="Plus" @click="openCreateBook">新建</el-button>
            </template>
            <div v-loading="booksLoading" class="book-list">
              <div
                v-for="book in books"
                :key="book.id"
                class="book-item"
                :class="{ active: book.id === selectedBookId }"
                @click="selectBook(book.id)"
              >
                <div class="book-item-row">
                  <div class="book-item-copy">
                    <div class="book-title-row">
                      <span class="book-name">{{ book.name }}</span>
                      <DsTag :tone="book.owner_type === 'platform' ? 'info' : 'accent'">
                        {{ book.owner_type === "platform" ? "平台" : "私有" }}
                      </DsTag>
                    </div>
                    <div class="book-desc">{{ book.description || "—" }}</div>
                    <DsTag v-if="book.status !== 'active'" tone="neutral" class="book-status-tag">已停用</DsTag>
                  </div>
                </div>
              </div>
              <DsEmpty v-if="!books.length && !booksLoading" title="暂无价格表" description="新建一张租户私有价格表，填入模型售价">
                <template #action>
                  <el-button size="small" type="primary" :icon="Plus" @click="openCreateBook">新建价格表</el-button>
                </template>
              </DsEmpty>
            </div>
          </PortalContentCard>

          <!-- entries -->
          <PortalContentCard class="entries-card">
            <template #header>
              <div>
                <span class="font-bold">{{ selectedBook ? selectedBook.name + " · 条目" : "请选择价格表" }}</span>
                <p v-if="selectedBook" class="price-unit-note">文本/向量：USD / 1M tokens；图片：USD / 张；视频：USD / 秒</p>
              </div>
            </template>
            <template v-if="selectedBook" #actions>
              <el-button size="small" :icon="CopyDocument" @click="copyBook">复制</el-button>
              <el-button v-if="isWritable" size="small" type="success" plain :icon="Refresh" :loading="syncing" @click="syncCommon">自动同步常用模型</el-button>
              <el-button v-if="isWritable" size="small" :icon="Edit" @click="openEditBook">编辑表</el-button>
              <el-button v-if="isWritable" size="small" :icon="Delete" type="danger" plain @click="removeBook">删除表</el-button>
              <el-button v-if="isWritable" size="small" type="primary" :icon="Plus" @click="openCreateEntry">新建条目</el-button>
            </template>

            <DsTable
              :frame="false"
              :columns="entryColumns"
              :rows="entries"
              row-key="model_code"
              :loading="entriesLoading"
              expandable
            >
              <template #expand="{ row }">
                <!-- 内嵌分档明细面板:细边框小表格,不撑满整行;非 token 计费走同款容器空态 -->
                <div class="tier-panel">
                  <template v-if="row.token_price_tiers?.length">
                    <div class="tier-panel__head">
                      <span>输入上下文</span><span>输入</span><span>输出</span><span>缓存写</span><span>缓存读</span>
                    </div>
                    <div v-for="(tier, index) in row.token_price_tiers" :key="index" class="tier-panel__row">
                      <span>{{ formatTierLimit(tier.up_to_input_tokens) }}</span>
                      <span>{{ formatUsd(tier.input_per_1m_usd) }}</span>
                      <span>{{ formatUsd(tier.output_per_1m_usd) }}</span>
                      <span>{{ formatUsd(tier.cache_write_per_1m_usd) }}</span>
                      <span>{{ formatUsd(tier.cache_read_per_1m_usd) }}</span>
                    </div>
                  </template>
                  <p v-else class="tier-panel__empty">该能力不按 token 计费</p>
                </div>
              </template>

              <template #cell-standardPrice="{ row }">
                <div v-if="row.capability_type === 'image'" class="price-stack">
                  <div class="price-line">
                    <span class="price-label price-label--input">默认</span>
                    <span class="price-value price-value--input">{{ formatUsd(row.image_default_price_usd) }}/张</span>
                  </div>
                </div>
                <div v-else-if="row.capability_type === 'video'" class="price-stack">
                  <div class="price-line">
                    <span class="price-label price-label--input">默认</span>
                    <span class="price-value price-value--input">{{ formatUsd(row.video_default_price_usd) }}/秒</span>
                  </div>
                </div>
                <div v-else-if="row.token_price_tiers?.length" class="price-stack">
                  <div class="price-line">
                    <span class="price-label price-label--input">输入</span>
                    <span class="price-value price-value--input">{{ formatUsd(row.token_price_tiers[0].input_per_1m_usd) }}</span>
                  </div>
                  <div class="price-line">
                    <span class="price-label price-label--output">输出</span>
                    <span class="price-value price-value--output">{{ formatUsd(row.token_price_tiers[0].output_per_1m_usd) }}</span>
                  </div>
                  <DsTag v-if="row.token_price_tiers.length > 1" tone="accent">{{ row.token_price_tiers.length }} 档</DsTag>
                </div>
                <span v-else class="price-value price-value--fallback">—</span>
              </template>

              <template #cell-overrides="{ row }">
                <div v-if="row.capability_type === 'image' || row.capability_type === 'video'" class="price-stack">
                  <div
                    v-for="item in (row.capability_type === 'image' ? row.image_prices : row.video_prices) || []"
                    :key="`${row.model_code}-${item.resolution}`"
                    class="price-line"
                  >
                    <span class="price-label price-label--cache-read">{{ row.capability_type === "image" ? imageTierLabel(item.resolution) : item.resolution }}</span>
                    <span class="price-value price-value--cache-read">
                      {{ formatUsd(item.price) }}{{ row.capability_type === "image" ? "/张" : "/秒" }}
                    </span>
                  </div>
                  <span v-if="!((row.capability_type === 'image' ? row.image_prices : row.video_prices) || []).length" class="price-value price-value--fallback">无覆盖</span>
                </div>
                <span v-else class="price-value price-value--fallback">—</span>
              </template>

              <template #cell-actions="{ row }">
                <el-button link type="primary" size="small" @click="openEditEntry(row)">编辑</el-button>
                <el-button link type="danger" size="small" @click="removeEntry(row)">删除</el-button>
              </template>

              <template #empty>
                <DsEmpty
                  :title="selectedBook ? '暂无条目' : '请选择价格表'"
                  :description="selectedBook ? (isWritable ? '可手动新建条目，或点击「自动同步常用模型」批量导入' : '平台公共价格表暂无条目，可复制为私有表后维护') : '先在左侧选择一个价格表查看条目'"
                >
                  <template v-if="selectedBook && isWritable" #action>
                    <el-button size="small" type="primary" :icon="Plus" @click="openCreateEntry">新建条目</el-button>
                  </template>
                </DsEmpty>
              </template>
            </DsTable>
          </PortalContentCard>
        </div>
      </div>
    </PortalPagePanel>

    <!-- Price book dialog -->
    <el-dialog v-model="bookDialog" :title="bookEditing ? '编辑价格表' : '新建价格表'" width="460px">
      <el-form label-width="80px">
        <el-form-item label="名称" required>
          <el-input v-model="bookForm.name" placeholder="如：标准价 / 优惠价" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="bookForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item v-if="bookEditing" label="状态">
          <el-radio-group v-model="bookForm.status">
            <el-radio value="active">启用</el-radio>
            <el-radio value="disabled">停用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bookDialog = false">取消</el-button>
        <el-button type="primary" :loading="bookSaving" @click="submitBook">保存</el-button>
      </template>
    </el-dialog>

    <PriceBookEntryFormDialog
      v-model:visible="entryDialog"
      v-model:form="entryForm"
      :editing="entryEditing"
      :lite-l-l-m-options="llmOptions"
      :lite-l-l-m-loading="llmSearching"
      @search-lite-l-l-m="searchLiteLLM"
      @apply-lite-l-l-m="applyLiteLLM"
      @submit="submitEntry"
    />
  </div>
</template>

<style scoped>
.pricing-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 主从布局:PortalPagePanel body 无内边距,用 24px 容器排布汇率卡与两栏;fill 模式下随面板撑满视口 */
.pricing-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
  flex: 1;
  min-height: 0;
}

.pricing-main {
  display: grid;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  gap: 20px;
  flex: 1;
  min-height: 0;
  align-items: stretch;
}

/* 两张卡片随 grid 行拉伸,内部 flex 列让列表/表格区吃掉剩余高度 */
.book-list-card,
.entries-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.book-list-card :deep(.portal-content-card__body),
.entries-card :deep(.portal-content-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.entries-card :deep(.ds-table) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 条目空态在表格剩余区域纵向居中 */
.entries-card :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

@media (max-width: 1200px) {
  .pricing-main {
    grid-template-columns: 1fr;
  }
}

.rate-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}

.rate-copy {
  min-width: 0;
}

.rate-title {
  margin: 0;
  font-size: 13px;
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.rate-desc {
  margin: 3px 0 0;
  font-size: 12px;
  color: var(--ds-faint);
}

.rate-controls {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.rate-value {
  font-size: 18px;
  font-weight: 700;
  color: var(--ds-ink);
}

.book-list { flex: 1; min-height: 0; overflow-y: auto; padding: 8px 12px; }
.book-item {
  padding: 10px 12px;
  border-radius: var(--ds-radius-control);
  cursor: pointer;
  border: 1px solid transparent;
  margin-bottom: 6px;
}
.book-item:hover { background: var(--ds-panel-muted); }
.book-item.active { background: var(--ds-accent-soft); border-color: color-mix(in srgb, var(--ds-accent) 40%, var(--ds-line)); }
.book-item-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.book-item-copy { min-width: 0; flex: 1; }
.book-title-row { display: flex; align-items: center; gap: 8px; min-width: 0; }
.book-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 700;
  color: var(--ds-ink);
}
.book-desc {
  margin-top: 2px;
  font-size: 12px;
  color: var(--ds-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.book-status-tag { margin-top: 6px; }
.price-unit-note {
  margin-top: 3px;
  color: var(--ds-faint);
  font-size: 12px;
  font-weight: 500;
}
.price-stack {
  display: grid;
  gap: 3px;
  padding: 2px 0;
}
/* 展开区:覆盖 DsTable 默认灰底为白底(仅本页),留白与主表格单元格(10px 16px)对齐 */
:deep(.ds-table__expand-cell) {
  padding: 14px 16px 16px;
  background: var(--ds-panel);
}
/* 内嵌分档明细小表格:圆角细边框,限宽 860px,作为主表格的从属明细面板 */
.tier-panel {
  max-width: 860px;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
}
.tier-panel__head,
.tier-panel__row {
  display: grid;
  grid-template-columns: minmax(130px, 1.2fr) repeat(4, minmax(90px, 1fr));
  gap: 12px;
  padding: 8px 14px;
}
.tier-panel__head {
  border-bottom: 1px solid var(--ds-line);
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}
.tier-panel__row {
  color: var(--ds-ink-soft);
  font-variant-numeric: tabular-nums;
}
.tier-panel__row + .tier-panel__row {
  border-top: 1px solid var(--ds-line);
}
.tier-panel__row:hover {
  background: var(--ds-panel-muted);
}
.tier-panel__empty {
  margin: 0;
  padding: 12px 14px;
  color: var(--ds-faint);
  font-size: 12.5px;
  font-weight: 600;
}
.price-line {
  display: grid;
  grid-template-columns: 34px minmax(64px, 1fr);
  align-items: center;
  column-gap: 8px;
}
.price-label {
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}
.price-value {
  color: var(--ds-ink);
  font-variant-numeric: tabular-nums;
  font-weight: 650;
  white-space: nowrap;
}
.price-label--input,
.price-value--input {
  color: var(--ds-info);
}
.price-label--output,
.price-value--output {
  color: var(--ds-accent);
}
.price-label--cache-read,
.price-value--cache-read {
  color: var(--ds-positive);
}
.price-value--fallback {
  color: var(--ds-faint);
  font-weight: 600;
}
</style>
