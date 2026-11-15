# pingtai Go edition

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
2. Double-click start.bat.
3. The browser opens automatically. No Notepad or command-line configuration is needed.
4. In the web wizard, keep the default local account mode or select school SSO. In local mode, create the initial administrator username and password.
5. If SSO is selected, configure the OAuth2 client ID, client secret, exact registered callback URL and SSO administrator account identifier(s). A JWT signing secret is generated automatically when left blank. The first-run wizard can accept a legacy OpenAI-compatible model configuration; it is imported into SQLite provider management on the next startup.
6. Set the SQLite database file path if needed. The server creates the directory and database file, initializes tables, and writes pingtai-config.json next to the executable. No MySQL server or database account is required.
7. Stop the current server (Ctrl+C or close its window), then double-click start.bat again. The saved configuration is loaded and the normal frontend opens.

The console window is only for service logs and shutdown, not configuration. After startup, manage AI providers, API keys, discovered models and the default model at /admin. Use /setup only for login authentication, SQLite path, listening address and system-level settings; these changes require a restart. If you change the listening port, use the new port after restart.

The EXE serves the embedded frontend directly. No frontend/ folder is required.

## Linux deployment

To build from source on Ubuntu 22.04/24.04, install Go 1.26+, Node.js 20.19+ and npm, then run `chmod +x build-linux.sh && ./build-linux.sh`. The script produces `dist/pingtai-linux-amd64` and, when `zip` is installed, `dist/pingtai-linux-amd64-release.zip`.


Extract the ZIP on a Linux AMD64 host, then run:

       chmod +x pingtai-linux-amd64 manage-go.sh
       ./manage-go.sh start

On first start (or when database/configuration initialization fails), the server stays running in setup mode and logs a private URL such as http://127.0.0.1:5050/setup#token=.... Open that exact URL in a browser. For a remote server, use HTTPS or an SSH tunnel; replace the hostname in the URL as appropriate. The setup key changes on every process start and is disabled after a successful save. Do not share this URL or its key.

Complete the same web wizard, stop and restart the process, then open the normal frontend. No terminal prompts are required. The optional `create-admin` command can create or reset a local administrator in SQLite.

For systemd, use deploy/pingtai.service. The example uses systemd `StateDirectory=pingtai` to create `/var/lib/pingtai` with ownership for the service user; both `pingtai-config.json` and `pingtai.db` are stored there. The legacy `/etc/pingtai.env` is optional. Read the initial private link from the service journal, configure in the browser, then restart with `systemctl restart pingtai`.

## Saved configuration and restart behavior

- Web configuration is stored atomically in pingtai-config.json. By default it is next to the executable; PINGTAI_CONFIG_FILE can select another writable path. The systemd example stores it under /var/lib/pingtai.
- The saved web configuration takes precedence over .env and environment variables, so old environment values cannot silently override changes made in the browser. Until the first web save, existing .env/environment configuration still works.
- The file contains credentials. Keep it private, outside any externally served WEB_DIR, and back it up securely. On Unix it is created with owner-only permissions. On Windows restrict access to the deployment folder.
- Saved passwords and keys are never sent back to the page. Blank secret inputs preserve existing values; explicit clear checkboxes remove them. Changing the JWT secret invalidates existing login tokens after restart.
- Existing active administrators are not reset by the wizard. Initial administrator fields are used only when the target database has no active administrator.
- If validation or SQLite initialization fails, the existing saved file is not overwritten. If writing the file fails after table/admin initialization, those database changes remain; fix folder permissions and save again.
- A successful save requires a manual service restart. Setup locks further configuration changes until that restart; an already-running application continues with its previous configuration.
- A SQLite file or migration failure on restart opens protected recovery setup rather than terminating the process. Health requests return HTTP 503 until normal initialization succeeds.
- Model credentials are optional for initial deployment. The page warns if the selected chat provider is incomplete; chat calls require working provider credentials and a reachable upstream service.

## School unified identity authentication

The web login uses the school's OAuth2 authorization-code flow described in the integration guide:

1. The browser visits `/auth/sso/login` and is redirected to `https://ids.zcvc.cn/cas/oauth2.0/authorize`.
2. The school returns a one-time authorization code to the registered callback.
3. The Go server exchanges the code at `/oauth2.0/token` using a form-encoded POST.
4. The server retrieves the user profile from `/oauth2.0/profile`, then issues its own short-lived access and refresh session cookies.
5. The server refreshes the school's OAuth2 refresh token when the local access session is renewed. If the school refresh token is no longer valid, the user must sign in again.

