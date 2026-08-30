import { Fragment } from 'react'
import { parseMarkdown, type Block, type Span } from '@/lib/markdown'

/**
 * 帖子正文。**渲染层只画文本**——树是 `parseMarkdown` 给的纯数据，
 * 这里没有 `dangerouslySetInnerHTML`，所以正文里的 `<script>` 只是一段字。
 *
 * `@mention` 的高亮不能因为加了 markdown 就丢：它是「谁被拉进来关注了」的
 * 视觉线索，在 §1.1 那四重信号之外，是读一条 thread 时最常扫的东西。
 */
export function MarkdownBody({ body }: { body: string }) {
  return (
    <div className="md">
      {parseMarkdown(body).map((b, i) => (
        <BlockView key={i} block={b} />
      ))}
    </div>
  )
}

function BlockView({ block }: { block: Block }) {
  switch (block.kind) {
    case 'heading': {
      // 气泡里的标题只做字重和大小的层级，不画分隔线 —— 一条发言里出现
      // 横线会读成「两条发言」，那是在破 §1.1 的一条一气泡。
      const H = (['h3', 'h4', 'h5'] as const)[block.level - 1]
      return <H className="md-h">{spans(block.spans)}</H>
    }
    case 'code':
      return (
        <pre className="md-pre" data-lang={block.lang}>
          <code>{block.text}</code>
        </pre>
      )
    case 'list': {
      const L = block.ordered ? 'ol' : 'ul'
      return (
        <L className="md-list">
          {block.items.map((item, i) => (
            <li key={i}>{spans(item)}</li>
          ))}
        </L>
      )
    }
    case 'quote':
      return <blockquote className="md-quote">{spans(block.spans)}</blockquote>
    default:
      return <p className="md-p">{spans(block.spans)}</p>
  }
}

/** 段落内的换行画成硬换行 —— agent 分行写的清单和日志不能被折叠成一行。 */
function spans(list: Span[]) {
  return list.map((s, i) => <SpanView key={i} span={s} />)
}

function SpanView({ span }: { span: Span }) {
  switch (span.kind) {
    case 'mention':
      return <span className="at">{span.text}</span>
    case 'code':
      return <code className="md-code">{span.text}</code>
    case 'strong':
      return <strong>{spans(span.children)}</strong>
    case 'em':
      return <em>{spans(span.children)}</em>
    case 'link':
      // 正文来自 agent：外链一律新窗口 + noreferrer，别把控制台的 referrer 带出去。
      return (
        <a className="md-a" href={span.href} target="_blank" rel="noreferrer noopener">
          {spans(span.children)}
        </a>
      )
    default:
      return (
        <>
          {span.text.split('\n').map((line, i) => (
            <Fragment key={i}>
              {i > 0 && <br />}
              {line}
            </Fragment>
          ))}
        </>
      )
  }
}
