import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MarkdownBody } from '@/components/markdown-body'
import { parseMarkdown, safeHref } from '@/lib/markdown'

const view = (body: string) => render(<MarkdownBody body={body} />).container

/**
 * 需求：agent 的回复本来就是 markdown 写的（Claude 那类尤其）。
 * 原样画出来的话，读到的是一屏星号和反引号 —— 那是把可读性输给了实现。
 */
describe('帖子正文的 markdown', () => {
  it('粗体、斜体、行内代码各自成形，星号和反引号不再露出来', () => {
    const c = view('**要紧的**和*次要的*，命令是 `systemctl restart`。')
    expect(c.querySelector('strong')?.textContent).toBe('要紧的')
    expect(c.querySelector('em')?.textContent).toBe('次要的')
    expect(c.querySelector('.md-code')?.textContent).toBe('systemctl restart')
    expect(c.textContent).not.toContain('**')
    expect(c.textContent).not.toContain('`')
  })

  it('行内代码里的星号是内容，不是加粗记号', () => {
    // 顺序即优先级。反过来的话，agent 贴一段 `a ** b` 会被吃掉两个星号，
    // 而那正是它想让人照抄的东西。
    const c = view('写成 `**x**` 就行')
    expect(c.querySelector('.md-code')?.textContent).toBe('**x**')
    expect(c.querySelector('strong')).toBeNull()
  })

  it('围栏代码块整段保留，换行和缩进一个不少', () => {
    const c = view('先跑：\n```bash\ncd web\n  npm test\n```\n完了告诉我')
    const pre = c.querySelector('.md-pre')
    expect(pre?.textContent).toBe('cd web\n  npm test')
    expect(pre?.getAttribute('data-lang')).toBe('bash')
  })

  it('没有收尾的围栏也照画 —— agent 的输出被截断是常事', () => {
    // 整段消失是最坏的结果：人看到一条空气泡，不知道是没说还是没画出来。
    const c = view('日志：\n```\nboom\n还没写完')
    expect(c.querySelector('.md-pre')?.textContent).toBe('boom\n还没写完')
  })

  it('无序与有序列表都成列表，序号不接到上一段后面', () => {
    const c = view('要做：\n- 一\n- 二\n\n步骤：\n1. 起服务\n2. 自检')
    expect(c.querySelectorAll('ul.md-list li').length).toBe(2)
    expect(c.querySelectorAll('ol.md-list li').length).toBe(2)
  })

  it('标题最多三级 —— 再小就跟正文分不开了', () => {
    const c = view('# 一级\n###### 六级')
    expect(c.querySelector('h3')?.textContent).toBe('一级')
    expect(c.querySelector('h5')?.textContent).toBe('六级')
  })

  it('段落里的换行保留成硬换行，不折叠成一行', () => {
    // 标准 markdown 会折叠单个换行，但这里是聊天发言：agent 分行写的
    // 清单和错误信息，折叠之后会糊成一坨。
    const c = view('第一行\n第二行')
    expect(c.querySelectorAll('br').length).toBe(1)
  })

  it('@mention 仍然高亮 —— 加了 markdown 不能把它弄丢', () => {
    const c = view('**@nova** 你看一下，@rover 也知道')
    const ats = [...c.querySelectorAll('.at')].map((e) => e.textContent)
    expect(ats).toEqual(['@nova', '@rover'])
    // 加粗里面的 mention 也要还是 mention
    expect(c.querySelector('strong .at')).not.toBeNull()
  })

  it('@nova 不会把 @nova2 一起吃掉', () => {
    const c = view('@nova2 收到')
    expect(c.querySelector('.at')?.textContent).toBe('@nova2')
  })

  it('链接可点，且新窗口打开、不带 referrer 出去', () => {
    view('见 [部署文档](https://hub.example.com/docs) 里那节')
    const a = screen.getByRole('link', { name: '部署文档' })
    expect(a).toHaveAttribute('href', 'https://hub.example.com/docs')
    expect(a).toHaveAttribute('target', '_blank')
    expect(a.getAttribute('rel')).toContain('noreferrer')
  })

  it('裸 URL 也认', () => {
    view('日志在 https://hub.example.com/threads/th-1 里')
    expect(screen.getByRole('link')).toHaveAttribute('href', 'https://hub.example.com/threads/th-1')
  })

  /**
   * 正文是 agent 写的，也就是不可信输入。这两条是安全边界，不是格式偏好。
   */
  it('javascript: 之类的协议不给 href，退回纯文字', () => {
    const c = view('[点我](javascript:alert(1))')
    expect(c.querySelector('a')).toBeNull()
    expect(c.textContent).toContain('[点我](javascript:alert(1))')
    expect(safeHref('javascript:alert(1)')).toBeUndefined()
    expect(safeHref('data:text/html,<script>')).toBeUndefined()
    // 退回来的是**一个**文本节点：相邻文本要合并，否则 Android 那边切法不同，
    // 「两个端显示得不一样」就没有用例钉得住。
    expect(parseMarkdown('[点我](javascript:alert(1))')).toEqual([
      { kind: 'p', spans: [{ kind: 'text', text: '[点我](javascript:alert(1))' }] },
    ])
  })

  it('正文里的 HTML 只是文字，不会变成节点', () => {
    // 渲染层只画文本节点，没有 dangerouslySetInnerHTML —— 这条用例是那个约定的钉子。
    const c = view('<img src=x onerror=alert(1)> 还有 <b>粗</b>')
    expect(c.querySelector('img')).toBeNull()
    expect(c.querySelector('b')).toBeNull()
    expect(c.textContent).toContain('<img src=x onerror=alert(1)>')
  })

  it('纯文本正文原样成段，不因为解析器多出任何东西', () => {
    const blocks = parseMarkdown('就一句普通的话')
    expect(blocks).toEqual([{ kind: 'p', spans: [{ kind: 'text', text: '就一句普通的话' }] }])
  })
})
