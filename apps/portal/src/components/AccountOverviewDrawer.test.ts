import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountOverviewDrawer from './AccountOverviewDrawer.vue'
import { platformAdminApi } from '@/api/platformAdmin'

vi.mock('@/api/platformAdmin', () => ({
  platformAdminApi: {
    getAccountBalance: vi.fn()
  }
}))

const balanceResponse = {
  totalCredits: 200,
  usedCredits: 80,
  remainingCredits: 120,
  frozenCredits: 20,
  availableCredits: 100,
  permanentCredits: 70,
  timedCredits: 50,
  outstandingDebtMicro: 250_000,
  serviceState: 'blocked_debt' as const,
  packages: []
}

const passthroughStub = { template: '<div><slot /><slot name="action" /></div>' }

describe('AccountOverviewDrawer', () => {
  beforeEach(() => {
    vi.mocked(platformAdminApi.getAccountBalance).mockResolvedValue(balanceResponse)
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
    expect(wrapper.text()).toContain('75积分')
    expect(wrapper.text()).toContain('未结透支')
    expect(wrapper.text()).toContain('-25 积分')
  })
})
