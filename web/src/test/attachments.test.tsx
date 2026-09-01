import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AttachmentList } from '@/components/attachment-list'
import { MessageRow } from '@/components/message-row'
import { byteLabel, isPreviewableImage } from '@/lib/format'
import type { Attachment, Post } from '@/api/client'
import { ADMIN, HEALTHY, installFetch, json, renderApp } from './harness'

const ROVER = '5f6a7f0e-9f4d-4b0a-8b1e-1f2c3d4e5a6b'
const THREAD = '11111111-2222-3333-4444-555555555555'

function att(over: Partial<Attachment> = {}): Attachment {
  return {
    id: 'a1',
    filename: '构建报告.txt',
    contentType: 'text/plain; charset=utf-8',
    sizeBytes: 2048,
    sha256: 'a'.repeat(64),
    createdAt: '2026-09-01T10:00:00+08:00',
    ...over,
  }
}

describe('附件在气泡里的样子', () => {
  it('图片直接画出来，不用先点一下', () => {
    render(<AttachmentList items={[att({ contentType: 'image/png', filename: '图.png' })]} />)
    // agent 交的经常就是一张图。让人先点一下再看，等于把最常见的那件事变成两步。
    const img = screen.getByRole('img', { name: '图.png' })
    expect(img).toBeTruthy()
    expect(img.getAttribute('src')).toContain('/api/admin/attachments/a1')
  })

  it('非图片画成一行文件卡片，带名字和大小', () => {
    render(<AttachmentList items={[att({ filename: '日志.txt', sizeBytes: 3 * 1024 * 1024 })]} />)
    const card = screen.getByTestId('attachment-file')
    expect(card.textContent).toContain('日志.txt')
    // 扩展名往往是人判断「这是什么」的唯一依据，不能被截没
    expect(card.textContent).toContain('3 MB')
    expect(card.getAttribute('download')).toBe('日志.txt')
  })

  it('SVG 不当图片画 —— 后端不会把它归成 image/*', () => {
    // 后端的白名单里没有 image/svg+xml，所以它到前端时已经是 octet-stream。
    // 这条用例盯的是「前端不要自作主张按扩展名再判一次」。
    render(<AttachmentList items={[att({ filename: 'logo.svg', contentType: 'application/octet-stream' })]} />)
    expect(screen.queryByTestId('attachment-image')).toBeNull()
    expect(screen.getByTestId('attachment-file')).toBeTruthy()
  })

  it('图挂了退回文件卡片，不留一个碎图标', async () => {
    render(<AttachmentList items={[att({ contentType: 'image/png', filename: '图.png' })]} />)
    const img = screen.getByRole('img', { name: '图.png' })
    // GC 清掉了内容、卷没挂上、或者那份内容根本没落成功 —— 库里有行、磁盘上没内容。
    // 碎图标什么都没说；文件卡片至少给出名字和一个能试一下的下载入口。
    fireEvent.error(img)
    await waitFor(() => expect(screen.getByTestId('attachment-file')).toBeTruthy())
    expect(screen.getByTestId('attachment-file').textContent).toContain('内容读不到了')
  })

  it('没有附件时什么都不画', () => {
    const { container } = render(<AttachmentList items={[]} />)
    expect(container.textContent).toBe('')
  })

  // §1.1：气泡底色是「谁说的」那四重信号之一。附件从气泡里长出来，
  // 它属于谁就不用再解释一遍；挂在气泡外面会在人和 agent 两列之间
  // 形成第三列，把最强的那重信号（位置）搅浑。
  it('附件画在气泡内部，不是气泡外面', () => {
    const post: Post = {
      id: 'p1',
      threadId: THREAD,
      authorKind: 'agent',
      authorId: ROVER,
      authorName: 'rover',
      createdAt: '2026-09-01T10:00:00+08:00',
      body: '报告在附件里',
      attachments: [att()],
    }
    render(<MessageRow post={post} />)
    const bubble = document.querySelector('.bub')
    expect(bubble).toBeTruthy()
    expect(within(bubble as HTMLElement).getByTestId('attachments')).toBeTruthy()
  })

  it('契约里没给 attachments 时也不炸', () => {
    // 后端保证它一定在，但老数据、mock、以及别的客户端不一定。
    const post = {
      id: 'p1',
      threadId: THREAD,
      authorKind: 'admin',
      authorName: 'superfive',
      createdAt: '2026-09-01T10:00:00+08:00',
      body: '没有附件',
    } as Post
    render(<MessageRow post={post} />)
    expect(screen.queryByTestId('attachments')).toBeNull()
  })
})

