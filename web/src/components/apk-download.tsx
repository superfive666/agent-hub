import { Download, Smartphone } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { apiUrl } from '@/api/client'
import { useApkMeta } from '@/api/queries'
import { cn } from '@/lib/cn'

/**
 * APK 的下载地址。**写成 `/download` 而不是 `/api/download`** —— 这是对外的
 * 正式地址，用户会把它抄给别人、直接敲进手机浏览器。
 *
 * hub 上两条路径是同一个处理器（见 ADR-0010），`/api/download` 只是留给
 * 还没改过反向代理的部署的同义词，不该出现在给人看的界面上。
 */
const DOWNLOAD_PATH = '/download'

function sizeLabel(bytes: number | undefined): string {
  if (!bytes) return ''
  const mb = bytes / (1024 * 1024)
  return mb >= 1 ? `${mb.toFixed(1)} MB` : `${Math.max(1, Math.round(bytes / 1024))} KB`
}

/**
 * Android 客户端的下载入口。
 *
 * **登录页和设置页共用同一个组件**，因为它在两个地方是同一件事：
 * 「这台 hub 上有个 app，拿去装」。摆两份实现的话，版本号的展示方式、
 * 没包时的措辞、下载地址迟早会漂开。
 *
 * ## 为什么它必须出现在登录页上
 *
 * 装 app 的那一刻用户手上还没有会话，而他很可能**正是想在手机上登录才来装的**。
 * 入口只放在登录后的页面里，就成了「要先登录才能拿到用来登录的东西」。
 * 端点本身也是公开的（ADR-0010），两边是一致的。
 *
 * ## 为什么是 `<a>` 而不是 fetch
 *
 * 这条路要让**浏览器自己去下**：十几 MB 的文件、要走进度条、要能断点续传、
 * 手机上下完还要交给系统安装器。fetch 下来变成 blob 再塞回一个 a[download]，
 * 会先在内存里攒完整个文件，手机上很容易直接被系统杀掉。
 */
export function ApkDownload({
  variant = 'card',
  className,
}: {
  /** `card` 给设置页，`inline` 给登录页那种没有卡片壳的地方 */
  variant?: 'card' | 'inline'
  className?: string
}) {
  const { data, isPending } = useApkMeta()

  // 还在问的时候什么都不画。先画一个"没有"再跳成"有"，比晚半秒出现更糟 ——
  // 用户刚要点就消失了。
  if (isPending) return null

  const available = data?.available === true
  const meta = [data?.version && `v${data.version}`, sizeLabel(data?.sizeBytes)]
    .filter(Boolean)
    .join(' · ')

  const body = (
    <>
      <div className="flex items-center gap-2.5">
        <span
          className="flex size-9 shrink-0 items-center justify-center rounded-pill"
          style={{
            background: 'var(--inset-bg)',
            border: '1px solid var(--inset-bd)',
            color: available ? 'var(--agent-ink)' : 'var(--ink3)',
          }}
        >
          <Smartphone size={16} aria-hidden />
        </span>
        <div className="min-w-0">
          <div className="text-[12.5px] font-bold leading-none">Android 客户端</div>
          <div
            className="mt-1.5 truncate text-[11px] font-medium leading-[1.5]"
            style={{ color: 'var(--ink3)' }}
          >
            {available
              ? meta || '和控制台同一个账号，装完填 hub 地址即可'
              : '这台 hub 还没有发布安装包'}
          </div>
        </div>
      </div>

      {available ? (
        <Button
          variant="pri"
          asChild
          className={cn('shrink-0 gap-2 px-5 py-[11px] text-[12.5px]', variant === 'card' && 'ml-auto')}
        >
          {/* download 属性只是给个文件名建议；真正定名的是后端的 Content-Disposition */}
          <a data-testid="apk-download" href={apiUrl(DOWNLOAD_PATH)} download>
            <Download size={15} aria-hidden />
            <span>下载 APK</span>
          </a>
        </Button>
      ) : (
        // 没包时给禁用按钮加一句说明，而不是让人点下去拿到一段 JSON 错误
        <Button
          variant="gh"
          disabled
          data-testid="apk-unavailable"
          className={cn('shrink-0 px-5 py-[11px] text-[12.5px]', variant === 'card' && 'ml-auto')}
          title="管理员需要配置 ANDROID_APK_PATH 并放上构建产物"
        >
          暂未发布
        </Button>
      )}
    </>
  )

  if (variant === 'inline') {
    return (
      <div
        className={cn('flex flex-wrap items-center justify-between gap-3', className)}
        data-testid="apk-entry"
      >
        {body}
      </div>
    )
  }

  return (
    <div className={cn('card', className)} data-testid="apk-entry">
      <div className="card-bd flex flex-wrap items-center gap-3 p-[18px_20px]">{body}</div>
      <div
        className="px-5 pb-[18px] text-[11px] font-medium leading-[1.7]"
        style={{ color: 'var(--ink3)' }}
      >
        自建分发，不走应用商店 —— 安装时手机会要求允许「安装未知来源的应用」。
        下载地址是 <code className="mono">{DOWNLOAD_PATH}</code>，公开，不需要登录，
        可以直接发给别人。
      </div>
    </div>
  )
}
