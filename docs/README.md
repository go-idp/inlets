# Inlets documentation site

This directory is a self-contained **VitePress** project (TypeScript config, **pnpm** lockfile). All site assets, Markdown, and Node tooling for the docs live here—mirroring the layout used by [go-zoox/ingress](https://github.com/go-zoox/ingress).

## Prerequisites

- [Node.js](https://nodejs.org/) **18.12+** (VitePress 1.x / Vite 5 do not support Node 16)
- [pnpm](https://pnpm.io/) 9.x (`packageManager` is pinned in `package.json`)

Mermaid diagrams use **`vitepress-plugin-mermaid`** (see `.vitepress/config.ts`). If diagrams fail to load under pnpm, try `pnpm install --shamefully-hoist` as [recommended by the plugin](https://www.npmjs.com/package/vitepress-plugin-mermaid).

## Commands

```bash
cd docs
pnpm install
pnpm dev          # local dev server
pnpm build        # static output to .vitepress/dist
pnpm preview      # serve production build locally
pnpm typecheck    # TypeScript (config + scripts)
```

## Deploying

- The default `base` in `.vitepress/config.ts` is `/`. If you publish to GitHub Pages under a repository subpath (for example `https://user.github.io/inlets/`), set `base: '/inlets/'` or inject it via your CI step.
- Build artifact: `docs/.vitepress/dist`.

## Layout

Sidebar structure is inspired by [nps docs](https://ehang-io.github.io/nps/) (introduction → server → client → reference → deep dives → community).

| Path | Purpose |
| --- | --- |
| `.vitepress/config.ts` | Theme, nav, **single sidebar per locale** (`/` and `/zh/`), search, `locales`. |
| `guide/` | Introduction, install, quick start, examples (English). |
| `server/` | Server overview, run, configuration, advanced (English). |
| `client/` | Client overview, HTTP/TCP, auth, advanced (English). |
| `reference/` | Architecture, full CLI tables (English). |
| `community/` | FAQ, changelog pointers (English). |
| `zh/` | Mirror of the same tree for 简体中文. |
| `features/` | Long-form technical notes (EN routes; `NEW_PROTOCOL_ISSUES` also copied under `zh/features/`). |
| `public/` | Static assets (`/logo.svg`, etc.). |
| `scripts/` | Maintainer TypeScript (`tsconfig` includes this tree). |

## Languages

- Default locale: **English** (`/`).
- **简体中文**: [`/zh/`](./zh/index.md) — use the language menu in the nav bar (VitePress adds it when multiple `locales` define `link`).
- After editing one language, update the other if the page is meant to stay in sync (feature docs may differ: e.g. `NEW_PROTOCOL_ISSUES` is maintained primarily under `zh/features/` as a copy of the root file for routing).
