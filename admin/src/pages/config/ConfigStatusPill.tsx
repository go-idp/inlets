type Props = {
  status: 'ok' | 'warn' | 'err'
  errorCount: number
}

export function ConfigStatusPill({ status, errorCount }: Props) {
  // 仅在有问题时展示；校验通过用 topOk 临时提示，避免常驻绿条占空间
  if (status === 'ok') return null

  return (
    <div className={`config-status ${status}`}>
      {status === 'warn' && '配置尚未加载或为空'}
      {status === 'err' && `存在 ${errorCount} 个校验问题`}
    </div>
  )
}
