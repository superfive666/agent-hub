import { BrowserRouter, Navigate, Route, Routes } from 'react-router'
import { HOME } from '@/components/app-shell'
import { RequireAuth } from '@/components/require-auth'
import BoardRoute from '@/routes/board'
import DirectoryRoute from '@/routes/directory'
import LoginRoute from '@/routes/login'
import NewAgentRoute from '@/routes/new-agent'
import NewTodoRoute from '@/routes/new-todo'
import SettingsRoute from '@/routes/settings'
import ThreadRoute from '@/routes/thread'
import TodosRoute from '@/routes/todos'

/** 路由表。除登录页外一律要会话，没有会话就被 RequireAuth 送去 /login。 */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginRoute />} />
      <Route
        path="/*"
        element={
          <RequireAuth>
            <Routes>
              <Route path="/threads/:threadId" element={<ThreadRoute />} />
              <Route path="/threads" element={<ThreadRoute />} />
              <Route path="/board" element={<BoardRoute />} />
              {/* /agents/new 不行：AppShell 的 navValueOf 用 startsWith 判高亮，
                  挂在 /directory 下面侧栏才会继续亮着「名录」 */}
              <Route path="/directory/new" element={<NewAgentRoute />} />
              <Route path="/directory" element={<DirectoryRoute />} />
              <Route path="/todos/new" element={<NewTodoRoute />} />
              <Route path="/todos" element={<TodosRoute />} />
              <Route path="/settings" element={<SettingsRoute />} />
              <Route path="*" element={<Navigate to={HOME} replace />} />
            </Routes>
          </RequireAuth>
        }
      />
    </Routes>
  )
}

export function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  )
}
