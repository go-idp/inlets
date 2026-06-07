type Props = {
  count: number
}

export function OverrideBanner({ count }: Props) {
  if (count <= 0) return null
  return (
    <div
      className="alert"
      style={{
        background: 'rgba(232, 184, 74, 0.10)',
        border: '1px solid rgba(232, 184, 74, 0.35)',
        color: 'var(--warn)',
      }}
    >
      {count} 项临时覆盖生效中，进程重启后失效。
    </div>
  )
}
