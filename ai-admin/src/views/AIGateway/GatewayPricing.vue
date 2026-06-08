<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh, Search } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createPriceBook,
  deletePriceBook,
  deletePriceBookEntry,
  deleteTenantSellBinding,
  deleteUserSellBinding,
  formatTimestamp,
  getCreditsPerUSD,
  getUserSellBinding,
  listPriceBookEntries,
  listPriceBooks,
  listTenantSellBindings,
  searchLiteLLMModels,
  syncCommonModels,
  setCreditsPerUSD,
  updatePriceBook,
  upsertPriceBookEntry,
  upsertTenantSellBinding,
  upsertUserSellBinding
} from '@/api/aiGateway'

// ── Rate (USD→credit) ─────────────────────────────────────────────────────
const creditsPerUsd = shallowRef(7)
const rateDraft = shallowRef(7)
const rateEditing = shallowRef(false)
const rateSaving = shallowRef(false)

async function loadRate() {
  try {
    const res = await getCreditsPerUSD()
    creditsPerUsd.value = res?.credits_per_usd ?? 7
    rateDraft.value = creditsPerUsd.value
  } catch (e) { /* interceptor toasts */ }
}

async function saveRate() {
  if (!(rateDraft.value > 0)) {
    ElMessage.warning('汇率必须为正数')
    return
  }
  rateSaving.value = true
  try {
    await setCreditsPerUSD(rateDraft.value)
    creditsPerUsd.value = rateDraft.value
    rateEditing.value = false
    ElMessage.success('汇率已更新')
  } finally {
    rateSaving.value = false
  }
}

// ── Price books ─────────────────────────────────────────────────────────────
const books = shallowRef([])
const booksLoading = shallowRef(false)
const selectedBookId = shallowRef('')
const selectedBook = computed(() => books.value.find(b => b.id === selectedBookId.value) || null)

async function loadBooks() {
  booksLoading.value = true
  try {
    const res = await listPriceBooks()
    books.value = Array.isArray(res) ? res : []
    if (!selectedBookId.value && books.value.length) {
      selectBook(books.value[0].id)
    } else if (selectedBookId.value) {
      // refresh entries of current selection
      loadEntries()
    }
  } finally {
    booksLoading.value = false
  }
}

function selectBook(id) {
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
  if (bookEditing.value) {
    await updatePriceBook(bookForm.id, { name: bookForm.name, description: bookForm.description, status: bookForm.status })
    ElMessage.success('已保存')
  } else {
    const created = await createPriceBook({ name: bookForm.name, description: bookForm.description })
    selectedBookId.value = created?.id || ''
    ElMessage.success('已创建')
  }
  bookDialog.value = false
  await loadBooks()
}

async function removeBook() {
  if (!selectedBook.value) return
  await ElMessageBox.confirm(`删除价格表「${selectedBook.value.name}」？其下所有条目将一并删除。`, '确认删除', { type: 'warning' })
  await deletePriceBook(selectedBook.value.id)
  ElMessage.success('已删除')
  selectedBookId.value = ''
  await loadBooks()
}

// ── Entries ───────────────────────────────────────────────────────────────
const entries = shallowRef([])
const entriesLoading = shallowRef(false)

async function loadEntries() {
  if (!selectedBookId.value) { entries.value = []; return }
  entriesLoading.value = true
  try {
    const res = await listPriceBookEntries(selectedBookId.value)
    entries.value = Array.isArray(res) ? res : []
  } finally {
    entriesLoading.value = false
  }
}

// 一键同步常用模型（白名单）
const syncing = shallowRef(false)
async function syncCommon() {
  if (!selectedBookId.value) return
  await ElMessageBox.confirm('从 LiteLLM 拉取一批常用模型（Claude / GPT / Gemini 等）的价格填入本价格表？已手动编辑的条目不会被覆盖。', '同步常用模型', { type: 'info' })
  syncing.value = true
  try {
    const res = await syncCommonModels(selectedBookId.value)
    const missing = res?.missing?.length ? `，未找到 ${res.missing.length} 个` : ''
    ElMessage.success(`已同步 ${res?.synced ?? 0} 个常用模型${missing}`)
    await loadEntries()
  } finally {
    syncing.value = false
  }
}

// LiteLLM 远程搜索填充（按需，不全量入库）
const llmSearching = shallowRef(false)
const llmOptions = shallowRef([])

