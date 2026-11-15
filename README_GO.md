# 超凡AI问答平台 · Go edition

## Release package

The editable frontend source is a Vue 3 application in frontend/. The built HTML, JavaScript, CSS and images are embedded into both Go executables with the Go embed feature. The release does not need Node.js or a separate web server at runtime. Building from source requires Node.js 20.19 or newer and npm.

On Windows, run `build.ps1`. On Ubuntu/Linux, run `./build-linux.sh`. Both scripts build the Vue frontend, embed it in the Go executable, run Go tests and vet, and create a Linux release package. The Windows build also creates the Windows executable.

1. Installs frontend dependencies and runs the Vite production build in frontend/.
2. Copies frontend/dist/ into the Go embed source at internal/httpapi/web/.
3. Runs gofmt, go test ./... and go vet ./....
4. Builds the Windows AMD64 and Linux AMD64 executables.
5. Packages runtime configuration, startup scripts and deployment files.
6. Creates dist/pingtai-release.zip.

The ZIP contains:

- pingtai.exe: Windows AMD64 server, frontend embedded.
- pingtai-linux-amd64: Linux AMD64 server, frontend embedded.
- .env.example: safe configuration template, no secrets.
- start.bat: Windows launcher that opens the browser for web-based setup.
- manage-go.sh: Linux launcher.
- README.md: deployment instructions.
- deploy/: systemd and Nginx examples.

The ZIP intentionally excludes any local .env file and the original frontend folder. The HTML, JS, CSS, images and other static files are inside the executable.

## Windows deployment

1. Extract pingtai-release.zip to a writable directory.
2. Double-click start.bat. The platform starts and opens the normal login page.
3. On the first visit, create the first administrator account. The first account in a new SQLite database is automatically assigned the administrator role.
4. Sign in, open **平台管理**, add an OpenAI-compatible provider, enter its Base URL and API Key, discover models, select a default model, and activate the provider.
5. Restart the service after activating or changing the active provider. Then start a new conversation and begin using the platform.

There is no separate setup wizard, setup token, or first-run configuration page. SQLite is created automatically. The administrator manages providers, model choices and local user accounts inside the platform. The listening address and database path can be supplied through environment variables when needed; no MySQL server is required.

The console window is for service logs and shutdown. The EXE serves the embedded frontend directly; no frontend/ folder or separate web server is required.

## Linux deployment

To build from source on Ubuntu 22.04/24.04, install Go 1.26+, Node.js 20.19+ and npm, then run `chmod +x build-linux.sh && ./build-linux.sh`. The script produces `dist/pingtai-linux-amd64` and, when `zip` is installed, `dist/pingtai-linux-amd64-release.zip`.

Extract the ZIP on a Linux AMD64 host, then run:

       chmod +x pingtai-linux-amd64 manage-go.sh
       ./manage-go.sh start

Open the platform address printed in the service log (default `http://127.0.0.1:5050`). On the first visit, create the first administrator account. No setup key, configuration link, or service restart is needed for initial account creation. After signing in, configure the model provider in **平台管理**. Restart the service after activating or changing the active model provider.

For systemd, use deploy/pingtai.service. The example uses systemd `StateDirectory=pingtai` to create `/var/lib/pingtai` with ownership for the service user; the SQLite database is stored at `/var/lib/pingtai/pingtai.db`. Use `systemctl restart pingtai` after activating or changing the active model provider.

## Platform initialization and data

- The first visit checks whether the SQLite database contains any account. If it is empty, the login page displays the first-administrator registration form.
- The first account is created atomically as an administrator. Once an account exists, public first-admin registration is permanently closed for that database.
- A random JWT signing secret is generated automatically and stored in the SQLite `ai_platform_setting` table. No setup key or JWT secret needs to be copied into a web form.
- Conversation history, users, provider credentials and platform settings are stored in SQLite. Provider API keys are never returned to the browser after saving.
- The default database path is `data/pingtai.db` next to the executable. Set `PINGTAI_DB_PATH` to a writable persistent path for server deployments.
- Protect the SQLite database and its backups; they contain account hashes, provider API keys and the platform signing secret.
- Back up the database before replacing the executable. Database migrations are additive and do not delete conversation records.

## AI provider management

After the first administrator signs in, open `/admin` to manage AI providers. Add one or more OpenAI-compatible providers with a display name, Base URL and API key. Save a provider, then use **Discover models** to request its `/models` endpoint. The platform stores the returned model IDs and recognizes common optional context metadata fields such as `context_length`, `context_window`, `max_model_len` and `max_input_tokens`.

Some OpenAI-compatible services return only model IDs. In that case, context length and output limits remain unknown until an administrator enters them manually. The platform does not invent missing context values. Context length is stored as provider metadata; automatic token counting and context-window truncation are separate runtime work and are not implied by a model-list response.

