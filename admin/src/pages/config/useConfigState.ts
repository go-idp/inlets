import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, type ConfigSchema, type FieldDef, type ValidationError } from '../../api/client'
import { computeDiff } from '../../components/DiffViewer'
import { getByPath, setByPath } from '../../schema/renderers'
import { serializeValuesToYAML } from '../../schema/serialize'
import { valuesFromYAML } from './parseValues'
import { collectSecrets, maskSecretsInValues, SECRET_PLACEHOLDER } from './secrets'

type Tab = 'visual' | 'yaml' | 'revisions'

export type Status = 'ok' | 'warn' | 'err'

export function useConfigState() {
  const [tab, setTab] = useState<Tab>('visual')
  const [yaml, setYaml] = useState('')
  const [rawYaml, setRawYaml] = useState('') // original raw (for ordering)
  const [path, setPath] = useState('')
  const [schema, setSchema] = useState<ConfigSchema | null>(null)
  const [values, setValues] = useState<any>({})
  const [errors, setErrors] = useState<ValidationError[]>([])
  const [topError, setTopError] = useState('')
  const [topOk, setTopOk] = useState('')
  const [saving, setSaving] = useState(false)
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [pendingYAML, setPendingYAML] = useState('')
  const [pendingDiff, setPendingDiff] = useState('')
  const [summary, setSummary] = useState('')
  const secretsRef = useRef<Record<string, string>>({})

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const [raw, sch] = await Promise.all([
          api.getConfigRaw(),
          api.getConfigSchema(),
        ])
        if (cancelled) return
        setRawYaml(raw.yaml)
        setYaml(raw.yaml)
        setPath(raw.path)
        setSchema(sch)
        // Structured GET /config uses Go JSON field names (Domain, TCPPort, …) which
        // do not match schema paths (domain, tcpPort, …). Parse raw YAML instead.
        const cfg = valuesFromYAML(raw.yaml)
        secretsRef.current = collectSecrets(cfg, sch)
        setValues(maskSecretsInValues(cfg, sch))
        const result = await api.validateConfig(raw.yaml)
        if (cancelled) return
        setErrors(result.errors ?? [])
      } catch (e) {
        if (cancelled) return
        setTopError(e instanceof Error ? e.message : 'failed to load config')
      }
    }
    load()
    return () => { cancelled = true }
  }, [])

  const errorByPath = useMemo(() => {
    const m: Record<string, ValidationError> = {}
    for (const e of errors) {
      if (e.path) m[e.path] = e
    }
    return m
  }, [errors])

  const onFieldChange = useCallback((field: FieldDef, v: unknown) => {
    setValues((prev: any) => {
      const next = setByPath(prev, field.path, v)
      if (field.kind === 'secret') {
        if (typeof v === 'string' && v !== '' && v !== SECRET_PLACEHOLDER) {
          secretsRef.current[field.path] = v
        } else if (v === '' || v == null) {
          delete secretsRef.current[field.path]
        }
      }
      return next
    })
  }, [])

  const onYAMLChange = useCallback(async (newYAML: string) => {
    setYaml(newYAML)
    setTopError('')
    setTopOk('')
    try {
      const result = await api.validateConfig(newYAML)
      setErrors(result.errors ?? [])
    } catch (e) {
      setTopError(e instanceof Error ? e.message : 'validate failed')
    }
  }, [])

  const buildYAMLFromValues = useCallback((): string => {
    let merged = values
    for (const [pathStr, secretValue] of Object.entries(secretsRef.current)) {
      const current = getByPath(merged, pathStr)
      if (current === SECRET_PLACEHOLDER || current == null) {
        merged = setByPath(merged, pathStr, secretValue)
      }
    }
    return serializeValuesToYAML(merged, rawYaml)
  }, [values, rawYaml])

  const onSave = useCallback(async () => {
    setTopError('')
    setTopOk('')
    try {
      const out = tab === 'yaml' ? yaml : buildYAMLFromValues()
      const result = await api.validateConfig(out)
      if (!result.ok) {
        setErrors(result.errors ?? [])
        setTopError(`校验失败：${result.errors.length} 个问题`)
        return
      }
      setPendingYAML(out)
      setPendingDiff(computeDiff(rawYaml, out))
      setShowSaveDialog(true)
    } catch (e) {
      setTopError(e instanceof Error ? e.message : 'save failed')
    }
  }, [tab, yaml, buildYAMLFromValues, rawYaml])

  const setTabWithSync = useCallback((next: Tab) => {
    if (next === 'visual' && tab === 'yaml' && yaml.trim()) {
      try {
        const parsed = valuesFromYAML(yaml)
        secretsRef.current = collectSecrets(parsed, schema)
        setValues(maskSecretsInValues(parsed, schema))
      } catch {
        /* keep existing values when YAML tab has parse errors */
      }
    }
    setTab(next)
  }, [tab, yaml, schema])

  const refreshFromServer = useCallback(async () => {
    const raw = await api.getConfigRaw()
    setRawYaml(raw.yaml)
    setYaml(raw.yaml)
    const cfg = valuesFromYAML(raw.yaml)
    secretsRef.current = collectSecrets(cfg, schema)
    setValues(maskSecretsInValues(cfg, schema))
    const result = await api.validateConfig(raw.yaml)
    setErrors(result.errors ?? [])
  }, [schema])

  const onConfirmSave = useCallback(async () => {
    setSaving(true)
    setTopError('')
    setTopOk('')
    try {
      await api.putConfig(pendingYAML, summary)
      setShowSaveDialog(false)
      setSummary('')
      setTopOk('配置已保存并重载')
      await refreshFromServer()
    } catch (e) {
      setTopError(e instanceof Error ? e.message : 'save failed')
    } finally {
      setSaving(false)
    }
  }, [pendingYAML, summary, refreshFromServer])

  const onRestored = useCallback(async () => {
    setTopOk('已恢复到历史版本')
    await refreshFromServer()
  }, [refreshFromServer])

  const onValidate = useCallback(async () => {
    setTopError('')
    setTopOk('')
    try {
      const out = tab === 'yaml' ? yaml : buildYAMLFromValues()
      const result = await api.validateConfig(out)
      setErrors(result.errors ?? [])
      if (result.ok) setTopOk('校验通过')
      else setTopError(`校验失败：${result.errors.length} 个问题`)
    } catch (e) {
      setTopError(e instanceof Error ? e.message : 'validate failed')
    }
  }, [tab, yaml, buildYAMLFromValues])

  const status: Status = useMemo(() => {
    if (errors.length > 0) return 'err'
    if (!values || Object.keys(values).length === 0) return 'warn'
    return 'ok'
  }, [errors, values])

  return {
    state: {
      tab, yaml, rawYaml, path, schema, values, errors, topError, topOk, saving,
      showSaveDialog, pendingYAML, pendingDiff, summary, errorByPath, status,
    },
    actions: {
      setTab: setTabWithSync, setSummary, setShowSaveDialog,
      onFieldChange, onYAMLChange, onSave, onConfirmSave, onValidate, onRestored,
    },
  }
}
