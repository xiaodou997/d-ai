import request from '@/utils/request'

const BASE_PATH = '/urm/v1/governance'

export function listClients() {
  return request({
    url: `${BASE_PATH}/clients`,
    method: 'get'
  })
}
