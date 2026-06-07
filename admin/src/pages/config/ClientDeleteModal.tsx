type Props = {
  open: boolean
  clientId: string
  onCancel: () => void
  onConfirm: () => void
}

export function ClientDeleteModal({ open, clientId, onCancel, onConfirm }: Props) {
  if (!open) return null

  return (
    <div className="modal-backdrop" onClick={onCancel}>
      <div className="modal modal-sm" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">删除客户端</div>
        <div className="modal-body">
          <p style={{ fontSize: 13, lineHeight: 1.6 }}>
            确定删除客户端 <code className="inline">{clientId || '(未命名)'}</code> 吗？
            此操作仅更新当前草稿，需点击「保存并重载」后才会写入配置文件。
          </p>
        </div>
        <div className="modal-foot">
          <button type="button" className="btn btn-secondary" onClick={onCancel}>
            取消
          </button>
          <button type="button" className="btn btn-danger" onClick={onConfirm}>
            确认删除
          </button>
        </div>
      </div>
    </div>
  )
}
