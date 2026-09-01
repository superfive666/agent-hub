import { Avatar } from '@/components/ui/avatar'
import { Bubble, type BubbleTone } from '@/components/ui/bubble'
import { Chip } from '@/components/ui/chip'
import { cn } from '@/lib/cn'
import type { Post, ThreadDetail } from '@/api/client'
import { authorOf, timeLabel } from '@/lib/format'
import { MarkdownBody } from '@/components/markdown-body'
import { AttachmentList } from '@/components/attachment-list'

export interface MessageRowProps {
  post: Post
  /** 主 agent / 关注者的判定要看整条 thread，所以把它带进来 */
  thread?: Pick<ThreadDetail, 'primaryAgentId' | 'watchers'>
}

/**
 * §1.1 四重信号叠加，少一重都不行：
 *   位置（人靠右 / agent 靠左）+ 气泡底色 + `@` 前缀 + 「人类」chip。
 * 位置是最强的那一重 —— 任何布局改动都不能把人和 agent 混进同一列。
 *
 * 判定依据只有契约里的 `authorKind`：admin = 人，agent = 机器。
 */
export function MessageRow({ post, thread }: MessageRowProps) {
  const author = authorOf(post, thread)
  const human = author.isHuman
  const tone: BubbleTone = human ? 'me' : author.participation === 'primary' ? 'primary' : 'watch'
  const at = timeLabel(post.createdAt)

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
              <span className="t">{at}</span>
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
              <span className="t">{at}</span>
            </>
          )}
        </div>
        <Bubble tone={tone}>
          <MarkdownBody body={post.body} />
          {/* 附件画在气泡**内部**：气泡底色是「谁说的」那四重信号之一（§1.1），
              从气泡里长出来的东西不用再解释一遍它属于谁。挂在外面的话，
              一串附件卡片会在人和 agent 两列之间形成第三列，
              把最强的那重信号（位置）搅浑。 */}
          <AttachmentList items={post.attachments ?? []} />
        </Bubble>
      </div>
    </div>
  )
}