async function searchLiteLLM(query) {
  llmSearching.value = true
  try {
    const res = await searchLiteLLMModels(query || '', 50)
    llmOptions.value = Array.isArray(res) ? res : []
  } catch (e) {
    llmOptions.value = []
  } finally {
    llmSearching.value = false
  }
}

function applyLiteLLM(modelCode) {
  const m = llmOptions.value.find(o => o.model_code === modelCode)
  if (!m) return
  entryForm.model_code = m.model_code
  entryForm.capability_type = m.capability_type || 'chat'
  entryForm.input_per_1m_usd = m.input_per_1m_usd || 0
  entryForm.output_per_1m_usd = m.output_per_1m_usd || 0
  entryForm.cache_write_per_1m_usd = m.cache_write_per_1m_usd || 0
  entryForm.cache_read_per_1m_usd = m.cache_read_per_1m_usd || 0
}

// Entry dialog
const entryDialog = shallowRef(false)
const entryEditing = shallowRef(false)
const entryForm = reactive(emptyEntryForm())

function emptyEntryForm() {
  return {
    model_code: '',
    capability_type: 'chat',
    input_per_1m_usd: 0,
    output_per_1m_usd: 0,
    cache_write_per_1m_usd: 0,
    cache_read_per_1m_usd: 0,
    reasoning_per_1m_usd: 0,
    audio_tts_per_1m_chars_usd: 0,
    audio_stt_per_minute_usd: 0,
    image_prices: [],
    video_prices: []
  }
}

function openCreateEntry() {
  entryEditing.value = false
  Object.assign(entryForm, emptyEntryForm())
  llmOptions.value = []
  entryDialog.value = true
}

function openEditEntry(row) {
  entryEditing.value = true
  Object.assign(entryForm, {
    model_code: row.model_code,
    capability_type: row.capability_type || 'chat',
    input_per_1m_usd: row.input_per_1m_usd || 0,
    output_per_1m_usd: row.output_per_1m_usd || 0,
    cache_write_per_1m_usd: row.cache_write_per_1m_usd || 0,
    cache_read_per_1m_usd: row.cache_read_per_1m_usd || 0,
    reasoning_per_1m_usd: row.reasoning_per_1m_usd || 0,
    audio_tts_per_1m_chars_usd: row.audio_tts_per_1m_chars_usd || 0,
    audio_stt_per_minute_usd: row.audio_stt_per_minute_usd || 0,
    image_prices: (row.image_prices || []).map(p => ({ ...p })),
    video_prices: (row.video_prices || []).map(p => ({ ...p }))
  })
  entryDialog.value = true
}

const isTokenCap = computed(() => ['chat', 'embedding', 'rerank'].includes(entryForm.capability_type))
const isImageCap = computed(() => entryForm.capability_type === 'image')
const isVideoCap = computed(() => entryForm.capability_type === 'video')
const isAudioCap = computed(() => entryForm.capability_type === 'audio_tts' || entryForm.capability_type === 'audio_stt')

function addResolution(list) { list.push({ resolution: '', price: 0 }) }
function removeResolution(list, i) { list.splice(i, 1) }

async function submitEntry() {
  if (!entryForm.model_code.trim()) {
    ElMessage.warning('请填写模型名（model_code）')
    return
  }
  const payload = {
    model_code: entryForm.model_code.trim(),
    capability_type: entryForm.capability_type,
    input_per_1m_usd: entryForm.input_per_1m_usd,
    output_per_1m_usd: entryForm.output_per_1m_usd,
    cache_write_per_1m_usd: entryForm.cache_write_per_1m_usd,
    cache_read_per_1m_usd: entryForm.cache_read_per_1m_usd,
    reasoning_per_1m_usd: entryForm.reasoning_per_1m_usd,
    audio_tts_per_1m_chars_usd: entryForm.audio_tts_per_1m_chars_usd,
    audio_stt_per_minute_usd: entryForm.audio_stt_per_minute_usd,
    image_prices: entryForm.image_prices.filter(p => p.resolution),
    video_prices: entryForm.video_prices.filter(p => p.resolution)
  }
  await upsertPriceBookEntry(selectedBookId.value, payload)
  ElMessage.success('已保存')
  entryDialog.value = false
  await loadEntries()
}

