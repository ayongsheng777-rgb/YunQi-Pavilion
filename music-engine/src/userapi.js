// 云栖阁音乐引擎：音源脚本沙盒，协议移植自 lx-music-desktop 的 userApi（src/main/modules/userApi）。
// 用 Node vm 加载用户的自定义音源脚本，提供同样的 lx 桥（EVENT_NAMES/on/send/request/utils），
// 脚本不需要任何修改即可在本引擎运行。
import vm from 'node:vm'
import needle from 'needle'
import zlib from 'node:zlib'
import fs from 'node:fs'
import path from 'node:path'
import { createCipheriv, createHash, publicEncrypt, constants, randomBytes } from 'node:crypto'
import { proxyUrl } from './proxy.js'

export const EVENT_NAMES = { request: 'request', inited: 'inited', updateAlert: 'updateAlert' }
const allSources = ['kw', 'kg', 'tx', 'wy', 'mg', 'local']
export const supportQualitys = {
  kw: ['128k', '320k', 'flac', 'flac24bit'],
  kg: ['128k', '320k', 'flac', 'flac24bit'],
  tx: ['128k', '320k', 'flac', 'flac24bit'],
  wy: ['128k', '320k', 'flac', 'flac24bit'],
  mg: ['128k', '320k', 'flac', 'flac24bit'],
  local: [],
}
const supportActions = {
  kw: ['musicUrl'], kg: ['musicUrl'], tx: ['musicUrl'], wy: ['musicUrl'], mg: ['musicUrl'],
  xm: ['musicUrl'], local: ['musicUrl', 'lyric', 'pic'],
}

const parseScriptHeader = (script) => {
  const meta = { name: '', description: '', version: '', author: '', homepage: '' }
  const head = String(script).slice(0, 2048)
  for (const key of Object.keys(meta)) {
    const m = head.match(new RegExp(`@` + key + `\\s+(.+)`))
    if (m) meta[key] = m[1].trim()
  }
  return meta
}

const makeRequest = () => (url, opts = {}, callback) => {
  const { method = 'get', timeout, headers, body, form, formData } = opts || {}
  const options = { headers }
  let data
  if (body) data = body
  else if (form) { data = form; options.json = false }
  else if (formData) { data = formData; options.json = false }
  options.response_timeout = typeof timeout === 'number' && timeout > 0 ? Math.min(timeout, 60_000) : 60_000
  const p = proxyUrl()
  if (p) options.proxy = p
  let req
  try {
    req = needle.request(method, url, data, options, (err, resp, body) => {
      if (err) return callback.call(null, err, null, null)
      try {
        let b = resp.raw ? resp.raw.toString() : body
        const parsedBody = (() => { try { return JSON.parse(b) } catch { return b } })()
        callback.call(null, null, {
          statusCode: resp.statusCode,
          statusMessage: resp.statusMessage,
          headers: resp.headers,
          bytes: resp.bytes,
          raw: resp.raw,
          body: parsedBody,
        }, parsedBody)
      } catch (e) {
        callback.call(null, e, null, null)
      }
    }).request
  } catch (e) {
    callback.call(null, e, null, null)
    return () => {}
  }
  return () => { try { if (req && !req.aborted) req.abort() } catch (_) {} req = null }
}

const verifyLyricInfo = (info) => {
  if (typeof info !== 'object' || info === null || typeof info.lyric !== 'string') throw new Error('failed')
  if (info.lyric.length > 51200) throw new Error('failed')
  return {
    lyric: info.lyric,
    tlyric: (typeof info.tlyric === 'string' && info.tlyric.length < 5120) ? info.tlyric : null,
    rlyric: (typeof info.rlyric === 'string' && info.rlyric.length < 5120) ? info.rlyric : null,
    lxlyric: (typeof info.lxlyric === 'string' && info.lxlyric.length < 8192) ? info.lxlyric : null,
  }
}

export class UserApiInstance {
  constructor(script, id) {
    this.id = id || 'default'
    this.script = script
    this.meta = parseScriptHeader(script)
    this.sources = {}
    this.inited = false
    this._requestHandler = null
    this._updateAlertShown = false
  }

