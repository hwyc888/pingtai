# Pingtai frontend

This directory contains the editable Vue 3 application source. The current application has two routes:

- `/login`: username/password login and token refresh.
- `/IntelligentAI`: model/agent selection, chat, attachments, reasoning display and account actions.

## Development

Requirements: Node.js 20.19 or newer and npm.

```sh
npm install
npm run dev
```

The Vite development server listens on port 5173 and proxies `/api` to `http://127.0.0.1:5050`. Set `PINGTAI_API_PROXY` if the Go server listens elsewhere.

## Production build

```sh
npm run build
```

The production files are written to `dist/`. The root `build.ps1` copies this output into `internal/httpapi/web/` before building the Go executables.

The chat API currently does not expose a conversation-list endpoint, so the history drawer is an empty state. Knowledge-base and web-search controls are retained as UI toggles; the current `/agents/chat` request contract does not accept separate flags for them.
