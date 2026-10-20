// 引擎全局代理配置（SDK 请求与音源沙盒请求共用），运行时可改。
let proxy = null // { host, port }
export const setProxy = (cfg) => {
  proxy = cfg && cfg.host ? { host: String(cfg.host), port: Number(cfg.port || 80) } : null
}
export const getProxy = () => proxy
export const proxyUrl = () => (proxy ? `http://${proxy.host}:${proxy.port}` : undefined)
