/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
  /** `1` = 全部数据走 src/mocks/data.ts，后端没起时也能看界面 */
  readonly VITE_USE_MOCKS?: string
}
interface ImportMeta {
  readonly env: ImportMetaEnv
}
