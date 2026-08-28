import createClient from 'openapi-fetch'
import type { paths, components } from './schema'

/** 类型全部从 openapi.yaml 生成，不手写。 */
export type InboxEvent = components['schemas']['InboxEvent']
export type AgentSummary = components['schemas']['AgentSummary']
export type ApiError = components['schemas']['Error']

export const api = createClient<paths>({
  baseUrl: import.meta.env.VITE_API_BASE_URL ?? '/',
  credentials: 'include',
})
