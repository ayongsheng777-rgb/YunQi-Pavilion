// 云栖阁音乐引擎（yunqi-music-engine）
// 把 lx-music-desktop 的搜歌/歌词/榜单/歌单能力（musicSdk）+ 音源脚本沙盒（userApi）
// 做成 HTTP 服务，供云栖阁后端调用。协议设计给云栖阁的音乐播放器用。
// 引擎自己管理代理：清掉宿主环境变量里的代理，避免 Node 24 自动读取导致请求被带偏/进程崩溃
for (const k of ['HTTP_PROXY', 'HTTPS_PROXY', 'http_proxy', 'https_proxy', 'ALL_PROXY', 'all_proxy']) delete process.env[k]
process.env.NODE_USE_ENV_PROXY = '0'
// 兜底：任何未捕获异常只记日志，不让引擎进程退出
process.on('uncaughtException', (e) => console.error('[engine] uncaught:', e && e.message))
process.on('unhandledRejection', (e) => console.error('[engine] unhandledRejection:', e && e.message))
import './src/shim.js'
import http from 'node:http'
import { URL } from 'node:url'
import needle from 'needle'
import { proxyUrl } from './src/proxy.js'
import { sourceManager } from './src/server-bridge.js'
const fetchTextViaEngineOnce = (url) => new Promise((resolve, reject) => {
  const opts = { headers: { 'User-Agent': 'yunqi-music-engine/1.0' }, follow_max: 5, response_timeout: 30000 }
  const pu = proxyUrl()
  if (pu) opts.proxy = pu
  needle.request('get', url, null, opts, (err, resp, body) => {
    if (err) return reject(err)
    if (!resp || resp.statusCode >= 400) return reject(new Error('HTTP ' + (resp && resp.statusCode)))
    resolve(resp.raw ? resp.raw.toString() : String(body ?? ''))
  })
})
// 网络抖动重试 3 次
const fetchTextViaEngine = async (url) => {
  let lastErr
  for (let i = 0; i < 3; i++) {
    try { return await fetchTextViaEngineOnce(url) } catch (e) {
      lastErr = e
      await new Promise((r) => setTimeout(r, 800 * (i + 1)))
    }
  }
  throw lastErr
}
import { setProxy, getProxy } from './src/proxy.js'
import { apis } from './src/musicSdk/api-source.js'
import kw from './src/musicSdk/kw/index.js'
import kg from './src/musicSdk/kg/index.js'
import tx from './src/musicSdk/tx/index.js'
import wy from './src/musicSdk/wy/index.js'
import mg from './src/musicSdk/mg/index.js'

const PORT = Number(process.env.PORT || 23400)
const sources = { kw, kg, tx, wy, mg }
const sourceNames = { kw: '酷我音乐', kg: '酷狗音乐', tx: 'QQ音乐', wy: '网易音乐', mg: '咪咕音乐' }

// 代理（走 NAS 上的 sing-box，音乐平台一般都需要；也可在云栖阁设置页运行时修改）
if (process.env.PROXY_HOST) {
  setProxy({ host: process.env.PROXY_HOST, port: Number(process.env.PROXY_PORT || 7890) })
}
const proxyState = () => {
  const p = getProxy()
  return p ? `${p.host}:${p.port}` : '未启用'
}
console.log('[engine] proxy:', proxyState())

const readBody = (req) => new Promise((resolve, reject) => {
  let data = ''
  req.on('data', (c) => { data += c; if (data.length > 5 * 1024 * 1024) reject(new Error('body 太大')) })
  req.on('end', () => resolve(data))
  req.on('error', reject)
})

const send = (res, code, obj) => {
  res.writeHead(code, { 'Content-Type': 'application/json; charset=utf-8', 'Access-Control-Allow-Origin': '*' })
  res.end(JSON.stringify(obj))
}
const ok = (res, data) => send(res, 200, { code: 0, data })
const fail = (res, msg, code = 500) => send(res, 200, { code, msg: String(msg) })

