// 云栖阁音乐引擎：移植自 lx-music-desktop 的公共工具函数（src/renderer/utils/index.ts 及 @common/utils）。
import crypto from 'node:crypto'

const entities = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ' }
export const decodeName = (str = '') => {
  if (!str) return ''
  return String(str)
    .replace(/&#(\d+);/g, (_, n) => String.fromCodePoint(Number(n)))
    .replace(/&#x([0-9a-fA-F]+);/g, (_, n) => String.fromCodePoint(parseInt(n, 16)))
    .replace(/&(\w+);/g, (m, name) => entities[name] ?? m)
}

export const formatPlayTime = (time) => {
  if (time == null || isNaN(time)) return '00:00'
  const t = Math.floor(Number(time))
  const m = Math.floor(t / 60)
  const s = t % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

export const sizeFormate = (size) => {
  if (size == null || isNaN(size)) return ''
  const num = Number(size)
  if (num < 1024) return `${num}B`
  if (num < 1024 * 1024) return `${(num / 1024).toFixed(2)}KB`
  if (num < 1024 * 1024 * 1024) return `${(num / 1024 / 1024).toFixed(2)}MB`
  return `${(num / 1024 / 1024 / 1024).toFixed(2)}GB`
}

export const formatPlayCount = (num) => {
  if (num > 100000000) return `${Math.trunc(num / 10000000) / 10}亿`
  if (num > 10000) return `${Math.trunc(num / 1000) / 10}万`
  return String(num)
}

const pad2 = (n) => String(n).padStart(2, '0')
export const dateFormat = (time, format = 'YYYY-MM-DD') => {
  const d = new Date(time)
  return format
    .replace('YYYY', String(d.getFullYear()))
    .replace('MM', pad2(d.getMonth() + 1))
    .replace('DD', pad2(d.getDate()))
    .replace('hh', pad2(d.getHours()))
    .replace('mm', pad2(d.getMinutes()))
    .replace('ss', pad2(d.getSeconds()))
}

export const dateFormat2 = (time) => {
  const differ = Math.trunc((Date.now() - time) / 1000)
  if (differ < 60) return `${differ}秒前`
  if (differ < 3600) return `${Math.trunc(differ / 60)}分钟前`
  if (differ < 86400) return `${Math.trunc(differ / 3600)}小时前`
  return dateFormat(time)
}

export const toMD5 = (str) => crypto.createHash('md5').update(String(str)).digest('hex')

export const deduplicationList = (list) => {
  const ids = new Set()
  return list.filter((s) => {
    if (ids.has(s.id)) return false
    ids.add(s.id)
    return true
  })
}
