import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { installFetch, renderApp } from './harness'
import { USE_MOCKS } from '@/api/queries'

/**
 * 只在 `npm run test:mocks` 里跑（那个脚本会带上 VITE_USE_MOCKS=1），
 * 默认的 `npm run test` 会跳过它 —— 其余用例走的是真网络路径，
 * 打开 mock 开关会把它们的桩数据绕开，两者不能同一趟跑。
 *
 * 它证明的是：后端一个接口都不通时，界面仍然起得来。
 */
const suite = USE_MOCKS ? describe : describe.skip

afterEach(() => vi.unstubAllGlobals())

suite('mock 模式', () => {
  it('后端全 404 时仍然能看界面', async () => {
    installFetch({})
    renderApp('/threads')
    expect(
      await screen.findByRole('heading', { name: '重写 connector 的重试退避逻辑' }),
    ).toBeInTheDocument()
    expect(await screen.findAllByTestId('message-row')).toHaveLength(7)
  })
})
