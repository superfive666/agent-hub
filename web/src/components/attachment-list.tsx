import { useState } from 'react'
import { Download, FileText, ImageOff } from 'lucide-react'
import { attachmentUrl, type Attachment } from '@/api/client'
import { byteLabel, isPreviewableImage } from '@/lib/format'
import { cn } from '@/lib/cn'

export interface AttachmentListProps {
  items: Attachment[]
  className?: string
}

/**
 * 一条发言里挂着的文件，画在气泡**内部**。
 *
 * 画在内部是有理由的：气泡的底色是「谁说的」那四重信号之一（§1.1），
 * 附件从气泡里长出来，它属于谁就不用再解释一遍。挂在气泡外面的话，
 * 一串附件卡片会在人和 agent 的两列之间形成第三列，把最强的那重信号
 * （位置）搅浑。
 *
 * 两种形态，按后端归一化之后的 contentType 分：
 * - 图片 → 直接画出来。agent 交的经常就是一张图，让人先点一下再看
 *   等于把最常见的那件事变成两步。
 * - 其它 → 一行文件卡片：名字、大小、一个下载按钮。
 */
export function AttachmentList({ items, className }: AttachmentListProps) {
  if (items.length === 0) return null
  return (
    <div className={cn('mt-2 flex flex-col gap-2', className)} data-testid="attachments">
      {items.map((a) => (
        <AttachmentItem key={a.id} a={a} />
      ))}
    </div>
  )
}

function AttachmentItem({ a }: { a: Attachment }) {
  const [broken, setBroken] = useState(false)
  const href = attachmentUrl(a.id)

  if (isPreviewableImage(a.contentType) && !broken) {
    return (
      <a
        href={href}
        target="_blank"
        rel="noreferrer"
        className="block overflow-hidden rounded-[14px]"
        style={{ border: '1px solid var(--inset-bd)', background: 'var(--inset-bg)' }}
        title={`${a.filename} · ${byteLabel(a.sizeBytes)}`}
        data-testid="attachment-image"
      >
        <img
          src={href}
          alt={a.filename}
          loading="lazy"
          // 缩略图限高，不限宽：一张长截图铺开会把整条消息流撑成一屏一张。
          // object-contain 保证不裁掉内容 —— 裁掉的那部分正是人要看的东西。
          className="block max-h-[280px] w-auto max-w-full object-contain"
          /**
           * 图挂了要退回文件卡片，不能留一个碎图标。
           *
           * 这不是理论情况：GC 把内容清掉了、卷没挂上、或者那份内容
           * 根本就没落成功 —— 库里有行、磁盘上没内容。碎图标什么都没说，
           * 文件卡片至少给出名字和一个能试一下的下载入口。
           */
          onError={() => setBroken(true)}
        />
      </a>
    )
  }

  return (
    <a
      href={href}
      // download 让点击直接落盘而不是导航过去。
      // 后端那边 Content-Disposition 已经是 attachment 了，这里是第二层 ——
      // 两层都在，是因为它们防的不是同一件事：那边防的是「被当页面渲染」，
      // 这里管的是「点一下的手感」。
      download={a.filename}
      className="flex items-center gap-2.5 rounded-[14px] px-3 py-2.5 no-underline"
      style={{
        border: '1px solid var(--inset-bd)',
        background: 'var(--inset-bg)',
        color: 'var(--ink)',
      }}
      data-testid="attachment-file"
    >
      {broken ? (
        <ImageOff size={16} style={{ color: 'var(--warn)', flexShrink: 0 }} aria-hidden />
      ) : (
        <FileText size={16} style={{ color: 'var(--ink2)', flexShrink: 0 }} aria-hidden />
      )}
      <span className="min-w-0 grow">
        {/* 文件名可能很长，而且可能是一长串没有空格的中文或路径式命名。
            break-all + 两行截断：既不撑破气泡，也不把扩展名藏起来 ——
            扩展名往往是人判断「这是什么」的唯一依据。 */}
        <span className="block break-all text-[12.5px] font-semibold leading-[1.4]">
          {a.filename}
        </span>
        <span
          className="mt-0.5 block text-[10.5px] font-semibold leading-none"
          style={{ color: 'var(--ink2)' }}
        >
          {broken ? '内容读不到了' : byteLabel(a.sizeBytes)}
        </span>
      </span>
      <Download size={15} style={{ color: 'var(--ink2)', flexShrink: 0 }} aria-hidden />
    </a>
  )
}
