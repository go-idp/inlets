import { PageHeader } from '../components/PageHeader'
import { RevisionsPanel } from '../components/RevisionsPanel'
import { ConfigForm } from './config/ConfigForm'
import { ConfigSaveDialog } from './config/ConfigSaveDialog'
import { ConfigStatusPill } from './config/ConfigStatusPill'
import { ConfigTabs } from './config/ConfigTabs'
import { ConfigYamlEditor } from './config/ConfigYamlEditor'
import { useConfigState } from './config/useConfigState'

export function ConfigPage() {
  const { state, actions } = useConfigState()
  const {
    tab, yaml, path, schema, values, errors, topError, topOk, saving,
    showSaveDialog, pendingDiff, summary, errorByPath, status,
  } = state

  return (
    <>
      <PageHeader
        title="配置管理"
        subtitle={path || '—'}
        actions={
          <>
            <button type="button" className="btn" onClick={actions.onValidate} disabled={saving}>
              校验
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={actions.onSave}
              disabled={saving || status === 'err'}
            >
              {saving ? '保存中…' : '保存并重载'}
            </button>
          </>
        }
      />

      {topError ? <div className="alert alert-danger">{topError}</div> : null}
      {topOk ? <div className="alert alert-ok">{topOk}</div> : null}

      <ConfigStatusPill status={status} errorCount={errors.length} />

      <ConfigTabs tab={tab} onChange={actions.setTab} />

      {tab === 'visual' && schema ? (
        <ConfigForm
          schema={schema}
          values={values}
          onFieldChange={actions.onFieldChange}
          errorByPath={errorByPath}
        />
      ) : null}

      {tab === 'yaml' ? (
        <ConfigYamlEditor value={yaml} onChange={actions.onYAMLChange} />
      ) : null}

      {tab === 'revisions' ? (
        <RevisionsPanel currentYaml={state.rawYaml} onRestored={actions.onRestored} />
      ) : null}

      <ConfigSaveDialog
        open={showSaveDialog}
        saving={saving}
        diff={pendingDiff}
        summary={summary}
        onSummaryChange={actions.setSummary}
        onCancel={() => actions.setShowSaveDialog(false)}
        onConfirm={actions.onConfirmSave}
      />
    </>
  )
}
