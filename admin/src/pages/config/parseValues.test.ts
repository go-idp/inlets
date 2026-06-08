import { describe, expect, it } from 'vitest'
import { valuesFromYAML } from './parseValues'

describe('valuesFromYAML', () => {
  it('parses camelCase keys matching schema paths', () => {
    const v = valuesFromYAML(`
domain: example.com
port: 8080
tcpPort: 8443
admin:
  enabled: true
  listen: 0.0.0.0:9090
clients:
  - clientId: c1
    clientSecret: s1
`)
    expect(v.domain).toBe('example.com')
    expect(v.port).toBe(8080)
    expect(v.tcpPort).toBe(8443)
    expect((v.admin as any).listen).toBe('0.0.0.0:9090')
    expect((v.clients as any[])[0].clientId).toBe('c1')
  })

  it('returns empty object for blank input', () => {
    expect(valuesFromYAML('')).toEqual({})
    expect(valuesFromYAML('   ')).toEqual({})
  })
})
