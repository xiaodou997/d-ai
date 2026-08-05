<!--
  价格表 — 维护各能力模型的 USD 售价（token 分档 / 图片 / 视频 / 音频）与 USD→积分汇率。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       汇率卡/价格表列表/条目表收进同卡 body 的 24px 容器）;条目表 el-table → DsTable
       （expandable 展开 token 分档）,el-tag → DsTag,空态 → DsEmpty;价格色值改用 --ds-* token,
       无硬编码 hex;业务逻辑与请求参数保持不变。
       ⚠ 条目接口无服务端分页,故不加 DsPagination。弹窗/表单仍为 element-plus。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, ref, shallowRef } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Download, Edit, Plus, Refresh, Upload } from '@element-plus/icons-vue'
import { Tags } from 'lucide-vue-next'
import { PortalContentCard, PortalPagePanel } from '@dai/app-core'
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from '@dai/ui'
import { aiAdminApi } from '../../../api/aiAdmin'
import PriceBookEntryFormDialog from './components/PriceBookEntryFormDialog.vue'
import { createPriceBookFile, parsePriceBookFile } from './pricingFile'
import {
  isTokenPricedCapability,
  validateTokenPriceTiers,
  type LiteLLMPriceModel,
  type PriceBookEntryForm,
  type PriceBookEntryRecord,
  type PriceBookRecord,
  type TokenPriceTier
} from './pricingTypes'

