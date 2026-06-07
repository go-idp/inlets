import { describe, it, expect } from 'vitest'
import YAML from 'yaml'
import { serializeValuesToYAML } from './serialize'

describe('serializeValuesToYAML', () => {
  it('preserves simple scalar values', () => {
    const src = 'domain: example.com\nport: 80\n'
    const parsed = YAML.parse(src)
    const out = YAML.parse(serializeValuesToYAML(parsed, src))
    expect(out.domain).toBe('example.com')
    expect(out.port).toBe(80)
  })

  it('preserves nested objects', () => {
    const src = `
domain: example.com
clients:
  - clientId: a
    clientSecret: b
    tunnels:
      - type: http
        upstream: 127.0.0.1:80
`
    const parsed = YAML.parse(src)
    const out = YAML.parse(serializeValuesToYAML(parsed, src))
    expect(out.clients[0].clientId).toBe('a')
    expect(out.clients[0].tunnels[0].type).toBe('http')
  })

  it('roundtrips a mutated value back to the same field', () => {
    const src = `
domain: a.example.com
port: 80
clients:
  - clientId: a
    clientSecret: b
`
    const parsed = YAML.parse(src)
    parsed.port = 8080
    const out = serializeValuesToYAML(parsed, src)
    const reParsed = YAML.parse(out)
    expect(reParsed.port).toBe(8080)
    expect(reParsed.domain).toBe('a.example.com')
  })

  it('preserves the order of fields from source', () => {
    const src = `domain: a\nport: 80\ntcpPort: 81\n`
    const parsed = YAML.parse(src)
    const out = serializeValuesToYAML({ ...parsed, tcpPort: 82 }, src)
    // domain should still come before port which should still come before tcpPort
    const dIdx = out.indexOf('domain:')
    const pIdx = out.indexOf('port:')
    const tIdx = out.indexOf('tcpPort:')
    expect(dIdx).toBeLessThan(pIdx)
    expect(pIdx).toBeLessThan(tIdx)
  })

  it('passes through unknown fields (pass-through strategy)', () => {
    const src = `domain: a\nfutureField: keep-me\n`
    const parsed = YAML.parse(src)
    const out = YAML.parse(serializeValuesToYAML(parsed, src))
    expect(out.futureField).toBe('keep-me')
  })
})
