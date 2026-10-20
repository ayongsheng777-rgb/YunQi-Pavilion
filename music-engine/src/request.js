// 云栖阁音乐引擎：网络请求层，移植自 lx-music-desktop src/renderer/utils/request.js
// 用 needle 实现，接口契约与原版一致：httpFetch(url, options) -> { promise, cancelHttp }
import needle from 'needle'
import { requestMsg } from './message.js'
import { debugRequest } from './env.js'
import { getProxy, proxyUrl, setProxy as setProxyCfg } from './proxy.js'
import { timeout as defaultTimeout, headers as sdkHeaders } from './musicSdk/options.js'

const defaultHeaders = {
  'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/69.0.3497.100 Safari/537.36',
}

// 代理配置（由 server.js 启动或运行时设置），SDK 与沙盒共用
export const setProxy = setProxyCfg

const doRequest = (url, options, callback) => {
  let data
  const opts = { ...options }
  if (opts.body) {
    data = opts.body
  } else if (opts.form) {
    data = opts.form
    opts.json = false
  } else if (opts.formData) {
    data = opts.formData
    opts.json = false
  }
  opts.response_timeout = opts.timeout || defaultTimeout
  opts.headers = { ...defaultHeaders, ...sdkHeaders, ...(opts.headers || {}) }
  const p = proxyUrl()
  if (p) opts.proxy = p
  return needle.request(opts.method || 'get', url, data, opts, (err, resp, body) => {
    if (!err && resp) {
      try {
        const raw = resp.raw ? resp.raw.toString() : (typeof body === 'string' ? body : JSON.stringify(body))
        resp.body = raw
        try { resp.body = JSON.parse(raw) } catch (_) {}
        body = resp.body
      } catch (_) {}
    }
    callback(err, resp, body)
  })
}

const buildHttpPromise = (url, options = {}) => {
  const obj = {
    isCancelled: false,
    cancelHttp: () => {
      if (!obj.requestObj) { obj.isCancelled = true; return }
      try { obj.requestObj.abort() } catch (_) {}
      obj.requestObj = null
      const fn = obj.cancelFn
      obj.cancelFn = null
      if (fn) fn(new Error(requestMsg.cancelRequest))
    },
  }
  obj.promise = new Promise((resolve, reject) => {
    obj.cancelFn = reject
    debugRequest && console.log(`\n---send request------${url}------------`)
    const req = doRequest(url, options.method ? options : { ...options, method: 'get' }, (err, resp, body) => {
      debugRequest && console.log(`\n---response------${url}------------`)
      obj.requestObj = null
      obj.cancelFn = null
      if (err) return reject(err)
      resolve(resp)
    })
    obj.requestObj = req
    if (obj.isCancelled) obj.cancelHttp()
  })
  return obj
}

export const httpFetch = (url, options = { method: 'get' }) => {
  const requestObj = buildHttpPromise(url, options)
  requestObj.promise = requestObj.promise.catch((err) => {
    if (err && err.message === 'socket hang up') return Promise.reject(new Error(requestMsg.unachievable))
    switch (err && err.code) {
      case 'ETIMEDOUT':
      case 'ESOCKETTIMEDOUT':
        return Promise.reject(new Error(requestMsg.timeout))
      case 'ENOTFOUND':
        return Promise.reject(new Error(requestMsg.notConnectNetwork))
      default:
        return Promise.reject(err)
    }
  })
  return requestObj
}

export const cancelHttp = (requestObj) => {
  if (!requestObj) return
  try { if (requestObj.abort) requestObj.abort() } catch (_) {}
}

export const http = (url, options, cb) => {
  if (typeof options === 'function') { cb = options; options = {} }
  if (options.method == null) options.method = 'get'
  return doRequest(url, options, cb)
}

export const httpGet = (url, options, callback) => {
  if (typeof options === 'function') { callback = options; options = {} }
  return doRequest(url, { ...options, method: 'get' }, callback)
}

export const httpPost = (url, data, options, callback) => {
  if (typeof options === 'function') { callback = options; options = {} }
  return doRequest(url, { ...options, method: 'post', body: data }, callback)
}

export const http_jsonp = (url, options, callback) => {
  if (typeof options === 'function') { callback = options; options = {} }
  const cbName = `jsonp_${Date.now()}_${Math.floor(Math.random() * 1e6)}`
  const sep = url.includes('?') ? '&' : '?'
  doRequest(`${url}${sep}callback=${cbName}`, { ...options, method: 'get' }, (err, resp, body) => {
    if (err) return callback(err)
    try {
      const text = typeof body === 'string' ? body : String(body)
      const m = text.match(new RegExp(`^${cbName}\\((.*)\\)\\s*;?\\s*$`, 's'))
      const data = m ? JSON.parse(m[1]) : JSON.parse(text)
      callback(null, resp, data)
    } catch (e) { callback(e) }
  })
}

export const checkUrl = (url, options = {}) => {
  return httpFetch(url, { ...options, method: 'head' }).promise
}
