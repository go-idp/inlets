import type { ReactNode } from 'react'
import { X } from 'lucide-react'

type Props = {
  open: boolean
  title: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  nested?: boolean
}

export function Drawer({ open, title, onClose, children, footer, nested }: Props) {
  if (!open) return null

  const layer = nested ? ' drawer-nested' : ''

  return (
    <>
      <div className={`drawer-backdrop${layer}`} onClick={onClose} role="presentation" />
      <aside className={`drawer${layer}`} role="dialog" aria-modal="true" aria-labelledby="drawer-title">
        <div className="drawer-head">
          <h2 id="drawer-title">{title}</h2>
          <button type="button" className="drawer-close" onClick={onClose} aria-label="关闭">
            <X size={18} strokeWidth={1.75} />
          </button>
        </div>
        <div className="drawer-body">{children}</div>
        {footer ? <div className="drawer-foot">{footer}</div> : null}
      </aside>
    </>
  )
}
