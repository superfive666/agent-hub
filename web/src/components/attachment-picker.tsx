import { useRef, useState } from 'react'
import { Paperclip, X } from 'lucide-react'
import { uploadAttachment, type Attachment } from '@/api/client'
import { byteLabel } from '@/lib/format'
import { Button } from '@/components/ui/button'

/** 一个待发的附件：正在传、传好了、或者传失败了。 */
export interface Pending {
  /** 本地临时 key。传成功前还没有服务端 id，列表要靠它做 React key 和删除 */
  key: string
  name: string
  size: number
  /** 传成功之后才有。发帖时提交的就是这些 id */
  attachment?: Attachment
  error?: string
}

export interface AttachmentPickerProps {
  items: Pending[]
  onChange: (next: Pending[]) => void
  maxBytes: number
  maxPerPost: number
  disabled?: boolean
}

/**
 * 输入框上那枚回形针，以及它下面那排待发附件。
 *
 * **选完就传，不等发送。** 两个理由：
 * 1. 十几 MB 的文件在按下发送那一刻才开始传，人会以为界面卡住了 ——
 *    而这时他已经把话写完了，最不该等的就是这一刻。
 * 2. 传失败（太大、格式不收、hub 磁盘满）要在他还在写的时候就说，
 *    不是等他按下发送才把整条消息连同那段话一起打回来。
 *
 * 所以这里的状态机是三态的：传输中 / 传好了 / 传失败了。
 * 失败的那一条**留在列表里并标红**，不自动消失 —— 悄悄少一个附件
 * 正是那种「发出去才发现」的失败。
 */
export function AttachmentPicker({
  items,
  onChange,
  maxBytes,
  maxPerPost,
  disabled,
}: AttachmentPickerProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [full, setFull] = useState(false)
  const atLimit = items.length >= maxPerPost

  const pick = (files: FileList | null) => {
    if (!files || files.length === 0) return
    const room = maxPerPost - items.length
    const take = Array.from(files).slice(0, room)
    // 选多了要说一声。悄悄丢掉多出来的那几个，人是不会发现的 ——
    // 他记得自己选了 10 个，界面上只有 8 个，而没有任何提示。
    setFull(files.length > room)

    const fresh: Pending[] = take.map((f) => ({
      key: `${Date.now()}-${f.name}-${Math.random().toString(36).slice(2, 8)}`,
      name: f.name,
      size: f.size,
    }))
    let current = [...items, ...fresh]
    onChange(current)

    take.forEach((file, i) => {
      const key = fresh[i].key
      const settle = (patch: Partial<Pending>) => {
        // 每次都基于最新的 current 算，不要闭包捕获一份旧的 items ——
        // 同时传三个文件时，后完成的那个会把先完成的结果覆盖掉。
        current = current.map((p) => (p.key === key ? { ...p, ...patch } : p))
        onChange(current)
      }
      // 大小在本地先拦一道。让它传上去再被 413 打回来也能工作，
      // 但那是白白占着上行带宽等一个已经知道的答案。
      if (maxBytes > 0 && file.size > maxBytes) {
        settle({ error: `超过单个附件上限（${byteLabel(maxBytes)}）` })
        return
      }
      uploadAttachment(file)
        .then((a) => settle({ attachment: a }))
        .catch((e: unknown) => settle({ error: e instanceof Error ? e.message : '上传失败' }))
    })

    // 同一个文件连选两次也要能触发 change
    if (inputRef.current) inputRef.current.value = ''
  }


  return (
    <>
      <input
        ref={inputRef}
        type="file"
        multiple
        className="sr-only"
        // 不设 accept：agent 的产物什么都有可能，白名单只影响下载时回显什么
        // Content-Type，不该在这里把人能挑的东西也限死。
        onChange={(e) => pick(e.target.files)}
        data-testid="attachment-input"
      />
      <Button
        type="button"
        aria-label="添加附件"
        title={atLimit ? `一条消息最多 ${maxPerPost} 个附件` : '添加附件'}
        className="px-3 py-2.5"
        disabled={disabled || atLimit}
        onClick={() => inputRef.current?.click()}
      >
        <Paperclip size={16} />
      </Button>
      {full && (
        <span className="sr-only" role="status">
          一条消息最多 {maxPerPost} 个附件，多出来的没有加进来
        </span>
      )}
    </>
  )
}

/** 待发附件那一排。放在输入框下面，不占输入区的横向空间。 */
export function PendingAttachments({
  items,
  onRemove,
  maxPerPost,
  overflow,
}: {
  items: Pending[]
  onRemove: (key: string) => void
  maxPerPost: number
  overflow?: boolean
}) {
  if (items.length === 0 && !overflow) return null
  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5" data-testid="pending-attachments">
      {items.map((p) => (
        <span
          key={p.key}
          className="flex max-w-full items-center gap-1.5 rounded-pill py-1 pl-2.5 pr-1 text-[11px] font-semibold"
          style={{
            // 失败的那一条标红并留在原地。悄悄消失正是「发出去才发现少了个附件」
            // 的来源；留着它，人至少知道要重传。
            border: `1px solid ${p.error ? 'var(--alert)' : 'var(--inset-bd)'}`,
            background: 'var(--inset-bg)',
            color: p.error ? 'var(--alert)' : 'var(--ink)',
          }}
          data-testid="pending-attachment"
          data-state={p.error ? 'error' : p.attachment ? 'ready' : 'uploading'}
          title={p.error ?? p.name}
        >
          <span className="truncate">{p.name}</span>
          <span style={{ color: p.error ? 'var(--alert)' : 'var(--ink2)' }}>
            {p.error ? p.error : p.attachment ? byteLabel(p.size) : '上传中…'}
          </span>
          <button
            type="button"
            aria-label={`移除 ${p.name}`}
            onClick={() => onRemove(p.key)}
            className="grid h-4 w-4 shrink-0 place-items-center rounded-full"
            style={{ color: 'var(--ink2)' }}
          >
            <X size={11} />
          </button>
        </span>
      ))}
      {overflow && (
        <span className="text-[10.5px] font-semibold" style={{ color: 'var(--warn)' }}>
          一条消息最多 {maxPerPost} 个附件
        </span>
      )}
    </div>
  )
}

/** 还有没有在传的。发送按钮要等它们落地 —— 否则提交的是一串还不存在的 id。 */
export function hasUploading(items: Pending[]): boolean {
  return items.some((p) => !p.attachment && !p.error)
}

/** 提交给后端的那串 id。传失败的自然不在里面。 */
export function readyIds(items: Pending[]): string[] {
  return items.flatMap((p) => (p.attachment ? [p.attachment.id] : []))
}
