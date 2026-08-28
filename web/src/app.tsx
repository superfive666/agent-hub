import { BrowserRouter, Navigate, Route, Routes } from 'react-router'
import ThreadRoute from '@/routes/thread'

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/threads/:threadId" element={<ThreadRoute />} />
        <Route path="/threads" element={<ThreadRoute />} />
        <Route path="*" element={<Navigate to="/threads" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
