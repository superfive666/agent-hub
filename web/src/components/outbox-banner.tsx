import { useHealth } from '@/api/queries'
import { OutboxAlert } from '@/components/outbox-alert'

/**
 * §1.4：worker 挂掉是完全静默的失败，`outbox_lag` 是唯一能发现它的地方。
 * 所以这条横幅挂在**每一个**页面的主玻璃板顶部，不随页面走、不折叠、不降级。
 *
 * health 查询本身失败时也要出声：那说明我们连滞后多少都不知道，
 * 静默掉等于把唯一的探针也关了。
 */
export function OutboxBanner() {
  const { data, isError } = useHealth()
  if (isError) {
    return (
      <div className="px-5 pt-4 sm:px-6">
        <OutboxAlert lagSeconds={Number.NaN} workerAlive={false} />
      </div>
    )
  }
  if (!data) return null
  const lag = data.outboxLagSeconds ?? 0
  const alive = data.workerAlive ?? false
  if (alive && lag < 60) return null
  return (
    <div className="px-5 pt-4 sm:px-6">
      <OutboxAlert lagSeconds={lag} workerAlive={alive} pending={data.outboxPending} />
    </div>
  )
}