Choose an enabled default model and activate the provider. The active provider is loaded from SQLite when the Go process starts, so restart the service after switching providers. Chat clients use this platform default model automatically; they do not see or select provider/model credentials.

Provider API keys are stored in the SQLite database file. The database is created with owner-only file permissions (0600) and the systemd example keeps it under `/var/lib/pingtai`, owned by the service account. Protect database backups as secrets.

### Context-window budgeting

When the active default model has a context length recorded in provider model metadata, built-in web chat uses a conservative token estimate to fit the system prompt, recent history and current message into that window. It reserves output capacity and a safety margin, then keeps the newest contiguous set of complete user/assistant turns. It never splits a turn or drops the current question. If the current message and system prompt alone exceed the available input budget, the request is rejected with HTTP 413 and the user is asked to shorten the content or remove an attachment.

The estimator is intentionally model-agnostic: ASCII text is budgeted at roughly one token per three characters, CJK text at 1.5 tokens per character, and each image at a fixed 4,096-token reserve. This is a safety estimate, not the provider's exact tokenizer; actual image costs and tokenization vary by model. If the provider does not publish a context length, the platform does not invent one and continues to apply only the configured `CONTEXT_TURNS` limit. The configured output limit is reduced when necessary to fit the remaining known context window.

### Provider diagnostics

The administrator page offers two explicit checks. **Test connection** calls the provider's `/models` endpoint and reports HTTP status, latency and model count without saving changes. **Test default model** sends a short Chat Completions request capped at 8 output tokens; the confirmation dialog warns that the upstream may charge for this request. Neither check returns or logs the provider API key.

## Configuration

SQLite is the only runtime database. The server creates `data/pingtai.db` automatically (or the configured `PINGTAI_DB_PATH`) and applies non-destructive `CREATE TABLE IF NOT EXISTS` migrations. SQLite runs in WAL mode with a busy timeout; the database file and its directory must be writable by the service account. Stop the service before taking a simple file copy backup so the WAL is checkpointed.

### Generic OpenAI-compatible service (recommended)

Open /admin and add an OpenAI-compatible provider. Enter the upstream Base URL and API key, save it, then discover models. Select the enabled models and choose a default model. Activate the provider and restart the service. The first-run OpenAI-compatible environment settings are imported into this same provider list when available.

Save the supplier details first, then use **Discover models**. Discovery uses the saved Base URL and API key, requires an authenticated administrator, has a 20-second timeout and accepts up to 4 MB of model-list JSON. If the upstream does not support /models or denies access, the page reports the failure without deleting the supplier configuration. The model list supports enable/disable and manual context/output-token metadata. Returned models may include non-chat models; only enable models that support Chat Completions.

Enabled models and the default model are stored with the provider in SQLite. After restart they appear in the built-in chat model selector, with the default selected first. Selecting another model routes the next message to that model while retaining the conversation history. If the chosen model does not support the existing image/context format, choose a compatible model. An empty model list leaves chat unavailable until an administrator configures a provider.

This selection is for built-in web chat, not an API access-control allowlist. Standard /v1/models continues to return the upstream listing and API callers can provide any model supported by that upstream. The first selected model is used as default if no explicit default is set; otherwise the explicit default must be one of the selected models.

Supported address forms include a bare host (adds /v1), an API prefix such as https://provider.example/v1 or https://provider.example/compatible-mode/v1, and a full https://provider.example/api/v3/chat/completions URL. A trailing slash does not duplicate the path. Query parameters on the configured URL are preserved. Local services without authentication may leave the upstream API key blank.

Equivalent environment values for deployments without a saved web configuration:

       CHAT_PROVIDER=openai
       OPENAI_BASE_URL=https://provider.example/v1
       OPENAI_API_KEY=your-upstream-api-key
       OPENAI_MODEL=your-model-id
       OPENAI_MODELS=your-model-id,another-model-id
       OPENAI_TIMEOUT_SECONDS=180
       OPENAI_MAX_TOKENS=0
       OPENAI_SYSTEM_PROMPT=
       CONTEXT_TURNS=20
       MODEL_API_KEY=your-platform-client-key

The upstream API key is used only between this server and the provider. The separate MODEL_API_KEY is what external clients use to call this platform. Set it in the service environment before connecting an OpenAI client.

OPENAI_MAX_TOKENS and OPENAI_SYSTEM_PROMPT apply only to this platform's web chat. A zero output limit leaves the decision to the upstream (recommended for reasoning models and providers with different token-limit parameters). Standard API requests retain the caller's own generation parameters and system/developer messages.

