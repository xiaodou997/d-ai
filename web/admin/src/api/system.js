import request from '@/utils/request'

export function listSystemAdmins(params) {
  return request({ url: '/urm/v1/admin/system/admins', method: 'get', params })
}

export function createSystemAdmin(data) {
  return request({ url: '/urm/v1/admin/system/admins', method: 'post', data })
}

export function updateSystemAdmin(id, data) {
  return request({ url: `/urm/v1/admin/system/admins/${id}`, method: 'put', data })
}

export function deleteSystemAdmin(id) {
  return request({ url: `/urm/v1/admin/system/admins/${id}`, method: 'delete' })
}