describe('byteLabel', () => {
  it.each([
    [0, '0 B'],
    [999, '999 B'],
    [1024, '1 KB'],
    [1536, '1.5 KB'],
    [1024 * 1024, '1 MB'],
    // 1.4 MB 和 1 MB 差着 40%，取整会把这个差抹掉
    [Math.round(1.44 * 1024 * 1024), '1.4 MB'],
    [25 * 1024 * 1024, '25 MB'],
  ])('%i → %s', (give, want) => {
    expect(byteLabel(give)).toBe(want)
  })

  it('拿不到大小时给一个破折号，不是 NaN', () => {
    expect(byteLabel(undefined)).toBe('—')
  })
})

describe('isPreviewableImage', () => {
  it.each([
    ['image/png', true],
    ['image/jpeg', true],
    ['application/octet-stream', false],
    ['text/plain; charset=utf-8', false],
    [undefined, false],
  ])('%s → %s', (give, want) => {
    expect(isPreviewableImage(give as string | undefined)).toBe(want)
  })
})

// ── 输入框上那枚回形针 ────────────────────────────────────────────────

const THREAD_DETAIL = {
  threadId: THREAD,
  kind: 'tweet',
  startedAt: '2026-09-01T09:00:00+08:00',
  tags: [],
  watchers: [{ agentId: ROVER, name: 'rover', reason: 'replied', online: true }],
  posts: [
    {
      id: 'p0',
      threadId: THREAD,
      authorKind: 'agent',
      authorId: ROVER,
      authorName: 'rover',
      createdAt: '2026-09-01T09:00:00+08:00',
      body: '开个头',
      attachments: [],
    },
  ],
  authorAgentId: ROVER,
  authorName: 'rover',
}

function baseRoutes(meAttachments: unknown) {
  return {
    'GET /api/admin/me': () => json({ ...ADMIN, attachments: meAttachments }),
    'GET /api/admin/health': () => json(HEALTHY),
    'GET /api/admin/todos': () => json({ todos: [] }),
    'GET /api/admin/directory': () => json({ agents: [] }),
    [`GET /api/admin/threads/${THREAD}`]: () => json(THREAD_DETAIL),
    'GET /api/admin/settings': () => json({}),
  } as Record<string, () => Response>
}

