import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountOverviewDrawer from './AccountOverviewDrawer.vue'
import { platformAdminApi } from '@/api/platformAdmin'

vi.mock('@/api/platformAdmin', () => ({
  platformAdminApi: {
    getAccountBalance: vi.fn(),
    listBalanceLedger: vi.fn()
  }
}))

const balanceResponse = {
  currency: 'USD',
  totalUsd: 200,
  usedUsd: 80,
  remainingUsd: 120,
  availableUsd: 120,
  permanentUsd: 70,
  timedUsd: 50,
  outstandingDebtMicroUsd: 25_000_000,
  serviceState: 'blocked_debt' as const,
  balanceLots: []
}

const passthroughStub = { template: '<div><slot /><slot name="action" /></div>' }
const drawerStub = {
  props: ['width'],
  template: '<div class="drawer-stub" :data-width="width"><slot /></div>'
}
const tableStub = {
  props: ['columns', 'rows'],
  template: `
    <div class="table-stub">
      <span v-for="column in columns" :key="column.key" class="table-column">{{ column.title }}</span>
      <template v-for="row in rows" :key="row.balanceLotId || row.txnId">
        <slot v-for="column in columns" :key="column.key" :name="'cell-' + column.key" :row="row">
          {{ row[column.key] }}
        </slot>
      </template>
    </div>
  `
}

describe('AccountOverviewDrawer', () => {
  beforeEach(() => {
    vi.mocked(platformAdminApi.getAccountBalance).mockResolvedValue(balanceResponse)
    vi.mocked(platformAdminApi.listBalanceLedger).mockResolvedValue({ items: [], total: 0, page: 1, size: 20 })
  })

  it('loads the selected account and includes debt in the net balance', async () => {
    const wrapper = mount(AccountOverviewDrawer, {
      props: {
        open: true,
        accountType: 2,
        accountId: 'USER_1',
        accountName: '测试用户'
      },
      global: {
        directives: { loading: {} },
        stubs: {
          DsDrawer: drawerStub,
          DsEmpty: passthroughStub,
          DsTable: tableStub,
          DsTag: passthroughStub,
          ElButton: passthroughStub
        }
      }
    })

    await flushPromises()

    expect(platformAdminApi.getAccountBalance).toHaveBeenCalledWith({
      accountType: 2,
      accountId: 'USER_1',
      detail: true
    })
    expect(wrapper.text()).toContain('$95.00')
    expect(wrapper.text()).toContain('未结透支')
    expect(wrapper.text()).toContain('-$25.00')
  })

  it('shows when each active balance lot was created in a wider drawer', async () => {
    vi.mocked(platformAdminApi.getAccountBalance).mockResolvedValue({
      ...balanceResponse,
      balanceLots: [{
        balanceLotId: 'LOT_1',
        totalUsd: 50,
        remainingUsd: 40,
        createdAt: '2026-08-19T04:30:00Z',
        expiresAt: null,
        source: 'ADMIN_RECHARGE'
      }]
    })

    const wrapper = mount(AccountOverviewDrawer, {
      props: {
        open: true,
        accountType: 1,
        accountId: 'TENANT_1',
        accountName: '测试租户'
      },
      global: {
        directives: { loading: {} },
        stubs: {
          DsDrawer: drawerStub,
          DsEmpty: passthroughStub,
          DsTable: tableStub,
          DsTag: passthroughStub,
          ElButton: passthroughStub
        }
      }
    })

    await flushPromises()

    expect(wrapper.get('.drawer-stub').attributes('data-width')).toBe('1040px')
    expect(wrapper.text()).toContain('创建时间')
    expect(wrapper.text()).toContain(new Date('2026-08-19T04:30:00Z').toLocaleString('zh-CN', { hour12: false }))
  })
})
