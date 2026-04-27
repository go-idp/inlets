// Maintainer helper for future doc tooling (included for pnpm typecheck coverage).
export function docsNodeMajor(): number {
  const major = Number(process.versions.node.split('.')[0])
  if (!Number.isFinite(major)) {
    throw new Error('Unable to parse process.versions.node')
  }
  return major
}