async function removeEntry(row) {
  await ElMessageBox.confirm(`删除条目「${row.model_code}」？`, '确认删除', { type: 'warning' })
  await deletePriceBookEntry(selectedBookId.value, row.model_code)
  ElMessage.success('已删除')
  await loadEntries()
}

// ── Tenant sell bindings ────────────────────────────────────────────────────
const bindings = shallowRef([])
const bindingsLoading = shallowRef(false)

async function loadBindings() {
  bindingsLoading.value = true
  try {
    const res = await listTenantSellBindings()
    bindings.value = Array.isArray(res) ? res : []
  } finally {
    bindingsLoading.value = false
  }
}

const bindingDialog = shallowRef(false)
const bindingEditing = shallowRef(false)
const bindingForm = reactive({ tenant_id: '', price_book_id: '', sell_multiplier: 1, cache_billing_enabled: false })

function openCreateBinding() {
  bindingEditing.value = false
  Object.assign(bindingForm, { tenant_id: '', price_book_id: selectedBookId.value || '', sell_multiplier: 1, cache_billing_enabled: false })
  bindingDialog.value = true
}

function openEditBinding(row) {
  bindingEditing.value = true
  Object.assign(bindingForm, {
    tenant_id: row.tenant_id,
    price_book_id: row.price_book_id,
    sell_multiplier: row.sell_multiplier ?? 1,
    cache_billing_enabled: !!row.cache_billing_enabled
  })
  bindingDialog.value = true
}

async function submitBinding() {
  if (!bindingForm.tenant_id.trim()) { ElMessage.warning('请填写租户 ID'); return }
  if (!bindingForm.price_book_id) { ElMessage.warning('请选择价格表'); return }
  await upsertTenantSellBinding(bindingForm.tenant_id.trim(), {
    price_book_id: bindingForm.price_book_id,
    sell_multiplier: bindingForm.sell_multiplier,
    cache_billing_enabled: bindingForm.cache_billing_enabled
  })
  ElMessage.success('已保存')
  bindingDialog.value = false
  await loadBindings()
}

async function removeBinding(row) {
  await ElMessageBox.confirm(`删除租户「${row.tenant_id}」的售价绑定？该租户的请求将因无售价被拒绝。`, '确认删除', { type: 'warning' })
  await deleteTenantSellBinding(row.tenant_id)
  ElMessage.success('已删除')
  await loadBindings()
}

// User sell binding (cascade)
const userBindingDialog = shallowRef(false)
const userBindingForm = reactive({ tenant_id: '', user_multiplier: 1, cache_billing_enabled: false, exists: false })

async function openUserBinding(row) {
  Object.assign(userBindingForm, { tenant_id: row.tenant_id, user_multiplier: 1, cache_billing_enabled: false, exists: false })
  try {
    const ub = await getUserSellBinding(row.tenant_id)
    if (ub) {
      userBindingForm.user_multiplier = ub.user_multiplier ?? 1
      userBindingForm.cache_billing_enabled = !!ub.cache_billing_enabled
      userBindingForm.exists = true
    }
  } catch (e) { /* none */ }
  userBindingDialog.value = true
}

async function submitUserBinding() {
  await upsertUserSellBinding(userBindingForm.tenant_id, {
    user_multiplier: userBindingForm.user_multiplier,
    cache_billing_enabled: userBindingForm.cache_billing_enabled
  })
  ElMessage.success('已保存')
  userBindingDialog.value = false
}

async function removeUserBinding() {
  await deleteUserSellBinding(userBindingForm.tenant_id)
  ElMessage.success('已删除')
  userBindingDialog.value = false
}

const activeTab = shallowRef('books')

onMounted(() => {
  loadRate()
  loadBooks()
  loadBindings()
})
</script>