Register this exact callback URL with the school (replace the host with the public HTTPS address of this deployment):

```
https://your-pingtai-domain.example/auth/sso/callback
```

Set `SSO_CLIENT_ID`, `SSO_CLIENT_SECRET`, `SSO_REDIRECT_URL` and `SSO_ADMIN_ACCOUNTS` in the web setup page or environment. The redirect URL must exactly match the callback registered with the school. The default SSO base URL is `https://ids.zcvc.cn/cas`.

The profile parser prefers `user.useraccount`, then `user.account`, then the top-level `id` as the stable account identifier. It uses `user.username` as the display name when present. The school guide's sample profile shows these fields; confirm the account identifier selected for your tenant with the school's SSO administrator.

`SSO_ADMIN_ACCOUNTS` is a comma-, semicolon- or newline-separated allowlist of school SSO account identifiers. Only these accounts can access platform administration APIs and the protected setup page. Normal users can use the chat UI without a local account when SSO mode is selected. SQLite remains the platform's conversation and application-data store; it is not used to verify school passwords.

The OAuth2 guide does not define a separate token-revocation endpoint or guarantee a global CAS logout callback for this flow. Logging out of pingtai clears the local platform session; the school's SSO session may remain active until the school expires or ends it.


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

Open /admin and add an OpenAI-compatible provider. Enter the upstream Base URL and API key, save it, then discover models. Select the enabled models and choose a default model. Activate the provider and restart the service. The first-run OpenAI settings are imported automatically into this same provider list, so later model changes have one source of truth. Legacy CTYun/Coze deployments keep their existing configuration until migrated.

Model discovery uses the current form's Base URL and key, without saving settings or requiring a database connection. A blank API key preserves the saved key; the explicit clear checkbox sends the discovery request without a key. Discovery requires the same setup key/administrator verification as configuration changes. It queries the upstream /models endpoint, has a 20-second timeout and accepts up to 4 MB of model-list JSON. If unsupported or denied, the page reports the failure without clearing your existing selections. You can manually add model IDs (one per line or comma-separated). The list also supports search, select-all and deselect-all; select-all includes all loaded models, not just the search results. Returned models may include non-chat models; only select models your provider supports for Chat Completions.

Selected models are saved in `OpenAIModels` in pingtai-config.json. After restart they appear as separate model/agent options in the built-in chat, with the default listed first. Selecting an option routes the next chat request to that exact model. Switching models in the same conversation retains the stored conversation history; it does not ask multiple models simultaneously. If the chosen model does not support the existing image/context format, start a new conversation or choose a compatible model. Existing single-model configurations continue working. An empty selection and empty default leave web chat unconfigured.

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

The upstream API key is used only between this server and the provider. The separate MODEL_API_KEY is what clients use to call this platform. Configure it in the advanced web settings before connecting an OpenAI client.

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
- Existing /auth, /agents and /datasets routes are retained.

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
- Web image attachments retain their structured image_url content in the new ai_message_context table, so subsequent turns can still refer to them. Existing historical images saved only as an image placeholder cannot be reconstructed. TXT attachment text is included in context; PDF/DOC parsing remains a Coze feature. Web chat displays text/reasoning, not an autonomous tool execution loop; use the standard API for tools and client-managed streaming.

### Verification

`go test ./...` covers proxy paths, auth/error envelopes, parameter/context preservation and incremental SSE delivery. To run the real database setup/restart/context integration test, run `go test -count=1 -v ./cmd/pingtai`. The integration test creates an isolated temporary SQLite database and verifies initial web configuration, login, image-context persistence across reinitialization, whole-turn limits, failed-turn exclusion and conversation isolation.

For the optional official Python SDK interoperability test, set PINGTAI_TEST_PYTHON to a Python executable with `openai` already installed, then run `go test -count=1 -v ./internal/httpapi`. It uses a local mock upstream to verify SDK completion, streaming, model listing, tool-call response parsing and typed errors. This does not require a paid provider or prove availability of any particular external vendor.

## Security

Do not commit or share .env or pingtai-config.json. They contain JWT signing secrets, SSO client secrets and model API keys. The release ZIP only contains .env.example and never includes saved configuration or private setup links.


## Knowledge base versions

Knowledge documents are stored and indexed by the configured Coze knowledge service. The platform can publish version records for a dataset after all its documents finish processing. Each version records the document list, processing state, publisher and release note at that time. The original files and search index remain in Coze; these records are an audit snapshot, not a binary backup or one-click rollback. OpenAI-compatible chat requests do not automatically query Coze datasets; associate a dataset with a Coze agent when using the Coze agent chat path.
