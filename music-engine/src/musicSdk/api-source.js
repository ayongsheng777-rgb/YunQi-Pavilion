// 云栖阁音乐引擎：api-source 引擎版。SDK 通过 apis(source).getMusicUrl(musicInfo, quality)
// 拿播放地址；引擎里统一走音源脚本沙盒（与 lx-music 桌面版 user_api 模式行为一致）。
import { sourceManager } from '../server-bridge.js'

const apis = (source) => {
  if (sourceManager.current()) {
    return {
      getMusicUrl(musicInfo, type) {
        const promise = sourceManager.getMusicUrl(source, musicInfo, type).then((url) => ({ type, url }))
        return { canceleFn() {}, promise }
      },
      getLyric(musicInfo) {
        return { canceleFn() {}, promise: sourceManager.current().getLyric(source, musicInfo) }
      },
      getPic(musicInfo) {
        return { canceleFn() {}, promise: sourceManager.current().getPic(source, musicInfo) }
      },
    }
  }
  throw new Error('还没有导入音源脚本，无法获取播放地址')
}

const supportQuality = {
  kw: ['128k', '320k', 'flac', 'flac24bit'],
  kg: ['128k', '320k', 'flac', 'flac24bit'],
  tx: ['128k', '320k', 'flac', 'flac24bit'],
  wy: ['128k', '320k', 'flac', 'flac24bit'],
  mg: ['128k', '320k', 'flac', 'flac24bit'],
}

export { apis, supportQuality }
