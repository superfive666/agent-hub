import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query'
import {
  api,
  unwrap,
  type AdminMe,
  type AgentSummary,
  type BoardActivity,
  type BoardStarted,
  type Health,
  type Settings,
  type ThreadDetail,
  type TodoSummary,
} from './client'
import {
  mockBoardActivity,
  mockBoardStarted,
  mockDirectory,
  mockHealth,
  mockMe,
  mockSettings,
  mockThread,
  mockTodos,
} from '@/mocks/data'

/**
 * `VITE_USE_MOCKS=1` 时全部走 src/mocks/data.ts —— 后端没起也能看界面。
 * 假数据的形状就是契约类型，所以打开/关掉这个开关不会改变组件的代码路径。
 */
export const USE_MOCKS = import.meta.env.VITE_USE_MOCKS === '1'

export const qk = {
  me: ['me'] as const,
  thread: (id: string) => ['thread', id] as const,
  todos: () => ['todos'] as const,
  directory: ['directory'] as const,
  board: (date: string, groupBy: BoardGroupBy) => ['board', date, groupBy] as const,
  settings: ['settings'] as const,
  health: ['health'] as const,
}

export type BoardGroupBy = 'activity' | 'started'

/** 当前管理员。401 就是"没登录" —— RequireAuth 靠它判断。 */
export function useMe(options?: Partial<UseQueryOptions<AdminMe>>) {
  return useQuery<AdminMe>({
    queryKey: qk.me,
    queryFn: async () => {
      if (USE_MOCKS) return mockMe
      return unwrap(await api.GET('/api/admin/me'))
    },
    retry: false,
    staleTime: 5 * 60_000,
    ...options,
  })
}

export function useThread(threadId: string | undefined) {
  return useQuery<ThreadDetail>({
    queryKey: qk.thread(threadId ?? ''),
    enabled: !!threadId,
    queryFn: async () => {
      if (USE_MOCKS) return mockThread(threadId!)
      return unwrap(
        await api.GET('/api/agent/threads/{threadId}', { params: { path: { threadId: threadId! } } }),
      )
    },
  })
}

export function useTodos() {
  return useQuery<TodoSummary[]>({
    queryKey: qk.todos(),
    queryFn: async () => {
      if (USE_MOCKS) return mockTodos
      const data = unwrap(await api.GET('/api/admin/todos'))
      return data?.todos ?? []
    },
  })
}

/**
 * 名录。契约里只有 agent 侧的 `/api/agent/directory` 定义了响应体
 * （`/api/admin/agents` 的 200 没写 schema），所以这里读的是它。
 */
export function useDirectory() {
  return useQuery<AgentSummary[]>({
    queryKey: qk.directory,
    queryFn: async () => {
      if (USE_MOCKS) return mockDirectory
      const data = unwrap(await api.GET('/api/agent/directory'))
      return data?.agents ?? []
    },
  })
}

export function useBoard(date: string, groupBy: BoardGroupBy) {
  return useQuery<BoardActivity | BoardStarted>({
    queryKey: qk.board(date, groupBy),
    queryFn: async () => {
      if (USE_MOCKS) return groupBy === 'started' ? mockBoardStarted : mockBoardActivity
      return unwrap(await api.GET('/api/admin/board', { params: { query: { date, groupBy } } }))
    },
  })
}

export function useSettings() {
  return useQuery<Settings>({
    queryKey: qk.settings,
    queryFn: async () => {
      if (USE_MOCKS) return mockSettings
      return unwrap(await api.GET('/api/admin/settings'))
    },
  })
}

/**
 * 运行状态。`outboxLagSeconds` 是唯一能发现 worker 静默死亡的指标，
 * 所以这个 query 失败时**不能**静默 —— 见 OutboxAlert。
 */
export function useHealth() {
  return useQuery<Health>({
    queryKey: qk.health,
    queryFn: async () => {
      if (USE_MOCKS) return mockHealth
      return unwrap(await api.GET('/api/admin/health'))
    },
    refetchInterval: 30_000,
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: { username: string; password: string }) => {
      if (USE_MOCKS) return
      // 会话写在 HttpOnly Cookie 里，靠 client 上的 credentials:'include'
      unwrap(await api.POST('/api/admin/login', { body }))
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.me }),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      if (!USE_MOCKS) unwrap(await api.POST('/api/admin/logout'))
    },
    onSuccess: () => qc.clear(),
  })
}

export interface NewTodoInput {
  title: string
  body: string
  /** 必选且唯一。缺失后端直接 422，前端也不给提交 */
  primaryAgentId: string
  dueAt?: string
  tags?: string[]
}

export function useCreateTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: NewTodoInput) => {
      if (USE_MOCKS) return { threadId: 'th-new', startedAt: new Date().toISOString() }
      return unwrap(await api.POST('/api/admin/todos', { body: input }))
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.todos() }),
  })
}

/** 管理员确认完成 / 打回 / 取消。状态由 thread 里的动作驱动，没有独立的状态面板。 */
export function useTodoAction(threadId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (action: 'confirm' | 'reject' | 'cancel') => {
      if (USE_MOCKS) return
      unwrap(
        await api.POST('/api/admin/todos/{threadId}/state', {
          params: { path: { threadId: threadId! } },
          body: { action },
        }),
      )
    },
    onSuccess: () => {
      if (threadId) qc.invalidateQueries({ queryKey: qk.thread(threadId) })
      qc.invalidateQueries({ queryKey: qk.todos() })
    },
  })
}

/** 以人类身份回帖：authorKind=admin，界面据此靠右 + 换底色 + 挂「人类」chip。 */
export function useCreatePost(threadId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: { body: string; mentions?: string[] }) => {
      if (USE_MOCKS) return
      unwrap(
        await api.POST('/api/admin/threads/{threadId}/posts', {
          params: { path: { threadId: threadId! } },
          body,
        }),
      )
    },
    onSuccess: () => {
      if (threadId) qc.invalidateQueries({ queryKey: qk.thread(threadId) })
    },
  })
}
