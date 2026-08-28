import { Fragment } from 'react'
import { Link2 } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Bubble, type BubbleTone } from '@/components/ui/bubble'
import { Chip } from '@/components/ui/chip'
import { cn } from '@/lib/cn'
import type { Post } from '@/mocks/thread'

/** 正文里的 @name 高亮成 mention */
function renderBody(body: string) {
  return body.split('\n').map((line, li) => (
    <Fragment key={li}>
      {li > 0 && <br />}
      {line.split(/(@[a-z0-9_-]+)/gi).map((part, i) =>
        part.startsWith('@') ? (
          <span key={i} className="at">
            {part}
          </span>
        ) : (
          <Fragment key={i}>{part}</Fragment>
        ),
      )}
    </Fragment>
  ))
}

/**
 * §1.1 四重信号叠加，少一重都不行：
 *   位置（人靠右 / agent 靠左）+ 气泡底色 + `@` 前缀 + 「人类」chip。
 * 位置是最强的那一重 —— 任何布局改动都不能把人和 agent 混进同一列。
 */
export function MessageRow({ post }: { post: Post }) {
  const { author } = post
  const human = author.isHuman
  const tone: BubbleTone = human ? 'me' : author.participation === 'primary' ? 'primary' : 'watch'

  return (
    <div className={cn('msg', human && 'msg-me')} data-testid="message-row" data-human={human}>
      <Avatar
        kind={human ? 'human' : author.participation === 'primary' ? 'primary' : 'agent'}
        size="sm"
        initials={author.initials}
        online={human ? undefined : author.online}
        label={human ? author.name : `@${author.name}`}
      />
      <div className="min-w-0">
        <div className="who">
          {human ? (
            <>
              <span className="t">{post.at}</span>
              <span>{author.name}</span>
              {/* 人类 chip：四重信号的第四重，任何断点下都不能省 */}
              <Chip tone="human" size="sm">
                人类
              </Chip>
            </>
          ) : (
            <>
              <span>@{author.name}</span>
              {author.participation === 'primary' ? (
                <Chip tone="agent" size="sm">
                  主 agent
                </Chip>
              ) : (
                <Chip size="sm">关注</Chip>
              )}
              <span className="t">{post.at}</span>
            </>
          )}
        </div>
        <Bubble tone={tone}>{renderBody(post.body)}</Bubble>
        {post.deliverables && post.deliverables.length > 0 && (
          <div
            className="mt-[9px] flex flex-col gap-[7px] rounded-[14px] px-[13px] py-[11px]"
            style={{ background: 'var(--agent-soft)' }}
          >
            <span className="lbl" style={{ color: 'var(--agent-ink)' }}>
              交付物 · {post.deliverables.length}
            </span>
            {post.deliverables.map((d) => (
              <div
                key={d.label}
                className="flex items-center gap-2 text-[11.5px] font-semibold leading-[1.4]"
                style={{ color: 'var(--agent-ink)' }}
              >
                <Link2 size={14} aria-hidden />
                <span>{d.label}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
