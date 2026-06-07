import { DiffViewer } from '../../components/DiffViewer'

type Props = {
  open: boolean
  saving: boolean
  diff: string
  summary: string
  onSummaryChange: (s: string) => void
  onCancel: () => void
  onConfirm: () => void
}

export function ConfigSaveDialog({
  open, saving, diff, summary, onSummaryChange, onCancel, onConfirm,
}: Props) {
  if (!open) return null
  return (
    <div className="modal-backdrop" onClick={() => !saving && onCancel()}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">保存配置</div>
        <div className="modal-body">
          <div className="config-field">
            <label htmlFor="save-summary">变更说明（可选）</label>
            <input
              id="save-summary"
              type="text"
              value={summary}
              onChange={(e) => onSummaryChange(e.target.value)}
              placeholder="例：调整 client A 的带宽上限"
            />
          </div>
          <h4 style={{ fontSize: 12, marginBottom: 4, color: 'var(--text-muted)' }}>变更预览</h4>
          <DiffViewer raw={diff} />
        </div>
        <div className="modal-foot">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onCancel}
            disabled={saving}
          >
            取消
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={onConfirm}
            disabled={saving}
          >
            {saving ? '保存中…' : '确认保存'}
          </button>
        </div>
      </div>
    </div>
  )
}