This is Chat Completions protocol compatibility, not a universal protocol converter. Providers must expose an OpenAI-compatible endpoint. It is suitable for compatible endpoints from OpenAI, DeepSeek, Qwen/DashScope, SiliconFlow, model gateways and local vLLM/Ollama/LM Studio deployments, subject to their actual API support. Native Anthropic Messages, Gemini generateContent, Azure api-key authentication, OpenAI Responses, embeddings, audio and image-generation endpoints are not implemented here. Images, tools, JSON schema and other capabilities still require support from the chosen model/provider. No automatic fallback, parameter guessing or retry is performed, to avoid duplicate charges or silently changing request semantics.

### Legacy CTYun service

For Xirang/OpenAI-compatible Bearer authentication, set the exact endpoint, API key and model ID from the CTYun console:

       CHAT_PROVIDER=ctyun
       CTYUN_AUTH_MODE=bearer
       CTYUN_API_URL=https://your-ctyun-endpoint/v1/chat/completions
       CTYUN_API_KEY=your-app-key
       CTYUN_MODEL=your-model-id

For a Star MaaS service using HMAC-SHA256 signing, set CTYUN_AUTH_MODE=signature and provide CTYUN_APP_ID, CTYUN_APP_KEY, CTYUN_REGION and any order/knowledge-base values required by that API.

The CTYun login page is not an API credential. Obtain the endpoint and credentials from the corresponding model service in the CTYun console.

## API

- POST /v1/chat/completions: OpenAI-compatible chat proxy, including SSE streaming.
- GET /v1/models: upstream model listing. If the upstream returns 404/405 and a default model is configured, returns a list containing that model instead. Authentication/rate-limit failures are not hidden by this fallback.
- /api/v1/chat/completions and /api/v1/models are equivalent aliases; /model/chat/completions is retained.
- GET /api/health: process and database health.
- Local authentication, conversation history and provider management are available under /auth, /agents and /admin. Knowledge-base and external-agent management are not exposed in this release.

Use `Authorization: Bearer <MODEL_API_KEY>` (or X-API-Key). A valid platform access token is also accepted. Invalid credentials return HTTP 401 with an OpenAI-style error object. Invalid OpenAI request JSON/messages return HTTP 400; upstream status, JSON response, SSE data and [DONE] markers pass through without vendor-specific wrapping. Retry-After is preserved for rate-limit responses. The maximum proxy request body is 10 MB.

For the official Python SDK (install `openai` in the calling application):

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:5050/v1",
    api_key="your-platform-client-key",
)
messages = [{"role": "user", "content": "Remember that my name is Alice."}]
first = client.chat.completions.create(model="your-model-id", messages=messages)
messages.append(first.choices[0].message.model_dump(exclude_none=True))
messages.append({"role": "user", "content": "What is my name?"})

for chunk in client.chat.completions.create(
    model="your-model-id", messages=messages, stream=True
):
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

### Context and multi-turn conversations

- Standard /v1/chat/completions is stateless, as in OpenAI. The client sends the complete `messages` history each time. The platform does not invent a conversation ID, store these API messages, inject prompts or truncate them. System/developer/user/assistant/tool messages, tool_calls/tool_call_id, multimodal content arrays, tools, tool_choice, response_format, stream_options and provider extension parameters are preserved. Tool execution is the calling application's responsibility.
- The built-in web chat uses authenticated `/agents` routes. Its conversation and messages are stored in SQLite and restored after service restart. Only successful completed turns are added to future model context; failed/in-progress requests and their error messages are excluded. Each conversation is scoped to its owning user.
- CONTEXT_TURNS controls the number of complete previous web-chat turns (default 20). `0` sends all successful stored turns; 1-1000 sends only the most recent turns. This is a turn limit, not a tokenizer or an unlimited model context window. If input exceeds the upstream token window, reduce the history setting or start a new conversation. Standard API requests are not affected by this setting.
- Web image attachments retain their structured image_url content in the new ai_message_context table, so subsequent turns can still refer to them. Existing historical images saved only as an image placeholder cannot be reconstructed. TXT attachment text is included in context. Web chat displays text/reasoning, not an autonomous tool execution loop; use the standard API for tools and client-managed streaming.

### Verification

`go test ./...` covers proxy paths, auth/error envelopes, parameter/context preservation and incremental SSE delivery. The bootstrap tests verify that the first account is atomically created as administrator and that public first-admin registration closes after the first successful registration. Conversation repository tests verify history isolation and deletion.

For the optional official Python SDK interoperability test, set PINGTAI_TEST_PYTHON to a Python executable with `openai` already installed, then run `go test -count=1 -v ./internal/httpapi`. It uses a local mock upstream to verify SDK completion, streaming, model listing, tool-call response parsing and typed errors. This does not require a paid provider or prove availability of any particular external vendor.

## Security

Do not commit or share .env or the SQLite database. The database contains account password hashes, provider API keys and the platform JWT signing secret. Protect database backups and restrict file permissions.

