import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { PageHeader } from '../components/PageHeader'

export function ConfigPage() {
  const [yaml, setYaml] = useState('')
  const [path, setPath] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api
      .getConfigRaw()
      .then((r) => {
        setYaml(r.yaml)
        setPath(r.path)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  const onSave = async () => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      await api.putConfig(yaml)
      setMessage('配置已保存并重载')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'save failed')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <PageHeader
        title="配置管理"
        subtitle={path || '—'}
        actions={
          <button type="button" className="btn btn-primary" disabled={saving} onClick={onSave}>
            {saving ? '保存中…' : '保存并重载'}
          </button>
        }
      />
      {error ? <div className="alert alert-danger">{error}</div> : null}
      {message ? <div className="alert alert-ok">{message}</div> : null}
      <div className="panel">
        <textarea
          className="yaml-editor"
          value={yaml}
          onChange={(e) => setYaml(e.target.value)}
          spellCheck={false}
        />
      </div>
    </>
  )
}