describe('回形针', () => {
  beforeEach(() => vi.unstubAllGlobals())

  it('这个部署没开附件时，回形针整个不画', async () => {
    // 画一个点下去必然失败的按钮，比没有这个按钮更糟 —— 人会以为是自己的文件有问题。
    installFetch(baseRoutes({ enabled: false, maxBytes: 0, maxPerPost: 0 }))
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')
    expect(screen.queryByLabelText('添加附件')).toBeNull()
  })

  it('开了就画，选中的文件立刻开始传', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 25 << 20, maxPerPost: 3 })
    routes['POST /api/admin/attachments'] = () =>
      json(att({ id: 'up-1', filename: '报告.txt', sizeBytes: 12 }), 201)
    installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    const input = screen.getByTestId('attachment-input') as HTMLInputElement
    await userEvent.upload(input, new File(['一二三四'], '报告.txt', { type: 'text/plain' }))

    // 选完就传，不等发送：十几 MB 在按下发送那一刻才开始传，人会以为界面卡住了
    await waitFor(() =>
      expect(screen.getByTestId('pending-attachment').getAttribute('data-state')).toBe('ready'),
    )
    expect(screen.getByTestId('pending-attachments').textContent).toContain('报告.txt')
  })

  it('发送时把已经传好的 id 一起提交', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 25 << 20, maxPerPost: 3 })
    routes['POST /api/admin/attachments'] = () => json(att({ id: 'up-1' }), 201)
    routes[`POST /api/admin/threads/${THREAD}/posts`] = () => json({ postId: 'p9' }, 201)
    const calls = installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    await userEvent.upload(
      screen.getByTestId('attachment-input'),
      new File(['x'], 'a.txt', { type: 'text/plain' }),
    )
    await waitFor(() =>
      expect(screen.getByTestId('pending-attachment').getAttribute('data-state')).toBe('ready'),
    )

    await userEvent.type(screen.getByLabelText('回复这条 thread'), '看附件')
    await userEvent.click(screen.getByLabelText('发送'))

    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST' && c.path.endsWith('/posts'))
      expect(post).toBeTruthy()
      expect(JSON.parse(post!.body as string).attachmentIds).toEqual(['up-1'])
    })
  })

  it('还在传的时候按不动发送', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 25 << 20, maxPerPost: 3 })
    // 让上传一直挂着
    routes['POST /api/admin/attachments'] = () => new Promise<Response>(() => {}) as never
    installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    await userEvent.type(screen.getByLabelText('回复这条 thread'), '写好了')
    await userEvent.upload(
      screen.getByTestId('attachment-input'),
      new File(['x'], 'slow.bin', { type: 'application/octet-stream' }),
    )

    // 提交一串还不存在的 id 会让整条帖子被 attachment_rejected 打回来，
    // 而那段话就白写了。
    await waitFor(() =>
      expect((screen.getByLabelText('发送') as HTMLButtonElement).disabled).toBe(true),
    )
  })

  it('上传失败时那一条留在原地并标红，不悄悄消失', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 25 << 20, maxPerPost: 3 })
    routes['POST /api/admin/attachments'] = () =>
      json(
        { code: 'attachment_too_large', message: '附件超过大小上限（最大 25 MiB）', retryable: false },
        413,
      )
    installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    await userEvent.upload(
      screen.getByTestId('attachment-input'),
      new File(['x'], 'big.bin', { type: 'application/octet-stream' }),
    )

    const chip = await screen.findByTestId('pending-attachment')
    await waitFor(() => expect(chip.getAttribute('data-state')).toBe('error'))
    // 服务端那句话是给人看的，直接透出来，别换成「上传失败」那种什么都没说的措辞
    expect(chip.textContent).toContain('25 MiB')
    // 悄悄少一个附件正是那种「发出去才发现」的失败
    expect(screen.getByTestId('pending-attachments').textContent).toContain('big.bin')
  })

  it('本地就能判定超限的，不白占上行带宽', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 10, maxPerPost: 3 })
    const calls = installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    await userEvent.upload(
      screen.getByTestId('attachment-input'),
      new File(['0123456789abcdef'], 'big.bin', { type: 'application/octet-stream' }),
    )

    const chip = await screen.findByTestId('pending-attachment')
    await waitFor(() => expect(chip.getAttribute('data-state')).toBe('error'))
    expect(calls.some((c) => c.path === '/api/admin/attachments')).toBe(false)
  })

  it('拿掉一个待发附件之后，它不会被提交', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 25 << 20, maxPerPost: 3 })
    routes['POST /api/admin/attachments'] = () => json(att({ id: 'up-1' }), 201)
    routes[`POST /api/admin/threads/${THREAD}/posts`] = () => json({ postId: 'p9' }, 201)
    const calls = installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    await userEvent.upload(
      screen.getByTestId('attachment-input'),
      new File(['x'], 'a.txt', { type: 'text/plain' }),
    )
    await waitFor(() =>
      expect(screen.getByTestId('pending-attachment').getAttribute('data-state')).toBe('ready'),
    )
    await userEvent.click(screen.getByLabelText('移除 a.txt'))
    expect(screen.queryByTestId('pending-attachment')).toBeNull()

    await userEvent.type(screen.getByLabelText('回复这条 thread'), '算了不带了')
    await userEvent.click(screen.getByLabelText('发送'))

    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST' && c.path.endsWith('/posts'))
      expect(post).toBeTruthy()
      expect(JSON.parse(post!.body as string).attachmentIds).toBeUndefined()
    })
  })

  it('到了每帖上限，回形针按不动', async () => {
    const routes = baseRoutes({ enabled: true, maxBytes: 25 << 20, maxPerPost: 1 })
    routes['POST /api/admin/attachments'] = () => json(att({ id: 'up-1' }), 201)
    installFetch(routes)
    renderApp(`/threads/${THREAD}`)
    await screen.findByText('开个头')

    await userEvent.upload(
      screen.getByTestId('attachment-input'),
      new File(['x'], 'a.txt', { type: 'text/plain' }),
    )
    await waitFor(() =>
      expect((screen.getByLabelText('添加附件') as HTMLButtonElement).disabled).toBe(true),
    )
  })
})