const routes = {
  // 健康检查
  'GET /health': async () => ({ ok: true, version: '1.0.0', sources: Object.keys(sources), proxy: proxyState(), loadedScripts: sourceManager.meta.length }),

  // 运行时配置：代理
  'GET /api/config': async () => ({ proxy: getProxy(), engine: 'yunqi-music-engine/1.0.0' }),
  'POST /api/config': async (q, body) => {
    if (body.proxy !== undefined) {
      setProxy(body.proxy && body.proxy.host ? { host: body.proxy.host, port: body.proxy.port } : null)
      console.log('[engine] proxy changed to:', proxyState())
    }
    return { proxy: getProxy() }
  },

  // 音源脚本管理
  'GET /api/sources': async () => sourceManager.meta,
  'POST /api/sources/import': async (q, body) => {
    if (body.url) {
      const code = await fetchTextViaEngine(body.url)
      if (!code || code.length < 100) throw new Error('下载到的内容不像音源脚本')
      return sourceManager.addFromText(body.id || `source-${Date.now()}`, code)
    }
    if (body.code) return sourceManager.addFromText(body.id || `source-${Date.now()}`, body.code)
    throw new Error('需要 url 或 code')
  },
  'DELETE /api/sources/:id': async (q) => sourceManager.remove(q.id),

  // 搜索
  'GET /api/search': async (q) => {
    const source = String(q.source || 'kw')
    const mod = sources[source]
    if (!mod) throw new Error('不支持的音源：' + source)
    const r = await mod.musicSearch.search(String(q.q || ''), Number(q.page || 1), Number(q.limit || 30))
    const list = Array.isArray(r) ? r : (r && Array.isArray(r.list) ? r.list : [])
    return { source, sourceName: sourceNames[source], list, total: r && r.total, allPage: r && r.allPage }
  },

  // 播放地址（走音源脚本沙盒）
  'POST /api/music-url': async (q, body) => {
    const source = String(body.source || 'kw')
    const musicInfo = body.musicInfo
    const quality = String(body.quality || '320k')
    if (!musicInfo) throw new Error('缺少 musicInfo')
    const { url, type } = await apis(source).getMusicUrl(musicInfo, quality).promise
    return { source, quality: type, url }
  },

  // 歌词（SDK 直取；失败则尝试音源脚本的 lyric）
  'GET /api/lyric': async (q) => {
    const source = String(q.source || 'kw')
    const musicInfo = JSON.parse(String(q.musicInfo || '{}'))
    try {
      const r = await sources[source].getLyric(musicInfo)
      // SDK 的 getLyric 返回 httpFetch 句柄，要等它的 promise 才是歌词数据
      return r && r.promise ? await r.promise : r
    } catch (e) {
      const inst = sourceManager.current()
      if (inst && inst.sources[source] && inst.sources[source].actions.includes('lyric')) {
        return inst.getLyric(source, musicInfo)
      }
      throw e
    }
  },

  // 封面
  'GET /api/pic': async (q) => {
    const source = String(q.source || 'kw')
    const musicInfo = JSON.parse(String(q.musicInfo || '{}'))
    try {
      const r = await sources[source].getPic(musicInfo)
      return r && r.promise ? await r.promise : r
    } catch (e) {
      const inst = sourceManager.current()
      if (inst && inst.sources[source] && inst.sources[source].actions.includes('pic')) {
        return inst.getPic(source, musicInfo)
      }
      throw e
    }
  },

  // 榜单
  'GET /api/leaderboard': async (q) => {
    const source = String(q.source || 'kw')
    const list = await sources[source].leaderboard.getList()
    return { source, list }
  },
  'GET /api/leaderboard/songs': async (q) => {
    const source = String(q.source || 'kw')
    const list = await sources[source].leaderboard.getDetail(String(q.id || ''))
    return { source, list }
  },

  // 歌单
  'GET /api/songlist/hot-tags': async (q) => {
    const source = String(q.source || 'kw')
    return sources[source].songList.getHotTag()
  },
  'GET /api/songlist/list': async (q) => {
    const source = String(q.source || 'kw')
    return sources[source].songList.getList(String(q.tag || ''), Number(q.page || 1))
  },
  'GET /api/songlist/songs': async (q) => {
    const source = String(q.source || 'kw')
    return sources[source].songList.getDetail(String(q.id || ''), Number(q.page || 1))
  },

  // 热搜
  'GET /api/hot-search': async (q) => {
    const source = String(q.source || 'kw')
    const mod = sources[source]
    if (mod.hotSearch && mod.hotSearch.getList) return mod.hotSearch.getList()
    return []
  },
}

const server = http.createServer(async (req, res) => {
  try {
    const u = new URL(req.url, 'http://localhost')
    const path = u.pathname
    const query = Object.fromEntries(u.searchParams.entries())
    let key = `${req.method} ${path}`
    let params = query
    // 支持 :id 参数
    let handler = routes[key]
    if (!handler) {
      for (const rk of Object.keys(routes)) {
        const [m, p] = rk.split(' ')
        if (m !== req.method) continue
        const rp = p.split('/')
        const pp = path.split('/')
        if (rp.length !== pp.length) continue
        const matched = rp.every((seg, i) => seg.startsWith(':') || seg === pp[i])
        if (matched) {
          handler = routes[rk]
          params = { ...query }
          rp.forEach((seg, i) => { if (seg.startsWith(':')) params[seg.slice(1)] = decodeURIComponent(pp[i]) })
          break
        }
      }
    }
    if (!handler) return fail(res, '接口不存在', 404)
    let body = {}
    if (req.method === 'POST' || req.method === 'PUT') {
      const text = await readBody(req)
      if (text) { try { body = JSON.parse(text) } catch { throw new Error('请求体不是合法 JSON') } }
    }
    const data = await handler(params, body, req)
    ok(res, data)
  } catch (e) {
    console.error('[engine]', e.message)
    fail(res, e.message || '内部错误')
  }
})

await sourceManager.loadAll()
console.log('[engine] 已加载音源脚本：', sourceManager.meta.map((m) => `${m.id}(${m.ok ? 'ok' : '失败'})`).join(', ') || '无')
// 首次启动且没有音源时，按 DEFAULT_SOURCES 环境变量自动导入（格式：name=url;name=url）
if (!sourceManager.meta.length && process.env.DEFAULT_SOURCES) {
  const list = String(process.env.DEFAULT_SOURCES).split(';').map((x) => x.trim()).filter(Boolean)
  for (const item of list) {
    const i = item.indexOf('=')
    if (i < 0) continue
    const name = item.slice(0, i).trim()
    const url = item.slice(i + 1).trim()
    try {
      const code = await fetchTextViaEngine(url)
      if (!code || code.length < 100) throw new Error('内容不像音源脚本')
      await sourceManager.addFromText(name, code)
      console.log('[engine] 自动导入音源：', name, 'ok')
    } catch (e) {
      console.log('[engine] 自动导入音源失败：', name, e.message)
    }
  }
}
server.listen(PORT, '0.0.0.0', () => {
  console.log(`[engine] listening on :${PORT}`)
})