const formatTimestamp = (value: any) => {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

const formatUsd2 = (value: any) => Number(value ?? 0).toFixed(2)
const formatUsd = (value: any) => `$${formatUsd2(value)}`
const formatTierLimit = (limit: number | null) => limit === null ? '无上限' : limit.toLocaleString('zh-CN')
const imageTierLabel = (value: string) => ({ '1k': '1K', '2k': '2K', '4k': '4K' }[value] || value)

// ── Rate (USD→credit) ─────────────────────────────────────────────────────
const DEFAULT_CREDITS_PER_USD = 100
const MAX_CREDITS_PER_USD = 1_000_000
const creditsPerUsd = shallowRef(DEFAULT_CREDITS_PER_USD)
const rateDraft = shallowRef(DEFAULT_CREDITS_PER_USD)
const rateEditing = shallowRef(false)
const rateSaving = shallowRef(false)

async function loadRate() {
  try {
    const res: any = await aiAdminApi.getCreditsPerUSD()
    creditsPerUsd.value = res?.credits_per_usd ?? DEFAULT_CREDITS_PER_USD
    rateDraft.value = creditsPerUsd.value
  } catch (e) { /* 默认值兜底 */ }
}

async function saveRate() {
  if (!(rateDraft.value > 0) || rateDraft.value > MAX_CREDITS_PER_USD) {
    ElMessage.warning(`汇率必须大于 0 且不超过 ${MAX_CREDITS_PER_USD.toLocaleString('zh-CN')}`)
    return
  }
  rateSaving.value = true
  try {
    await aiAdminApi.setCreditsPerUSD(rateDraft.value)
    creditsPerUsd.value = rateDraft.value
    rateEditing.value = false
    ElMessage.success('汇率已更新')
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    rateSaving.value = false
  }
}

// ── Price books ─────────────────────────────────────────────────────────────
const books = shallowRef<PriceBookRecord[]>([])
const booksLoading = shallowRef(false)
const selectedBookId = shallowRef('')
const selectedBook = computed(() => books.value.find((book) => book.id === selectedBookId.value) || null)

async function loadBooks() {
  booksLoading.value = true
  try {
    const res = await aiAdminApi.listPriceBooks()
    books.value = res.items || []
    if (!selectedBookId.value && books.value.length) {
      selectBook(books.value[0].id)
    } else if (selectedBookId.value) {
      loadEntries()
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '加载价格表失败')
  } finally {
    booksLoading.value = false
  }
}

function selectBook(id: string) {
  selectedBookId.value = id
  loadEntries()
}

// Book dialog
const bookDialog = shallowRef(false)
const bookEditing = shallowRef(false)
const bookForm = reactive({ id: '', name: '', description: '', status: 'active' })

function openCreateBook() {
  bookEditing.value = false
  Object.assign(bookForm, { id: '', name: '', description: '', status: 'active' })
  bookDialog.value = true
}

function openEditBook() {
  if (!selectedBook.value) return
  bookEditing.value = true
  Object.assign(bookForm, {
    id: selectedBook.value.id,
    name: selectedBook.value.name,
    description: selectedBook.value.description || '',
    status: selectedBook.value.status || 'active'
  })
  bookDialog.value = true
}

async function submitBook() {
  if (!bookForm.name.trim()) {
    ElMessage.warning('请填写价格表名称')
    return
  }
  try {
    if (bookEditing.value) {
      await aiAdminApi.updatePriceBook(bookForm.id, { name: bookForm.name, description: bookForm.description, status: bookForm.status as any })
      ElMessage.success('已保存')
    } else {
      const created: any = await aiAdminApi.createPriceBook({ name: bookForm.name, description: bookForm.description })
      selectedBookId.value = created?.id || ''
      ElMessage.success('已创建')
    }
    bookDialog.value = false
    await loadBooks()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function removeBook() {
  if (!selectedBook.value) return
  await ElMessageBox.confirm(`删除价格表「${selectedBook.value.name}」？其下所有条目将一并删除。`, '确认删除', { type: 'warning' })
  try {
    await aiAdminApi.deletePriceBook(selectedBook.value.id)
    ElMessage.success('已删除')
    selectedBookId.value = ''
    await loadBooks()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

// ── Entries ───────────────────────────────────────────────────────────────
const entries = shallowRef<PriceBookEntryRecord[]>([])
const entriesLoading = shallowRef(false)

// DsTable 列:model_code 为标识符用 mono;标准价格/规格覆盖/操作走 #cell-* 插槽
const entryColumns: DsTableColumn[] = [
  { key: 'model_code', title: '模型', width: 220, mono: true },
  { key: 'capability_type', title: '能力', width: 90 },
  { key: 'standardPrice', title: '标准价格', width: 150 },
  { key: 'overrides', title: '规格覆盖' },
  { key: 'actions', title: '操作', width: 120 }
]

async function loadEntries() {
  if (!selectedBookId.value) { entries.value = []; return }
  entriesLoading.value = true
  try {
    const res = await aiAdminApi.listPriceBookEntries(selectedBookId.value)
    entries.value = res.items || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载条目失败')
  } finally {
    entriesLoading.value = false
  }
}

// 一键同步常用模型（白名单）
const syncing = shallowRef(false)
async function syncCommon() {
  if (!selectedBookId.value) return
  await ElMessageBox.confirm('自动获取一批常用模型（Claude / GPT / Gemini 等）的价格并填入本价格表？已手动编辑的条目不会被覆盖。', '同步常用模型', { type: 'info' })
  syncing.value = true
  try {
    const res: any = await aiAdminApi.syncCommonModels(selectedBookId.value)
    const missing = res?.missing?.length ? `，未找到 ${res.missing.length} 个` : ''
    ElMessage.success(`已同步 ${res?.synced ?? 0} 个常用模型${missing}`)
    await loadEntries()
  } catch (e: any) {
    ElMessage.error(e?.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

// LiteLLM 远程搜索填充（按需，不全量入库）
const llmSearching = shallowRef(false)
const llmOptions = shallowRef<LiteLLMPriceModel[]>([])

async function searchLiteLLM(query: string) {
  llmSearching.value = true
  try {
    const res = await aiAdminApi.searchLiteLLMModels(query || '', 50)
    llmOptions.value = res.items || []
  } catch (e) {
    llmOptions.value = []
  } finally {
    llmSearching.value = false
  }
}

function applyLiteLLM(modelCode: string) {
  const m = llmOptions.value.find((option) => option.model_code === modelCode)
  if (!m) return
  entryForm.value.model_code = m.model_code
  entryForm.value.capability_type = m.capability_type || 'chat'
  entryForm.value.token_price_tiers = m.token_price_tiers.map((tier) => ({ ...tier }))
}

// Entry dialog
const entryDialog = shallowRef(false)
const entryEditing = shallowRef(false)
const entryForm = ref<PriceBookEntryForm>(emptyEntryForm())

function emptyEntryForm(): PriceBookEntryForm {
  return {
    model_code: '',
    capability_type: 'chat',
    token_price_tiers: [emptyTokenTier()],
    audio_tts_per_1m_chars_usd: 0,
    audio_stt_per_minute_usd: 0,
    image_default_price_usd: 0,
    video_default_price_usd: 0,
    image_prices: [],
    video_prices: []
  }
}

function emptyTokenTier(): TokenPriceTier {
  return { up_to_input_tokens: null, input_per_1m_usd: 0, output_per_1m_usd: 0, cache_write_per_1m_usd: 0, cache_read_per_1m_usd: 0 }
}

function openCreateEntry() {
  entryEditing.value = false
  entryForm.value = emptyEntryForm()
  llmOptions.value = []
  entryDialog.value = true
}

function openEditEntry(row: PriceBookEntryRecord) {
  entryEditing.value = true
  entryForm.value = {
    model_code: row.model_code,
    capability_type: row.capability_type || 'chat',
    token_price_tiers: (row.token_price_tiers || [emptyTokenTier()]).map((tier) => ({ ...tier })),
    audio_tts_per_1m_chars_usd: row.audio_tts_per_1m_chars_usd || 0,
    audio_stt_per_minute_usd: row.audio_stt_per_minute_usd || 0,
    image_default_price_usd: row.image_default_price_usd || 0,
    video_default_price_usd: row.video_default_price_usd || 0,
    image_prices: (row.image_prices || []).map((price) => ({ ...price })),
    video_prices: (row.video_prices || []).map((price) => ({ ...price }))
  }
  entryDialog.value = true
}

async function submitEntry() {
  const form = entryForm.value
  if (!form.model_code.trim()) {
    ElMessage.warning('请填写模型名（model_code）')
    return
  }
  if (form.capability_type === 'image' && !(form.image_default_price_usd > 0)) {
    ElMessage.warning('image 能力必须填写默认价（每张 $）')
    return
  }
  if (form.capability_type === 'video' && !(form.video_default_price_usd > 0)) {
    ElMessage.warning('video 能力必须填写默认价（每秒 $）')
    return
  }
  const tierError = isTokenPricedCapability(form.capability_type)
    ? validateTokenPriceTiers(form.token_price_tiers)
    : ''
  if (tierError) {
    ElMessage.warning(tierError)
    return
  }
  const modelCode = form.model_code.trim()
  const payload = {
    capability_type: form.capability_type,
    token_price_tiers: form.token_price_tiers,
    audio_tts_per_1m_chars_usd: form.audio_tts_per_1m_chars_usd,
    audio_stt_per_minute_usd: form.audio_stt_per_minute_usd,
    image_default_price_usd: form.image_default_price_usd,
    video_default_price_usd: form.video_default_price_usd,
    image_prices: form.image_prices.filter((price) => price.resolution),
    video_prices: form.video_prices.filter((price) => price.resolution)
  }
  try {
    await aiAdminApi.upsertPriceBookEntry(selectedBookId.value, modelCode, payload)
    ElMessage.success('已保存')
    entryDialog.value = false
    await loadEntries()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function removeEntry(row: any) {
  await ElMessageBox.confirm(`删除条目「${row.model_code}」？`, '确认删除', { type: 'warning' })
  try {
    await aiAdminApi.deletePriceBookEntry(selectedBookId.value, row.model_code)
    ElMessage.success('已删除')
    await loadEntries()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

async function exportBook(book: any) {
  try {
    const [bookRes, entriesRes] = await Promise.all([
      aiAdminApi.getPriceBook(book.id),
      aiAdminApi.listPriceBookEntries(book.id)
    ])
    const data = createPriceBookFile(bookRes, entriesRes.items || [])
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `price-book-${book.name}-${new Date().toISOString().slice(0, 10)}.json`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
    ElMessage.success(`已导出「${book.name}」(${data.entries.length} 条)`)
  } catch (e: any) {
    ElMessage.error(e?.message || '导出失败')
  }
}

// ── 导入价格表 ─────────────────────────────────────────────────────────────
const importFileInput = ref<HTMLInputElement | null>(null)
const importing = shallowRef(false)

function openImportBook() {
  importFileInput.value?.click()
}

async function handleImportFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  let data: any
  try {
    data = JSON.parse(await file.text())
  } catch {
    ElMessage.error('文件解析失败，请选择有效的导出 JSON 文件')
    return
  }
  const parsed = parsePriceBookFile(data)
  if (!parsed.data) {
    ElMessage.error(parsed.error || '文件格式不正确')
    return
  }
	data = parsed.data

  const baseName = String(data.price_book.name || '').trim() || file.name.replace(/\.json$/i, '')
  const description = String(data.price_book.description || '')
  const entries: any[] = data.entries
  if (!entries.length) {
    ElMessage.warning('文件中没有条目，无需导入')
    return
  }

  // 重名自动加后缀
  const existingNames = new Set(books.value.map((b: any) => b.name))
  let finalName = baseName
  let suffix = 1
  while (existingNames.has(finalName)) {
    finalName = `${baseName} (导入${suffix})`
    suffix++
  }

  try {
    await ElMessageBox.confirm(
      `将创建新价格表「${finalName}」并导入 ${entries.length} 条条目。同名模型将被覆盖。`,
      '确认导入',
      { type: 'info' }
    )
  } catch {
    return
  }

  importing.value = true
  try {
    const created: any = await aiAdminApi.createPriceBook({ name: finalName, description })
    const newBookId = created?.id
    if (!newBookId) throw new Error('创建价格表失败')

    let success = 0
    let failed = 0
    for (const e of entries) {
      const modelCode = String(e.model_code || '').trim()
      if (!modelCode) { failed++; continue }
      try {
        await aiAdminApi.upsertPriceBookEntry(newBookId, modelCode, {
          capability_type: e.capability_type || 'chat',
          token_price_tiers: Array.isArray(e.token_price_tiers) ? e.token_price_tiers : [],
          audio_tts_per_1m_chars_usd: Number(e.audio_tts_per_1m_chars_usd) || 0,
          audio_stt_per_minute_usd: Number(e.audio_stt_per_minute_usd) || 0,
          image_default_price_usd: Number(e.image_default_price_usd) || 0,
          video_default_price_usd: Number(e.video_default_price_usd) || 0,
          image_prices: Array.isArray(e.image_prices) ? e.image_prices : [],
          video_prices: Array.isArray(e.video_prices) ? e.video_prices : []
        })
        success++
      } catch {
        failed++
      }
    }

    await loadBooks()
    selectBook(newBookId)

    if (failed) {
      ElMessage.warning(`已导入「${finalName}」：成功 ${success} 条，失败 ${failed} 条`)
    } else {
      ElMessage.success(`已导入「${finalName}」：成功 ${success} 条`)
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '导入失败')
  } finally {
    importing.value = false
  }
}

// 注：平台→租户、租户→用户 的售价倍率绑定已迁移到「租户↔分组」页（按分组维度），本页只管价格表与汇率。

onMounted(() => {
  loadRate()
  loadBooks()
})
</script>

<template>
  <div class="page-container pricing-page">
    <PortalPagePanel
      fill
      :icon="Tags"
      :breadcrumbs="[{ label: '智能服务' }, { label: '网关配置' }, { label: '价格表' }]"
      description="维护模型 USD 售价与 USD → 积分汇率。"
    >
      <!-- 主从布局:body 无内边距,用 24px 容器承载汇率卡 + 价格表/条目两栏 -->
      <div class="pricing-body">
        <!-- USD → credit rate -->
        <PortalContentCard class="rate-card">
          <div class="rate-row">
            <div class="rate-copy">
              <p class="rate-title">USD → 积分 汇率</p>
              <p class="rate-desc">对外计费时把价格表的 USD 售价换算成积分。仅作用于售价侧，上游成本仍以 USD 核算。</p>
            </div>
            <div class="rate-controls">
              <template v-if="!rateEditing">
                <span class="rate-value">1 USD = {{ creditsPerUsd }} 积分</span>
                <el-button size="small" :icon="Edit" @click="rateEditing = true">修改</el-button>
              </template>
              <template v-else>
                <span class="rate-hint-text">1 USD =</span>
                <el-input-number v-model="rateDraft" :min="0.0001" :max="MAX_CREDITS_PER_USD" :step="1" :precision="4" :controls="false" size="small" style="width: 140px" />
                <span class="rate-hint-text">积分</span>
                <el-button size="small" type="primary" :loading="rateSaving" @click="saveRate">保存</el-button>
                <el-button size="small" @click="rateEditing = false; rateDraft = creditsPerUsd">取消</el-button>
              </template>
            </div>
          </div>
        </PortalContentCard>

        <div class="pricing-main">
          <!-- book list -->
          <PortalContentCard title="价格表" body-padding="none" class="book-list-card">
            <template #actions>
              <el-button size="small" :icon="Upload" :loading="importing" @click="openImportBook">导入</el-button>
              <el-button size="small" type="primary" :icon="Plus" @click="openCreateBook">新建</el-button>
            </template>
            <input ref="importFileInput" type="file" accept=".json,application/json" class="hidden-file-input" @change="handleImportFile" />
            <div v-loading="booksLoading" class="book-list">
              <div
                v-for="b in books"
                :key="b.id"
                class="book-item"
                :class="{ active: b.id === selectedBookId }"
                @click="selectBook(b.id)"
              >
                <div class="book-item-row">
                  <div class="book-item-copy">
                    <div class="font-bold text-slate-800">{{ b.name }}</div>
                    <div class="text-xs text-slate-400 truncate">{{ b.description || '—' }}</div>
                    <DsTag v-if="b.status !== 'active'" tone="info" class="mt-1">已停用</DsTag>
                  </div>
                  <el-button link type="primary" :icon="Download" size="small" title="导出价格表" @click.stop="exportBook(b)" />
                </div>
              </div>
              <DsEmpty v-if="!books.length && !booksLoading" title="暂无价格表" description="新建一个价格表，或导入已导出的 JSON 文件">
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
                <span class="font-bold">{{ selectedBook ? selectedBook.name + ' · 条目' : '请选择价格表' }}</span>
                <p v-if="selectedBook" class="price-unit-note">文本/向量：USD / 1M tokens；图片：USD / 张；视频：USD / 秒</p>
              </div>
            </template>
            <template v-if="selectedBook" #actions>
              <el-button size="small" type="success" plain :icon="Refresh" :loading="syncing" @click="syncCommon">自动同步常用模型</el-button>
              <el-button size="small" :icon="Edit" @click="openEditBook">编辑表</el-button>
              <el-button size="small" :icon="Delete" type="danger" plain @click="removeBook">删除表</el-button>
              <el-button size="small" type="primary" :icon="Plus" @click="openCreateEntry">新建条目</el-button>
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
                    <span class="price-label price-label--cache-read">{{ row.capability_type === 'image' ? imageTierLabel(item.resolution) : item.resolution }}</span>
                    <span class="price-value price-value--cache-read">
                      {{ formatUsd(item.price) }}{{ row.capability_type === 'image' ? '/张' : '/秒' }}
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
                  :description="selectedBook ? '可手动新建条目，或点击「自动同步常用模型」批量导入' : '先在左侧选择一个价格表查看条目'"
                >
                  <template v-if="selectedBook" #action>
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
          <el-input v-model="bookForm.name" placeholder="如：标准价 / 中转便宜价" />
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
        <el-button type="primary" @click="submitBook">保存</el-button>
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

.rate-hint-text {
  color: var(--ds-ink-soft);
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
.hidden-file-input { display: none; }
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