  load() {
    return new Promise((resolve, reject) => {
      const self = this
      const timeout = setTimeout(() => reject(new Error('音源脚本初始化超时')), 15000)
      const done = (err) => { clearTimeout(timeout); err ? reject(err) : resolve() }

      const lx = {
        EVENT_NAMES,
        request: makeRequest(),
        send(eventName, data) {
          return new Promise((res, rej) => {
            if (!Object.values(EVENT_NAMES).includes(eventName)) return rej(new Error('The event is not supported: ' + eventName))
            switch (eventName) {
              case EVENT_NAMES.inited: {
                if (self.inited) return rej(new Error('Script is inited'))
                self.inited = true
                try {
                  const sourceInfo = {}
                  for (const source of allSources) {
                    const userSource = data && data.sources ? data.sources[source] : null
                    if (!userSource || userSource.type !== 'music') continue
                    const qualitys = supportQualitys[source] || []
                    const actions = supportActions[source] || []
                    sourceInfo[source] = {
                      type: 'music',
                      actions: actions.filter((a) => (userSource.actions || []).includes(a)),
                      qualitys: qualitys.filter((q) => (userSource.qualitys || []).includes(q)),
                    }
                  }
                  self.sources = sourceInfo
                  res()
                  done(null)
                } catch (e) { rej(e); done(e) }
                break
              }
              case EVENT_NAMES.updateAlert: {
                if (self._updateAlertShown) return rej(new Error('The update alert can only be called once.'))
                self._updateAlertShown = true
                res()
                break
              }
              default:
                rej(new Error('Unknown event name: ' + eventName))
            }
          })
        },
        on(eventName, handler) {
          if (!Object.values(EVENT_NAMES).includes(eventName)) return Promise.reject(new Error('The event is not supported: ' + eventName))
          if (eventName !== EVENT_NAMES.request) return Promise.reject(new Error('The event is not supported: ' + eventName))
          self._requestHandler = handler
          return Promise.resolve()
        },
        utils: {
          crypto: {
            aesEncrypt(buffer, mode, key, iv) {
              const cipher = createCipheriv(mode, key, iv)
              return Buffer.concat([cipher.update(buffer), cipher.final()])
            },
            rsaEncrypt(buffer, key) {
              buffer = Buffer.concat([Buffer.alloc(128 - buffer.length), buffer])
              return publicEncrypt({ key, padding: constants.RSA_NO_PADDING }, buffer)
            },
            randomBytes(size) { return randomBytes(size) },
            md5(str) { return createHash('md5').update(str).digest('hex') },
          },
          buffer: {
            from(...args) { return Buffer.from(...args) },
            bufToString(buf, format) { return Buffer.from(buf, 'binary').toString(format) },
          },
          zlib: {
            inflate(buf) {
              return new Promise((res, rej) => zlib.inflate(buf, (err, data) => err ? rej(new Error(err.message)) : res(data)))
            },
            deflate(data) {
              return new Promise((res, rej) => zlib.deflate(data, (err, buf) => err ? rej(new Error(err.message)) : res(buf)))
            },
          },
        },
        currentScriptInfo: {
          name: self.meta.name, description: self.meta.description, version: self.meta.version,
          author: self.meta.author, homepage: self.meta.homepage, rawScript: self.script,
        },
        version: '2.0.0',
        env: 'desktop',
      }

      const sandbox = {
        console, setTimeout, clearTimeout, setInterval, clearInterval,
        Buffer, Promise, JSON, Math, Date, RegExp, Error, Map, Set, WeakMap, WeakSet,
        Array, Object, String, Number, Boolean, Symbol, Reflect, Proxy,
        encodeURIComponent, decodeURIComponent, encodeURI, decodeURI,
        URL, URLSearchParams, TextEncoder, TextDecoder, queueMicrotask,
      }
      sandbox.globalThis = sandbox
      sandbox.lx = lx
      vm.createContext(sandbox)
      try {
        const code = new vm.Script(self.script, { filename: `userapi-${self.id}.js` })
        code.runInContext(sandbox, { timeout: 10000 })
      } catch (e) {
        done(e)
      }
    })
  }

