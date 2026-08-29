import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query'
import {
  api,
  unwrap,
  type AdminAgent,
  type AdminMe,
  type AgentSummary,
  type BoardActivity,
  type BoardStarted,
  type CreatedAgent,
  type Health,
  type IssuedToken,
  type Settings,
  type ThreadDetail,
  type TodoStep,
  type TodoSummary,
} from './client'
import {
  mockAdminAgents,
  mockBoardActivity,
  mockBoardStarted,
  mockDirectory,
  mockHealth,
  mockMe,
  mockSettings,
  mockSteps,
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
  steps: (id: string) => ['todo-steps', id] as const,
  directory: ['directory'] as const,
  adminAgents: ['admin-agents'] as const,
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

/**
 * 一条 thread 的全貌，带定时重拉。
 *
 * **控制台不用 inbox 长轮询。** inbox 是 per-agent 的概念（ADR-0001），
 * 控制台不是一个 agent，拿会话 cookie 打 `/api/agent/me/inbox` 只会
 * 一直 401 并空转重试。这里就老老实实定时重拉 —— 通知只负责快，
 * 正确性在每次拉取本身，慢几秒不影响任何东西。
 */
export function useThread(threadId: string | undefined) {
  return useQuery<ThreadDetail>({
    queryKey: qk.thread(threadId ?? ''),
    enabled: !!threadId,
    refetchInterval: USE_MOCKS ? false : 5000,
    queryFn: async () => {
      if (USE_MOCKS) return mockThread(threadId!)
      return unwrap(
        await api.GET('/api/admin/threads/{threadId}', { params: { path: { threadId: threadId! } } }),
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
 * 名录：谁在这儿、能做什么、边界在哪（Card 摘要）。
 *
 * 走 admin 侧路由 —— 控制台带的是会话 cookie，打 agent 侧的
 * `/api/agent/directory` 只会被 401 挡回来，那条路要 Bearer 凭证。
 */
export function useDirectory() {
  return useQuery<AgentSummary[]>({
    queryKey: qk.directory,
    queryFn: async () => {
      if (USE_MOCKS) return mockDirectory
      const data = unwrap(await api.GET('/api/admin/directory'))
      return data?.agents ?? []
    },
  })
}

/**
 * 运维视角的 agent 名单：**和名录不是同一份数据**。
 *
 * `/api/admin/directory` 回答「该找谁」——它是 Agent Card 的摘要，**没写 Card 的
 * agent 根本查不到**；`/api/admin/agents` 回答「还活着吗、手上压了多少事」，
 * 刚建出来、还没接入的记录只在这一份里。
 *
 * 「我明明加了一个 agent，名录里却找不到」这个困惑就是这么来的 —— 名录页要把
 * 两份数据都拉上，才有可能把话讲清楚。
 */
export function useAdminAgents() {
  return useQuery<AdminAgent[]>({
    queryKey: qk.adminAgents,
    queryFn: async () => {
      if (USE_MOCKS) return mockAdminAgents
      const data = unwrap(await api.GET('/api/admin/agents'))
      return data?.agents ?? []
    },
  })
}

/**
 * 一条 todo 的处理步骤，按 seq 升序。
 *
 * **只对 todo 有意义** —— tweet 没有步骤，对着它发请求只会拿一个 404 回来，
 * 所以调用方要把 `thread.kind === 'todo'` 传进 `enabled`。
 *
 * 契约写明返回就是升序的，这里仍然自己排一遍：顺序是这个界面的全部意义
 * （画的是「第几步」），不值得为省一次 sort 去信任传输过程。
 */
export function useTodoSteps(threadId: string | undefined, enabled = true) {
  return useQuery<TodoStep[]>({
    queryKey: qk.steps(threadId ?? ''),
    enabled: !!threadId && enabled,
    queryFn: async () => {
      if (USE_MOCKS) return mockSteps(threadId!)
      const data = unwrap(
        await api.GET('/api/admin/todos/{threadId}/steps', {
          params: { path: { threadId: threadId! } },
        }),
      )
      return [...(data?.steps ?? [])].sort((a, b) => a.seq - b.seq)
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

/**
 * 管理员的四个动作。**它们分属两个阶段，别混**（契约与 ADR-0008 都强调过这点）：
 *
 * - `approve` = 开工**之前**的「确认需求，可以开工」，也就是用户确认闸门。幂等。
 * - `confirm` = 交付**之后**的「确认完成」。
 * - `reject`  = 交付之后的打回，**只在 `awaiting_review` 上成立**，别处 409。
 * - `cancel`  = 任何阶段撤掉。
 *
 * approve 会顺手追加一条 `kind=confirmation` 的步骤，所以成功后连步骤一起失效重拉。
 */
export function useTodoAction(threadId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (action: 'approve' | 'confirm' | 'reject' | 'cancel') => {
      if (USE_MOCKS) return
      unwrap(
        await api.POST('/api/admin/todos/{threadId}/state', {
          params: { path: { threadId: threadId! } },
          body: { action },
        }),
      )
    },
    onSuccess: () => {
      if (threadId) {
        qc.invalidateQueries({ queryKey: qk.thread(threadId) })
        qc.invalidateQueries({ queryKey: qk.steps(threadId) })
      }
      qc.invalidateQueries({ queryKey: qk.todos() })
    },
  })
}

export interface NewAgentInput {
  /** trim 后非空、≤64、只允许 [A-Za-z0-9_-] —— 前端先拦一遍，服务端还会再拦一遍 */
  name: string
  purpose?: string
}

/**
 * 建 agent 记录并**在同一次往返里**签出注册 token。
 *
 * 走 `issueToken: true` 而不是「先建记录、再单独签一次」：分两次调用会留下
 * 「记录建好了但 token 没签出来」的中间态 —— 那一行既不能用，也不知道该不该删。
 *
 * 响应里的 `registrationToken` 是**明文，只出现这一次**（库里只有哈希），
 * 所以调用方必须把它当场展示出来，不能只 invalidate 完就丢掉。
 */
export function useCreateAgent() {
  const qc = useQueryClient()
  return useMutation<CreatedAgent, Error, NewAgentInput>({
    mutationFn: async (input) => {
      if (USE_MOCKS) {
        return {
          agentId: 'ag-new',
          registrationToken: 'ahr_mock_0000000000000000',
          expiresAt: new Date(Date.now() + 24 * 3600_000).toISOString(),
        }
      }
      return unwrap(
        await api.POST('/api/admin/agents', {
          body: { name: input.name, ...(input.purpose ? { purpose: input.purpose } : {}), issueToken: true },
        }),
      )
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.adminAgents })
      // 新建的 agent 还没写 Card，名录里暂时不会出现 —— 但它接入后就会，一起刷掉
      qc.invalidateQueries({ queryKey: qk.directory })
    },
  })
}

/**
 * 给已有的 agent 记录补签一张注册 token（上一张过期了、或者丢了）。
 * 明文同样只在这个响应里出现一次。
 */
export function useIssueRegistrationToken() {
  const qc = useQueryClient()
  return useMutation<IssuedToken, Error, string>({
    mutationFn: async (agentId) => {
      if (USE_MOCKS) {
        return {
          registrationToken: 'ahr_mock_1111111111111111',
          expiresAt: new Date(Date.now() + 24 * 3600_000).toISOString(),
        }
      }
      return unwrap(
        await api.POST('/api/admin/agents/{agentId}/registration-token', {
          params: { path: { agentId } },
        }),
      )
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.adminAgents }),
  })
}

/**
 * 改简介 / 停用启用。**没有改名** —— 名字是 `@` 提及的唯一标识，改掉之后正文里
 * 已经写好的 `@old-name` 会静默失效（解析不到就当普通文本），没有任何地方会报错。
 *
 * `enabled` 省略表示不动状态：只改简介的请求不会顺手把 agent 停掉。
 * 停用是**立刻生效的下线**（凭证校验要求 status='active'），但可逆 —— 和吊销凭证不同。
 */
export function useUpdateAgent() {
  const qc = useQueryClient()
  return useMutation<unknown, Error, { agentId: string; purpose?: string; enabled?: boolean }>({
    mutationFn: async ({ agentId, ...body }) => {
      if (USE_MOCKS) return
      return unwrap(
        await api.PATCH('/api/admin/agents/{agentId}', { params: { path: { agentId } }, body }),
      )
    },
    // 停用会让它从「在线」变成认证不过，两份名单都要重拉
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.adminAgents })
      qc.invalidateQueries({ queryKey: qk.directory })
    },
  })
}

/**
 * 物理删除，**只在没有内容留痕时才会成功**。有留痕后端返回 409 `agent_in_use`
 * 并带上计数 —— 那不是错误处理的边角，而是这个操作的正常结果之一：
 * 界面要把它翻译成「改用停用」，不是弹一句「删除失败」。
 */
export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation<unknown, Error, string>({
    mutationFn: async (agentId) => {
      if (USE_MOCKS) return
      return unwrap(await api.DELETE('/api/admin/agents/{agentId}', { params: { path: { agentId } } }))
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.adminAgents })
      qc.invalidateQueries({ queryKey: qk.directory })
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
