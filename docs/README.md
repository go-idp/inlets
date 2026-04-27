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
pnpm dev          # local dev server (open http://localhost:5173/inlets/ — base matches GitHub Pages)
pnpm build        # static output to .vitepress/dist
pnpm preview      # serve production build locally
pnpm typecheck    # TypeScript (config + scripts)
```

## Deploying

- Build artifact: `docs/.vitepress/dist`.
- **GitHub Actions → GitHub Pages** (same approach as [go-zoox/ingress](https://github.com/go-zoox/ingress/blob/master/.github/workflows/docs.yml)): workflow `.github/workflows/docs.yml` runs on pushes to `master` / `main` when `docs/**` or the workflow file changes, or on manual `workflow_dispatch`. It uses `pnpm` + `pnpm run build`, then `actions/upload-pages-artifact` and `actions/deploy-pages`.
  1. In the GitHub repo: **Settings → Pages → Build and deployment**, set **Source** to **GitHub Actions** (not “Deploy from a branch”).
  2. The first run may require approving the `github-pages` environment if your org restricts deployments.
  3. This repo sets **`base: '/inlets/'`** in `.vitepress/config.ts` for **https://go-idp.github.io/inlets/**. If you fork to a **user/org root** site (`https://<user>.github.io/` with no repo prefix), change `siteBase` back to `'/'`.

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
