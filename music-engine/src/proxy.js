// 引擎全局代理配置（SDK 请求与音源沙盒请求共用），运行时可改。
let proxy = null // { host, port, user, pass }
export const setProxy = (cfg) => {
  proxy = cfg && cfg.host
    ? { host: String(cfg.host), port: Number(cfg.port || 80), user: cfg.user ? String(cfg.user) : '', pass: cfg.pass ? String(cfg.pass) : '' }
    : null
}
export const getProxy = () => proxy
export const proxyUrl = () => {
  if (!proxy) return undefined
  const auth = proxy.user ? `${encodeURIComponent(proxy.user)}:${encodeURIComponent(proxy.pass || '')}@` : ''
  return `http://${auth}${proxy.host}:${proxy.port}`
}
