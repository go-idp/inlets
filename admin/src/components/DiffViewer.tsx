interface DiffLine {
  kind: ' ' | '+' | '-'
  text: string
}

export function parseDiff(raw: string): DiffLine[] {
  if (!raw) return []
  return raw.split('\n').filter((l) => l.length > 0).map((line) => {
    const c = line[0]
    if (c === '+' || c === '-' || c === ' ') {
      return { kind: c as '+' | '-' | ' ', text: line.slice(1) }
    }
    return { kind: ' ' as const, text: line }
  })
}

export function DiffViewer({ raw }: { raw: string }) {
  const lines = parseDiff(raw)
  if (lines.length === 0) {
    return <div className="diff-empty">无差异</div>
  }
  return (
    <div className="diff-viewer">
      {lines.map((l, i) => (
        <div key={i} className={`diff-line diff-${l.kind === ' ' ? 'keep' : l.kind === '+' ? 'add' : 'del'}`}>
          <span className="diff-marker">{l.kind === ' ' ? ' ' : l.kind}</span>
          <span className="diff-text">{l.text || '\u00A0'}</span>
        </div>
      ))}
    </div>
  )
}

/** Build a unified diff in the same format as the server's UnifiedDiff. */
export function computeDiff(oldText: string, newText: string): string {
  const a = oldText.split('\n')
  const b = newText.split('\n')
  const edits = lcsDiff(a, b)
  return edits.map((e) => `${e.kind}${e.text}`).join('\n')
}

interface Edit { kind: ' ' | '+' | '-'; text: string }

function lcsDiff(a: string[], b: string[]): Edit[] {
  const n = a.length, m = b.length
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0))
  for (let i = 1; i <= n; i++) {
    for (let j = 1; j <= m; j++) {
      if (a[i - 1] === b[j - 1]) dp[i][j] = dp[i - 1][j - 1] + 1
      else dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1])
    }
  }
  const out: Edit[] = []
  let i = n, j = m
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && a[i - 1] === b[j - 1]) {
      out.push({ kind: ' ', text: a[i - 1] })
      i--; j--
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      out.push({ kind: '+', text: b[j - 1] })
      j--
    } else {
      out.push({ kind: '-', text: a[i - 1] })
      i--
    }
  }
  return out.reverse()
}
