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
          DsDrawer: passthroughStub,
          DsEmpty: passthroughStub,
          DsTable: passthroughStub,
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
})
