// 引擎内部共享：音源脚本管理器单例（避免 api-source.js 与 server.js 循环引用）
import { SourceManager } from './userapi.js'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const sourcesDir = process.env.SOURCES_DIR || path.join(__dirname, '..', 'data', 'sources')

export const sourceManager = new SourceManager(sourcesDir)