  async request(source, action, info) {
    if (!this._requestHandler) throw new Error('音源脚本未就绪（没有注册 request 事件）')
    const response = await this._requestHandler.call(null, { source, action, info })
    switch (action) {
      case 'musicUrl':
        if (typeof response !== 'string' || response.length > 2048 || !/^https?:/.test(response)) throw new Error('音源返回的播放地址不合法')
        return response
      case 'lyric':
        return verifyLyricInfo(response)
      case 'pic':
        if (typeof response !== 'string' || response.length > 2048 || !/^https?:/.test(response)) throw new Error('failed')
        return response
      default:
        return response
    }
  }

  getMusicUrl(source, musicInfo, quality) {
    return this.request(source, 'musicUrl', { musicInfo, type: quality })
  }
  getLyric(source, musicInfo) {
    return this.request(source, 'lyric', { musicInfo })
  }
  getPic(source, musicInfo) {
    return this.request(source, 'pic', { musicInfo })
  }
}

// 地址验真：有些音源脚本会返回已失效的播放地址（404 等），拿到后先探活，
// 活不了就换下一个脚本试。403/405/501 属"探不准"，放行（宁可给用户一个可能能播的地址）。
const validatePlayUrl = (url) =>
  new Promise((resolve) => {
    if (!url || typeof url !== 'string') return resolve(false)
    let done = false
    let stream = null
    const finish = (ok) => {
      if (done) return
      done = true
      clearTimeout(timer)
      try { stream?.destroy() } catch {}
      resolve(ok)
    }
    const timer = setTimeout(() => finish(false), 15000)
    try {
      stream = needle.get(url, {
        proxy: proxyUrl(),
        open_timeout: 8000,
        response_timeout: 12000,
        follow_max: 5,
        headers: {
          Range: 'bytes=0-1023',
          'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36',
        },
      })
    } catch {
      return finish(false)
    }
    stream.on('header', (status) => {
      finish(status === 403 || status === 405 || status === 501 || (status >= 200 && status < 400))
    })
    stream.on('error', () => finish(false))
  })

// ---- 音源脚本管理：存 data/sources 目录 ----
export class SourceManager {
  constructor(dir) {
    this.dir = dir
    this.instances = [] // 数组，第一项为当前生效源
    this.meta = []
  }

  listDir() {
    try { return fs.readdirSync(this.dir).filter((f) => f.endsWith('.js')) } catch { return [] }
  }

  async loadAll() {
    this.instances = []
    this.meta = []
    for (const file of this.listDir()) {
      const script = fs.readFileSync(path.join(this.dir, file), 'utf8')
      const inst = new UserApiInstance(script, file)
      try {
        await inst.load()
        this.instances.push(inst)
        this.meta.push({ id: file, ...inst.meta, sources: inst.sources, ok: true })
      } catch (e) {
        this.meta.push({ id: file, ...inst.meta, sources: {}, ok: false, error: e.message })
      }
    }
    return this.meta
  }

  async addFromText(id, script) {
    const file = id.endsWith('.js') ? id : `${id}.js`
    fs.mkdirSync(this.dir, { recursive: true })
    fs.writeFileSync(path.join(this.dir, file), script, 'utf8')
    await this.loadAll()
    return this.meta
  }

  async remove(id) {
    const file = id.endsWith('.js') ? id : `${id}.js`
    try { fs.unlinkSync(path.join(this.dir, file)) } catch {}
    await this.loadAll()
    return this.meta
  }

  current() { return this.instances[0] || null }

  // 多音源自动切换：按顺序尝试所有已加载脚本，地址还要验真，能打开才算数
  async getMusicUrl(source, musicInfo, quality) {
    if (!this.instances.length) throw new Error('还没有导入音源脚本，无法获取播放地址')
    let lastErr = null
    for (const inst of this.instances) {
      const s = inst.sources[source]
      if (!s || !s.actions.includes('musicUrl')) continue
      try {
        const r = await inst.getMusicUrl(source, musicInfo, quality)
        const url = typeof r === 'string' ? r : r?.url
        if (url && (await validatePlayUrl(url))) return r
        lastErr = new Error(`音源「${inst.meta?.name || '未知'}」给的播放地址打不开`)
      } catch (e) {
        lastErr = e
      }
    }
    throw lastErr || new Error(`没有音源脚本支持 ${source} 的播放地址`)
  }
}