<template>
  <div class="space-y-5">
    <!-- USD → credit rate -->
    <el-card shadow="never" class="rate-card">
      <div class="flex items-center justify-between flex-wrap gap-3">
        <div>
          <p class="text-sm text-slate-500 font-bold">USD → 积分 汇率</p>
          <p class="text-xs text-slate-400 mt-1">对外计费时把价格表的 USD 售价换算成积分。仅作用于售价侧，上游成本仍以 USD 核算。</p>
        </div>
        <div class="flex items-center gap-3">
          <template v-if="!rateEditing">
            <span class="text-lg font-black text-slate-800">1 USD = {{ creditsPerUsd }} 积分</span>
            <el-button size="small" :icon="Edit" @click="rateEditing = true">修改</el-button>
          </template>
          <template v-else>
            <span class="text-slate-600">1 USD =</span>
            <el-input-number v-model="rateDraft" :min="0.0001" :step="1" :precision="4" size="small" style="width: 140px" />
            <span class="text-slate-600">积分</span>
            <el-button size="small" type="primary" :loading="rateSaving" @click="saveRate">保存</el-button>
            <el-button size="small" @click="rateEditing = false; rateDraft = creditsPerUsd">取消</el-button>
          </template>
        </div>
      </div>
    </el-card>

    <el-tabs v-model="activeTab">
      <!-- ────────── Price books ────────── -->
      <el-tab-pane label="价格表" name="books">
        <el-row :gutter="16">
          <!-- book list -->
          <el-col :span="7">
            <el-card shadow="never">
              <template #header>
                <div class="flex items-center justify-between">
                  <span class="font-bold">价格表</span>
                  <el-button size="small" type="primary" :icon="Plus" @click="openCreateBook">新建</el-button>
                </div>
              </template>
              <div v-loading="booksLoading" class="book-list">
                <div
                  v-for="b in books"
                  :key="b.id"
                  class="book-item"
                  :class="{ active: b.id === selectedBookId }"
                  @click="selectBook(b.id)"
                >
                  <div class="font-bold text-slate-800">{{ b.name }}</div>
                  <div class="text-xs text-slate-400 truncate">{{ b.description || '—' }}</div>
                  <el-tag v-if="b.status !== 'active'" size="small" type="info" class="mt-1">已停用</el-tag>
                </div>
                <el-empty v-if="!books.length && !booksLoading" description="暂无价格表" :image-size="60" />
              </div>
            </el-card>
          </el-col>

          <!-- entries -->
          <el-col :span="17">
            <el-card shadow="never">
              <template #header>
                <div class="flex items-center justify-between flex-wrap gap-2">
                  <span class="font-bold">{{ selectedBook ? selectedBook.name + ' · 条目' : '请选择价格表' }}</span>
                  <div v-if="selectedBook" class="flex gap-2">
                    <el-button size="small" type="success" plain :icon="Refresh" :loading="syncing" @click="syncCommon">自动同步常用模型</el-button>
                    <el-button size="small" :icon="Edit" @click="openEditBook">编辑表</el-button>
                    <el-button size="small" :icon="Delete" type="danger" plain @click="removeBook">删除表</el-button>
                    <el-button size="small" type="primary" :icon="Plus" @click="openCreateEntry">新建条目</el-button>
                  </div>
                </div>
              </template>

              <el-table v-loading="entriesLoading" :data="entries" size="small" stripe>
                <el-table-column prop="model_code" label="模型" min-width="180" show-overflow-tooltip />
                <el-table-column prop="capability_type" label="能力" width="90" />
                <el-table-column label="输入 $/1M" width="100">
                  <template #default="{ row }">{{ row.input_per_1m_usd }}</template>
                </el-table-column>
                <el-table-column label="输出 $/1M" width="100">
                  <template #default="{ row }">{{ row.output_per_1m_usd }}</template>
                </el-table-column>
                <el-table-column label="缓存读 $/1M" width="110">
                  <template #default="{ row }">{{ row.cache_read_per_1m_usd || '—' }}</template>
                </el-table-column>
                <el-table-column label="来源" width="90">
                  <template #default="{ row }">
                    <el-tag size="small" :type="row.manually_edited ? 'success' : 'info'" effect="plain">
                      {{ row.manually_edited ? '手改' : (row.source === 'litellm' ? 'LiteLLM' : '手动') }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="120" fixed="right">
                  <template #default="{ row }">
                    <el-button link type="primary" size="small" @click="openEditEntry(row)">编辑</el-button>
                    <el-button link type="danger" size="small" @click="removeEntry(row)">删除</el-button>
                  </template>
                </el-table-column>
                <template #empty>
                  <span class="text-slate-400">{{ selectedBook ? '暂无条目，可手动新建或从 LiteLLM 导入' : '左侧选择一个价格表' }}</span>
                </template>
              </el-table>
            </el-card>
          </el-col>
        </el-row>
      </el-tab-pane>

      <!-- ────────── Tenant sell bindings ────────── -->
      <el-tab-pane label="租户售价绑定" name="bindings">
        <el-card shadow="never">
          <template #header>
            <div class="flex items-center justify-between">
              <span class="font-bold">平台 → 租户 售价绑定</span>
              <el-button size="small" type="primary" :icon="Plus" @click="openCreateBinding">新增绑定</el-button>
            </div>
          </template>
          <el-alert
            type="warning"
            :closable="false"
            show-icon
            class="mb-3"
            title="未配置售价绑定的租户，其请求会被拒绝（fail-closed）。"
          />
          <el-table v-loading="bindingsLoading" :data="bindings" size="small" stripe>
            <el-table-column prop="tenant_id" label="租户 ID" min-width="240" show-overflow-tooltip />
            <el-table-column prop="price_book_name" label="价格表" min-width="140" />
            <el-table-column label="售价倍率" width="100">
              <template #default="{ row }">×{{ row.sell_multiplier }}</template>
            </el-table-column>
            <el-table-column label="缓存计价" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.cache_billing_enabled ? 'success' : 'info'" effect="plain">
                  {{ row.cache_billing_enabled ? '开' : '关' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="更新时间" width="170">
              <template #default="{ row }">{{ formatTimestamp(row.updated_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openEditBinding(row)">编辑</el-button>
                <el-button link type="primary" size="small" @click="openUserBinding(row)">用户售价</el-button>
                <el-button link type="danger" size="small" @click="removeBinding(row)">删除</el-button>
              </template>
            </el-table-column>
            <template #empty><span class="text-slate-400">暂无绑定</span></template>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

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

    <!-- Entry dialog -->
    <el-dialog v-model="entryDialog" :title="entryEditing ? '编辑条目' : '新建条目'" width="640px">
      <el-form label-width="120px">
        <el-form-item v-if="!entryEditing" label="从 LiteLLM 填充">
          <el-select
            filterable
            remote
            clearable
            :remote-method="searchLiteLLM"
            :loading="llmSearching"
            placeholder="搜索模型名（如 claude / gpt-5），选中自动填价"
            style="width: 100%"
            @change="applyLiteLLM"
            @visible-change="(v) => v && !llmOptions.length && searchLiteLLM('')"
          >
            <el-option
              v-for="o in llmOptions"
              :key="o.model_code"
              :label="`${o.model_code}  ·  in $${o.input_per_1m_usd}/1M  out $${o.output_per_1m_usd}/1M`"
              :value="o.model_code"
            />
          </el-select>
          <span class="hint">参考价目，选中后可手动修改；只有保存才会入库。</span>
        </el-form-item>
        <el-form-item label="模型 model_code" required>
          <el-input v-model="entryForm.model_code" :disabled="entryEditing" placeholder="如 gpt-5.4 / claude-opus-4.5" />
        </el-form-item>
        <el-form-item label="能力类型">
          <el-select v-model="entryForm.capability_type" style="width: 100%">
            <el-option v-for="c in capabilityOptions" :key="c.value" :label="c.label" :value="c.value" />
          </el-select>
        </el-form-item>

        <template v-if="isTokenCap">
          <el-form-item label="输入 $/1M">
            <el-input-number v-model="entryForm.input_per_1m_usd" :min="0" :step="0.1" :precision="6" />
          </el-form-item>
          <el-form-item label="输出 $/1M">
            <el-input-number v-model="entryForm.output_per_1m_usd" :min="0" :step="0.1" :precision="6" />
          </el-form-item>
          <el-form-item label="缓存写 $/1M">
            <el-input-number v-model="entryForm.cache_write_per_1m_usd" :min="0" :step="0.1" :precision="6" />
            <span class="hint">0 = 按输入价</span>
          </el-form-item>
          <el-form-item label="缓存读 $/1M">
            <el-input-number v-model="entryForm.cache_read_per_1m_usd" :min="0" :step="0.1" :precision="6" />
            <span class="hint">0 = 按输入价</span>
          </el-form-item>
          <el-form-item label="推理 $/1M">
            <el-input-number v-model="entryForm.reasoning_per_1m_usd" :min="0" :step="0.1" :precision="6" />
            <span class="hint">0 = 按输出价</span>
          </el-form-item>
        </template>

        <template v-else-if="isImageCap">
          <el-form-item label="按分辨率(每张 $)">
            <div class="w-full space-y-2">
              <div v-for="(p, i) in entryForm.image_prices" :key="i" class="flex gap-2">
                <el-input v-model="p.resolution" placeholder="1024x1024" style="width: 160px" />
                <el-input-number v-model="p.price" :min="0" :step="0.01" :precision="6" />
                <el-button :icon="Delete" circle size="small" @click="removeResolution(entryForm.image_prices, i)" />
              </div>
              <el-button size="small" :icon="Plus" @click="addResolution(entryForm.image_prices)">添加分辨率</el-button>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="isVideoCap">
          <el-form-item label="按分辨率(每秒 $)">
            <div class="w-full space-y-2">
              <div v-for="(p, i) in entryForm.video_prices" :key="i" class="flex gap-2">
                <el-input v-model="p.resolution" placeholder="720p" style="width: 160px" />
                <el-input-number v-model="p.price" :min="0" :step="0.01" :precision="6" />
                <el-button :icon="Delete" circle size="small" @click="removeResolution(entryForm.video_prices, i)" />
              </div>
              <el-button size="small" :icon="Plus" @click="addResolution(entryForm.video_prices)">添加分辨率</el-button>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="isAudioCap">
          <el-form-item label="TTS $/1M字符">
            <el-input-number v-model="entryForm.audio_tts_per_1m_chars_usd" :min="0" :step="0.1" :precision="6" />
          </el-form-item>
          <el-form-item label="STT $/分钟">
            <el-input-number v-model="entryForm.audio_stt_per_minute_usd" :min="0" :step="0.001" :precision="6" />
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="entryDialog = false">取消</el-button>
        <el-button type="primary" @click="submitEntry">保存</el-button>
      </template>
    </el-dialog>

    <!-- Tenant sell binding dialog -->
    <el-dialog v-model="bindingDialog" :title="bindingEditing ? '编辑租户售价' : '新增租户售价'" width="480px">
      <el-form label-width="90px">
        <el-form-item label="租户 ID" required>
          <el-input v-model="bindingForm.tenant_id" :disabled="bindingEditing" placeholder="租户唯一 ID" />
        </el-form-item>
        <el-form-item label="价格表" required>
          <el-select v-model="bindingForm.price_book_id" style="width: 100%">
            <el-option v-for="b in books" :key="b.id" :label="b.name" :value="b.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="售价倍率">
          <el-input-number v-model="bindingForm.sell_multiplier" :min="0" :step="0.1" :precision="4" />
          <span class="hint">1=按价格表，1.5=加价 50%</span>
        </el-form-item>
        <el-form-item label="缓存计价">
          <el-switch v-model="bindingForm.cache_billing_enabled" />
          <span class="hint">关=缓存按输入价计</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindingDialog = false">取消</el-button>
        <el-button type="primary" @click="submitBinding">保存</el-button>
      </template>
    </el-dialog>

    <!-- User sell binding dialog -->
    <el-dialog v-model="userBindingDialog" title="租户 → 用户 售价（级联）" width="480px">
      <p class="text-xs text-slate-400 mb-3">
        用户售价 = 平台给该租户的售价 × 用户倍率。租户跟随平台为其选定的价格表。
      </p>
      <el-form label-width="90px">
        <el-form-item label="租户 ID">
          <el-input v-model="userBindingForm.tenant_id" disabled />
        </el-form-item>
        <el-form-item label="用户倍率">
          <el-input-number v-model="userBindingForm.user_multiplier" :min="0" :step="0.1" :precision="4" />
        </el-form-item>
        <el-form-item label="缓存计价">
          <el-switch v-model="userBindingForm.cache_billing_enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button v-if="userBindingForm.exists" type="danger" plain @click="removeUserBinding">删除</el-button>
        <el-button @click="userBindingDialog = false">取消</el-button>
        <el-button type="primary" @click="submitUserBinding">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.rate-card :deep(.el-card__body) { padding: 16px 20px; }
.book-list { max-height: 520px; overflow-y: auto; }
.book-item {
  padding: 10px 12px;
  border-radius: 10px;
  cursor: pointer;
  border: 1px solid transparent;
  margin-bottom: 6px;
}
.book-item:hover { background: #f8fafc; }
.book-item.active { background: #eff6ff; border-color: #bfdbfe; }
.hint { color: #94a3b8; font-size: 12px; margin-left: 8px; }
</style>
