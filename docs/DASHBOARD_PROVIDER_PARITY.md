# Dashboard ↔ Backend Provider Parity — Breakdown & TODO

> **Status**: Audit selesai 2026-09-21 (branch `backup/stitch-dashboard`, sync `main` @ `d14e116`).
> **Sumber audit**: `web/src/lib/providers.ts` (132 entri `PROVIDER_CATALOG`) +
> `web/src/lib/models.ts` (`BUILTIN_MODELS_BY_PROVIDER`, `PROVIDER_ID_TO_ALIAS`) vs
> `internal/providers/providers.go` (`KnownProviders`, 121 provider) +
> `internal/providers/aliases.go` (`ProviderAliasMap`) +
> `internal/providers/registry_models.go` (`ProviderModels`) +
> `internal/providers/oauth.go` (`KnownOAuthConfigs`).
> Script verifikasi: `/tmp/cmp_models.py` (FE vs BE model-set per provider, canonical
> via `ProviderAliasMap` / `PROVIDER_ID_TO_ALIAS`).

**Kabar baik**: semua 121 provider backend tampil di dashboard — tidak ada yang hilang.
Yang belum sama ada 4 kelompok di bawah. Setiap item punya **cara verifikasi** dan
**acceptance criteria** supaya executor bisa cek 1-per-1.

Cara verifikasi umum (server lokal, DB real `~/.9router/db/data.sqlite`):

```bash
PORT=20128 go run ./cmd/9router-go   # atau binary build
KEY=$(sqlite3 ~/.9router/db/data.sqlite "SELECT key FROM apiKeys WHERE isActive=1 LIMIT 1;")
curl -s -H "Authorization: Bearer $KEY" localhost:20128/api/connections | python3 -m json.tool
```

---

## Kelompok 1 — Alias tidak resolve di backend (5 provider)

**Dampak**: alias diketik di CLI / combo / chat (`cbcn`, `ps`, `vx`, `vxp`, `voyage`)
gagal dengan `could not resolve model: ...` (`internal/handlers/chat/resolution.go:311`).
Dashboard kirim `id` kanonis saat create connection, jadi yang kena hanya pemakaian alias.

| # | Provider | Alias dashboard (`providers.ts`) | Alias backend (`aliases.go`) | TODO |
|---|----------|----------------------------------|------------------------------|------|
| 1.1 | `codebuddy-cn` | `cbcn` | `cd` | Tambah `"cbcn": "codebuddy-cn"` ke `ProviderAliasMap`. (Catatan: `cbcn` SUDAH jadi key di `ProviderModels`, jadi model lookup aman — hanya resolusi koneksi yang mati.) |
| 1.2 | `poolside` | `ps` | — (tidak ada) | Tambah `"ps": "poolside"` ke `ProviderAliasMap`. |
| 1.3 | `vertex` | `vx` | — | Tambah `"vx": "vertex"` ke `ProviderAliasMap`. |
| 1.4 | `vertex-partner` | `vxp` | — | Tambah `"vxp": "vertex-partner"` ke `ProviderAliasMap`. |
| 1.5 | `voyage-ai` | `voyage` | — | Tambah `"voyage": "voyage-ai"` ke `ProviderAliasMap`. |

**Verifikasi**: `go test ./internal/providers/ -run TestResolveAlias` (atau manual:
`resolveProviderAlias("cbcn")` harus return `codebuddy-cn`). Checklist ada di
`internal/providers/aliases.go` — pastikan tidak tabrakan dengan alias existing.

**Acceptance**: 5 alias di atas resolve ke id kanonis; tidak ada alias duplikat;
`go build ./...` + `go test ./internal/providers/` hijau.

---

## Kelompok 2 — Kategori OAuth jomplang (2 provider)

**Dampak**: backend bisa refresh token OAuth (`KnownOAuthConfigs` di
`internal/providers/oauth.go`, dipakai `gemini_handler.go:225` +
`internal/proxy/oauth/init.go:12`), tapi dashboard tidak menampilkan flow/banner
OAuth karena `category` bukan `oauth` (`ProviderDetailView.svelte:56`,
`ProvidersOverviewGrid.svelte:43-44`). User hanya bisa pakai API key manual dan
token OAuth yang ditempel tidak di-refresh.

| # | Provider | Kategori dashboard | OAuth backend | TODO |
|---|----------|--------------------|---------------|------|
| 2.1 | `kimi-coding` | `apikey` | Ada (`auth.kimi.com/api/oauth/token`) | Ubah `category` → `oauth` di `providers.ts`. Pastikan tombol login/import OAuth muncul di `ProviderDetailView`. |
| 2.2 | `gemini-cli` | `free` | Ada (Google OAuth client `681255809395-...`) | Ubah `category` → `oauth` di `providers.ts`. Cek `noAuth`/label `free` tidak menutupi tombol OAuth. |

**Verifikasi**: buka `/dashboard/providers/kimi-coding` dan `/dashboard/providers/gemini-cli` —
banner + tombol OAuth muncul; koneksi tersimpan dengan `authType: 'oauth'`.

**Acceptance**: kedua provider tampil di grid OAuth (`ProvidersOverviewGrid`),
refresh token jalan (cek log `gemini_handler` saat token kedaluwarsa).

> Catatan (bukan TODO): 7 provider berkategori `oauth` di dashboard TANPA entri
> `KnownOAuthConfigs` (`claude`, `cursor`, `gitlab`, `kilocode`, `kimi`,
> `xiaomi-mimo`, `zed`) — sengaja, karena didukung via generic OAuth import
> (`HandleOAuthImport`). Jangan "diperbaiki" dengan menghapus kategorinya.

---

## Kelompok 3 — noAuth jomplang (3 provider)

**Dampak**: backend tidak butuh key (`NoAuth: true` = lokal, tanpa auth) tapi
dashboard (`category: apikey`, tanpa `noAuth: true`) memaksa user mengisi API key
untuk provider lokal. `isNoAuth` dihitung di `ProviderDetailView.svelte:57`.

| # | Provider | Dashboard | Backend | TODO |
|---|----------|-----------|---------|------|
| 3.1 | `comfyui` | `apikey`, minta key | `NoAuth: true` (lokal) | Set `category` → `freeTier` + `noAuth: true` di `providers.ts` (samakan dengan `local-device` yang sudah benar). |
| 3.2 | `sdwebui` | `apikey`, minta key | `NoAuth: true` (lokal) | Sama seperti 3.1. |
| 3.3 | `recraft` | `apikey`, minta key | `DefaultAPIKey: "public"` (key bawaan, seperti `opencode`) | Set `category` → `free` + `noAuth: true` (samakan dengan `opencode`; frontend sudah hardcode `providerId === 'opencode'` di `ProviderDetailView.svelte:57` sebagai preseden — pertimbangkan pola yang sama atau generalisasi `DefaultAPIKey`). |

**Verifikasi**: buka halaman detail provider — form tidak wajibkan API key;
buat koneksi tanpa key, lalu `POST /api/connections/{id}/test` return OK.

**Acceptance**: koneksi `comfyui`/`sdwebui`/`recraft` bisa dibuat tanpa key dan
lolos test-connection; tidak ada regresi ke provider `apikey` lain.

> Bukan TODO: `opencode` (`DefaultAPIKey: "public"`, FE `free`/`noAuth: true`) dan
> `mimo-free` / `searxng` / `tortoise` / `coqui` / `edge-tts` / `google-tts` /
> `local-device` (`NoAuth: true`, FE `freeTier`/`free` + `noAuth: true`) — semua
> sudah konsisten, jangan diubah.

---

## Kelompok 4 — Daftar model kosong / basi di dashboard (14+ provider)

**Dampak**: dropdown model di dashboard kosong (user tidak bisa pilih model) atau
menampilkan model yang sudah pensiun / tidak dikenal backend (request gagal
validasi atau model tidak ada).

### 4A. Dashboard tidak punya seksi model (12) — form model kosong

File: `web/src/lib/models.ts` → tambah seksi di `BUILTIN_MODELS_BY_PROVIDER`
dengan daftar dari `ProviderModels` (`internal/providers/registry_models.go`):

| # | Provider | Model backend (jumlah) |
|---|----------|------------------------|
| 4A.1 | `elevenlabs` | 2 (`eleven_multilingual_v2`, `eleven_turbo_v2_5`) — catatan: FE sudah punya sub-register `elevenlabs-tts-models`, tapi seksi provider-nya tidak ada |
| 4A.2 | `aws-polly` | 4 (`standard`, `neural`, `long-form`, `generative`) |
| 4A.3 | `cartesia` | 2 (`sonic-2`, `sonic-3`) |
| 4A.4 | `devin-cli` | 35 |
| 4A.5 | `trae` | 9 |
| 4A.6 | `windsurf` | 75 |
| 4A.7 | `fish-audio` | 4 |
| 4A.8 | `inworld` | 2 |
| 4A.9 | `playht` | 2 |
| 4A.10 | `coqui` | 1 |
| 4A.11 | `tortoise` | 1 |
| 4A.12 | `jina-ai` | 3 |

### 4B. Dashboard punya, backend tidak kenal (3) — validasi bisa nolak

| # | Provider | Isi dashboard | TODO |
|---|----------|---------------|------|
| 4B.1 | `edge-tts` | 11 model | Tambah entri `edge-tts` ke `ProviderModels` (backend), atau hapus seksi FE jika provider memang tidak didukung backend. |
| 4B.2 | `google-tts` | 60 model (voice list) | Sama seperti 4B.1. |
| 4B.3 | `local-device` | 1 model | Sama seperti 4B.1. (`local-device` ada di `KnownProviders`, jadi arah fix yang benar = tambah model ke backend.) |

### 4C. Selisih isi model (5) — sinkronkan kedua arah

| # | Provider | Hanya di dashboard (FE) → hapus / verifikasi | Hanya di backend (BE) → tambah ke FE |
|---|----------|-----------------------------------------------|---------------------------------------|
| 4C.1 | `codebuddy-intl` | `deepseek-v4-flash` (PENSIUN, ganti ke `deepseek-v4.1-flash`) | `deepseek-v4.1-flash` |
| 4C.2 | `codex` | `gpt-image-1.5`, `gpt-image-2`, `gpt-image-2.5`, `gpt-image-2.5-flare`, `gpt-image-2.5-sunburst` (5 model image — DITUNDA, tidak bisa verifikasi upstream; dibiarkan agar tidak merusak seleksi yang mungkin valid) | `codex-auto-review` ✅ ditambah ke FE 2026-09-21 |
| 4C.3 | `grok-cli` | `grok-build` — INI ALIAS BACKEND, bukan model. Kemungkinan salah tempat; pindahkan/relasikan ke alias, bukan daftar model | — |
| 4C.4 | `ollama` | — | `deepseek-v4.1-flash:cloud` |
| 4C.5 | `opencode` | 8 model free — hasil cek `https://opencode.ai/zen/v1/models` (74 model, 2026-09-21): KEEP `big-pickle`, `mimo-v2.5-free`, `nemotron-3-ultra-free`, `nemotron-3.5-lightning-free` (HIT upstream ✅, 4 ini juga ditambah ke `ProviderModels` backend); HAPUS `laguna-s-2.1-free`, `ling-3.0-flash-free`, `north-mini-code-free`, `union-alpha` (MISS upstream ✅ dihapus 2026-09-21) | — |

**Verifikasi (semua 4A–4C)**: jalankan `/tmp/cmp_models.py` (atau tulis ulang
sebagai Go test bila mau permanen) — target: nol selisih di luar sub-register
`*-tts-models` / `*-tts-voices` / `*-voices` (itu picker voice, bukan provider,
sengaja beda mekanisme dengan `internal/handlers/media/voices.go`).
Lalu cek manual 1-per-1 di `/dashboard/providers/<id>` — dropdown model terisi
dan model terpilih lolos request real.

**Acceptance**: diff model FE vs BE = 0 (di luar sub-register voice yang
by-design); `go test ./internal/providers/` hijau.

---

## Bukan TODO (sengaja dibiarkan beda)

- **8 entri header semu** di katalog (`x-codebuddy-request`, `x-github-api-version`,
  `x-requested-with`, `x-vscode-user-agent-library-version`, `anthropic-version`,
  `openai-intent`, `originator`, `user-agent`) — cerminan `StaticHeaders` backend
  untuk form koneksi custom. JANGAN dihapus tanpa mengganti UI custom-header.
  Tapi JANGAN juga dipakai membuat koneksi (resolusi backend pasti gagal,
  `resolution.go:311`) — pertimbangkan guard di `HandleCreateConnection`.
- **`selfhosted-tts` / `selfhosted-stt` / `selfhosted-embedding`** — ada di FE +
  `ProviderModels`, tapi tanpa `KnownProviders` / handler. Membuat koneksi dengan
  id ini mati di resolusi. TODO lanjutan (belum dipecah): definisikan endpoint
  lokal untuk ketiganya, atau sembunyikan dari katalog sampai didukung.
- **`serviceKinds` vs URL override** (mis. FE `openai: [image, tts, stt]`, backend
  tanpa `ImageURL`/`TTSURL`/`STTURL`) — BUKAN mismatch: media di-route generik via
  `BaseURL` OpenAI-compatible (`internal/handlers/media/media.go`). Tidak perlu action.
- **Spek historis** `docs/superpowers/specs/2026-07-19-mitm-proxy-design.md` masih
  menyebut `localhost:20128` — dokumen bertanggal, jangan diubah.
- **Port default kini `20130`** (`config.go`, `Makefile`, compose, Dockerfile,
  README sudah disamakan). Dashboard user di `:20128` = override `PORT=20128`,
  bukan default. Jangan "samakan" balik ke 20128.

## Urutan eksekusi yang disarankan

1. Kelompok 1 (alias) — 5 baris di `aliases.go`, risiko nol, test ada.
2. Kelompok 3 (noAuth) — 3 entri di `providers.ts`, murni frontend.
3. Kelompok 2 (OAuth) — 2 entri kategori + cek UI flow.
4. Kelompok 4 (models) — paling besar; kerjakan per sub-tabel 4A → 4B → 4C,
   verifikasi dengan script diff tiap selesai.

## Kelompok 5 — Auth OAuth selalu ke Google (DIPERBAIKI 2026-09-21)

**Gejala**: di dashboard Go, tombol login OAuth SEMUA provider membuka URL
Google Antigravity (`openGenericOAuth()` → `openAntigravityOAuth()`), padahal
tiap provider punya flow sendiri. Ketahuan saat bandingkan
`/dashboard/providers/clinepass` Go vs upstream TS (`:20128`, Next.js).

**Referensi upstream** (ditarik live dari `:20128/api/oauth/cline/*`):
- `GET /api/oauth/cline/authorize` → `{authUrl: https://api.cline.bot/api/v1/auth/authorize?client_type=extension&callback_url=...&redirect_uri=<dashboard-host>/callback, state, codeVerifier, codeChallenge, redirectUri, flowType: authorization_code}`
- Callback URL dinamis ngikutin host+port tempat dashboard dibuka (`internal/handlers/oauth/redirect.go`: `callbackRedirectURI`/`exchangeRedirectURI` — override via `?redirect_uri=` valid http(s), fallback `http://localhost:8080/callback` hanya bila request tanpa Host). Berlaku untuk cline, pkce, authcode, trae, windsurf, kimchi, xiaomi-mimo; antigravity sudah dinamis duluan (`getAntigravityRedirectURI`). Placeholder modal FE juga dinamis (`dashboardOrigin()`).
- `GET /callback` publik (tanpa API key, `internal/handlers/oauth/callback.go`): halaman penangkap — provider redirect browser ke sini, halaman tampilkan code + tombol Copy untuk dipaste ke modal dashboard. Full static + CSP `no-store`, nilai query tidak pernah direfleksikan ke HTML (XSS-safe). Didaftarkan di `SetupServerRouter` (bukan grup protected).
- **Auto-handoff (tanpa copy-paste)**: FE kirim `redirect_uri=<dashboard-origin>/callback` eksplisit di semua authorize (`client.ts`); callback tab (SPA `OAuthCallbackView` untuk dev, halaman Go `/callback` untuk production — keduanya same-origin dengan dashboard) tulis hasil ke `localStorage 9router.oauth.callback.v1` + `BroadcastChannel 9router-oauth`; modal (`ProviderDetailView`) simpan sesi pending saat authorize (`9router.oauth.pending.v1`: state, verifier, redirectUri, extra), dengarkan BC/storage/poll saat modal terbuka, cocokkan via `state`, restore verifier, isi `callbackInput`, auto-submit sekali (`web/src/lib/oauth-handoff.ts`, 15 bun test). Gagal auto → fallback paste manual tetap ada. Berlaku untuk cline, pkce, authcode, trae/windsurf/zed, kimchi, xiaomi-mimo; device-code tetap polling, cursor/PAT/cookie tetap manual.
- **Dual-auth clinepass (parity upstream)**: `clinepass` registry upstream `authModes: ["apikey","oauth"]` → halaman detail upstream tampil 2 tombol (OAuth + API Key). Go: `ProviderCatalogItem.authModes?` (`providers.ts`) + `hasDualAuthModes` di `ProviderDetailView` → tombol ganda OAuth (sekunder) + API Key (primer, buka `AddConnectionModal` → `authType: apikey`) di empty-state & header, plus hint "Choose OAuth or API Key.". Mekanisme generik — provider lain tinggal tambah `authModes` di katalog.
- **Quota page backend (2026-09-21)**: FE `QuotaTrackerView + quota/` (port upstream ProviderLimits) butuh `GET /api/providers/client` ber-pagination — backend Go sebelumnya mengabaikan semua param. `HandleGetProvidersClient` (`internal/handlers/dashboard/connections.go`) sekarang: filter eligibility (`features.usage` 24 provider + `usageApikey` 16 provider), filter `provider`/`accountStatus`, sort `priority|provider`, `page/pageSize` (default 20, max 500 clamp), `providerOptions`, `totals`, sanitize whitelist + `maskName` ala upstream (secret `data.apiKey` tidak pernah bocor). Test `providers_client_test.go` (pagination, filter, paging clamp, sort, no-leak). Satu-satunya konsumen endpoint ini = quota page, jadi aman.
- **Live usage 8 provider (2026-09-21)**: `GET /api/usage/{id}` Go tadinya live hanya untuk Antigravity (sisanya lock basi/kosong). `internal/handlers/dashboard/usage_providers.go` port `open-sse/services/usage/`: deepseek (balance), groq (header x-ratelimit + durasi Go), commandcode (whoami→credits+subs), ollama (limits+me), qoder (quota/usage), codebuddy-intl (Tencent refill/bonus), kiro (3x codewhisperer attempts + authMethod headers), grok-cli (billing+user, JWT tier, tanpa fallback gRPC). Terverifikasi live dengan kredensial asli: codebuddy-intl 3 baris, commandcode Credits, deepseek USD+CNY, grok-cli On-demand, kiro "KIRO STUDENT". qoder 401 = token tersimpan kedaluwarsa (perlu re-login, perilaku sama seperti upstream). Test helper `usage_providers_test.go`.
- **Antigravity 100%-semua fix (2026-09-21)**: path dashboard Go tadinya parse SEMUA model dari host PROD (`cloudcode-pa`) tanpa tier-check → akun free-tier/G1-Pro render 25 baris 100%. Akar masalah dibuktikan via curl langsung: PROD lapor `remainingFraction=1.0` (optimistis/basi) sementara DAILY (`daily-cloudcode-pa`, host yang dipakai upstream usage) menghilangkan `remainingFraction` untuk model exhausted dengan reset asli. `fetchAntigravityDashboardUsage` sekarang parity `getAntigravityUsage`: sub-info via PROD loadCodeAssist (plan/paidTier/project), `fetchAvailableModels` via DAILY, skip per-model bila free-tier murni, filter 17 important-models basis 1000, weekly overlay via DAILY `retrieveUserQuotaSummary` (weekly-only, tanpa session rows) + reconcile rule. Hasil live byte-identik dengan upstream :20128 (19 baris, nilai sama; hanya `resetAt` tanpa `.000` milidetik — kosmetik). Routing chat (`chat.RefreshAntigravityQuota` di PROD) tidak diubah.
- **Parity usage 9/9 provider terverifikasi live (2026-09-21)**: tiap koneksi dibandingkan Go `:20131` vs upstream `:20128` (normalisasi hanya millis `.000Z`). Hasil: `antigravity, codebuddy-intl, commandcode, deepseek, grok-cli, groq, kiro, ollama, qoder` semua IDENTIK. Temuan selama verifikasi (sudah fix): (1) `codebuddyIsRefill` salah kira `DeductionEndTime` detik → padahal milidetik, semua pack ke-label refill; (2) `usageResetTime` parse naive datetime sebagai UTC → sekarang `time.Local` meniru JS `new Date(str)`; (3) grok-cli Go sempat kirim `periodEnd` — upstream tidak pernah; (4) `toResponse` hilangkan `quotas` saat message — upstream selalu sertakan `quotas:{}` kecuali qoder (flag `bare`); (5) kiro 401 sekali = flaky AWS sesaat, retry langsung `KIRO STUDENT` identik. qoder 401 persisten = token tersimpan kedaluwarsa (perlu re-login; response error byte-identik dengan upstream).
- **Scan penuh 122 registry upstream (2026-09-21)**: dual-auth tepat 9 → `clinepass, codebuddy-cn, codebuddy-intl, kimchi, kimi, qoder, windsurf, xai, xiaomi-mimo` (semua sudah `authModes` di `providers.ts`; gate `hasDualAuthModes` murni data-driven agar `kimchi` freeTier ikut tercover). Label tombol ikut upstream: xai "Grok Build OAuth"/"xAI API Key", kimi "Kimi Coding OAuth"/"Kimi API Key", qoder key "PAT", sisanya "OAuth"/"API Key". Kategori diluruskan: `trae, windsurf apikey→oauth`. `kimi-coding` (belahan Go, tidak ada di upstream) tetap single-mode.
- **Koreksi kategori (2026-09-21)**: `comfyui, sdwebui, recraft` revert ke `apikey` tanpa `noAuth` — registry upstream `category: apikey` dan modal upstream wajib isi key (submit disabled bila kosong); keputusan keyless Kelompok-3 dicabut. `devin-cli apikey→free + noAuth:true` (upstream `free/noAuth/authModes:[none]` → `NoAuthProxyCard`, backend Go `devin://acp/stdio` lokal). `gemini-cli oauth→free` (flow OAuth tetap via `isAuthCodeOAuth` yang providerId-driven). `mmf` ditambahkan (`apikey, hidden:true, priority:200` + models `mimo-auto` sudah ada di `models.ts` + alias backend `mmf→mimo-free` sudah ada); `hidden` difilter di `ProvidersOverviewGrid` seperti upstream, halaman detail tetap bisa dibuka langsung.
- `POST /api/oauth/cline/exchange` `{code, codeVerifier, state, redirectUri}` → token envelope `{success, data: {accessToken, refreshToken}}`
- Kontrak exchange endpoint: `snake_case` (`grant_type`, `code_verifier`, `redirect_uri`, `client_type`) + header `Cline/3.0.61` + `X-CLIENT-*` (terbukti via error oracle upstream; fake code return `"invalid or expired authorization code"` = shape benar)

**Yang dikerjakan**:
- Backend `internal/handlers/oauth/cline.go` (baru): `HandleClineAuthorize` +
  `HandleClineExchange` untuk `cline` + `clinepass`, simpan koneksi `authType: 'oauth'`.
  Route di `internal/handlers/router.go`. Test `cline_test.go` (4 test).
- Frontend `web/src/api/client.ts`: `getClineAuthorizeUrl` + `clineExchange` +
  `importOAuthToken` (via `POST /api/oauth/{provider}/import` yang sudah ada).
- Frontend `ProviderDetailView.svelte`: `cline`/`clinepass` → flow Cline (popup
  api.cline.bot + tukar code); provider OAuth lain TANPA authorize endpoint →
  modal import token manual (tidak lagi ke Google).
- Verifikasi: response authorize Go identik bentuknya dengan upstream; exchange
  fake code return error Cline yang sama persis dengan upstream (= kontrak benar,
  tinggal code asli dari login betulan).

**Sisa lanjutan (belum dikerjakan)**: flow khusus `kiro` social, `codex` bulk,
`grok-cli` bulk ada endpoint backend-nya tapi belum ada UI di dashboard —
masih lewat modal import manual.

### Lanjutan 2026-09-21 — port semua keluarga OAuth upstream

Referensi: `/Users/luqmannul.hakim/htdocs/9router` (`src/lib/oauth/providers/*` 25 modul,
`src/app/api/oauth/*`, `open-sse/providers/registry/*`, `open-sse/shared/{zedAuth,qoder/*}`).

**Grup 1 — PKCE** (`internal/handlers/oauth/pkce.go`, generic `GET/POST /api/oauth/pkce/*`):
`claude` (claude.ai + JSON exchange + `#`-suffix strip), `codex` (auth.openai.com +
form + extraParams + email dari id_token), `xai` (OIDC discovery + nonce/plan/referrer,
email dari id_token), `gitlab` (baseUrl + clientId/Secret custom via modal, userinfo
`/api/v4/user`). Frontend: `isPKCEOAuth` + `pkceAuthorize/pkceExchange` + input
khusus GitLab. Test `pkce_test.go`.

**Grup 2a — authcode biasa** (`authcode.go`, `GET/POST /api/oauth/authcode/*`):
`gemini-cli` (Google OAuth client `681255809395…` + userinfo email),
`iflow` (Basic auth + userinfo, **apiKey dari userinfo wajib ada**).
Frontend: `isAuthCodeOAuth` + `authcodeAuthorize/authcodeExchange`.

**Grup 2b — custom** (`trae.go`, `windsurf.go`, `zed.go` + `POST/GET` per provider):
- `trae`: GetLoginGuidance → verification URL → callback parse → ExchangeToken
  (SSRF allowlist 4 origin) → userinfo → providerSpecificData SOLO
  (scope/aiRegion/tenant marscode…); plus mode paste-JWT.
- `windsurf`: windsurf.com/signin → callback `access_token` → RegisterUser →
  apiKey (+ paste `sk-ws-*` langsung / firebase JWT); userinfo best-effort.
- `zed`: RSA-2048 lokal + `native_app_signin` (tanpa local listener — user paste
  callback manual sesuai UX modal); dekripsi OAEP-SHA256 + fallback PKCS1v15
  dengan cek `�`; userinfo `cloud.zed.dev/client/users/me` + org resolve.
  Test `custom_test.go` incl. round-trip RSA penuh.
- Frontend: `isCustomOAuth` + `customAuthorize/customExchange`.

**Grup 3 — device-code** (`device.go`, `POST /api/oauth/device/start|poll` +
`session` opaque): `qoder` (PKCE+nonce lokal, poll openapi.qoder.sh, 202/404 =
pending), `kilocode` (initiate/poll api.kilo.ai, 202/403/410), `grok-cli`
(form + referrer + UA grok-pager), `github` (device flow standar + copilot
token + userinfo), `kiro` (register client AWS SSO OIDC + region allowlist
`^[a-z]{2}-[a-z]+-\d{1,2}$` + profileArn), `kimi`/`kimi-coding` (header X-Msh-* +
deviceId stabil), `codebuddy-cn/intl` (state+poll GET, code 11217 = pending,
domain/UA per varian). Frontend: `isDeviceOAuth` + `deviceStart/devicePoll` +
modal user_code + auto-poll per interval. Test `device_test.go`.
Verifikasi live 8/8: semua start return user_code + verification URL asli.

**Grup 4 — khusus** (`special.go`, `mimo.go`):
- `cursor`: import (validasi format token + UUID + JWT email) + auto-import
  baca `state.vscdb` lokal via modernc/sqlite (darwin/linux/win).
- `kimchi`: `app.kimchi.dev/cli-auth` + validasi token via api.cast.ai + userinfo.
- `xiaomi-mimo`: ECDH X25519 (stdlib `crypto/ecdh`) + AES-GCM; format verifier
  `mimo-x25519:`; test round-trip penuh.
- `gitlab PAT`: verifikasi via `Private-Token` userinfo, simpan
  `authKind: personal_access_token`.
- `iflow cookie`: cookie `BXAuth` → GET+POST `platform.iflow.cn/api/openapi/apikey`.
- Frontend: `isSpecialOAuth` + blok modal per provider (cursor import/auto-import,
  PAT gitlab, cookie iflow) + `cursorImport/cursorAutoImport/kimchiAuthorize/
  kimchiExchange/gitlabPAT/iflowCookie/mimoAuthorize/mimoExchange`.

**Yang masih manual-import** (tanpa authorize endpoint di upstream maupun Go):
`kimi-coding` (tercover device kimi), provider OAuth lain tanpa modul upstream
(`cursor` selain import, `gitlab` selain PKCE/PAT, `kilocode` selain device…).
`openGenericOAuth()` tidak lagi mengarah ke Google untuk provider manapun.

---

## Kelompok 6 — Modal "Add API Key" (DIPERBAIKI 2026-09-21)

**Gejala**: di `/dashboard/providers/clinepass`, klik tombol *Add ClinePass API Key*
memunculkan modal yang jauh lebih sederhana daripada upstream `:20128`
(Next.js): dua field saja, tanpa Check, tanpa Priority/Proxy Pool, footer
`Cancel` + `Save Connection`. Upstream pakai `AddApiKeyModal.js`.

**Referensi upstream**: `src/app/(dashboard)/dashboard/providers/[id]/AddApiKeyModal.js`,
`src/app/api/providers/validate/route.js`, `src/app/api/providers/route.js` (POST),
`src/shared/utils/bulkAdd.js`, `src/lib/db/repos/connectionsRepo.js`.

**Yang dikerjakan**:
- **Backend `POST /api/providers/validate`** (`internal/handlers/dashboard/validate.go`):
  probe API key mentah. Cabang: `noAuth` → valid; provider-specific
  (`cloudflare-ai` accountId, `azure` endpoint+deployment+apiVersion, `ollama-local`
  host, `gemini` `?key=`, `xiaomi-tokenplan` region); Anthropic-style
  (`AuthHeader: x-api-key` atau BaseURL `/messages`) → POST messages; sisanya
  generic OpenAI-compatible: GET `<base tanpa /chat/completions>/models`, fallback
  POST chat minimal. **401/403 = invalid, 400/lainnya = key diterima** (persis
  upstream). Response `{valid, supported, error}`.
- **Backend `POST /api/connections`** menerima `priority`, `providerSpecificData`,
  `testStatus`, `proxyPoolId`, `displayName` (mirror `POST /api/providers`
  upstream) dan menulisnya ke kolom `priority` + data JSON. Tanpa `priority` →
  `max(priority)+1` (`Repo.NextConnectionPriority`).
- **Frontend** `AddConnectionModal.svelte` ditulis ulang: mode **Single / Bulk Add**,
  field **Name** (wajib, placeholder `Production Key`), credential + tombol
  **Check** + badge **Valid/Invalid**, **Priority**, **Proxy Pool**, sub-form
  **Azure OpenAI**, **Cloudflare Workers AI**, **Ollama Host URL**, **Region**,
  footer **Save** + **Cancel** (dua-duanya full width). Bulk pakai
  `planBulkAdd` (`web/src/lib/bulk-add.ts`, 15 test) supaya nama auto-generated
  tidak menimpa koneksi yang sudah ada.
- **Katalog** `providers.ts`: tambah `regions`/`defaultRegion` untuk
  `xiaomi-tokenplan` (sgp/cn/ams — satu-satunya provider apikey yang
  cluster-specific di upstream; `trae` regions hanya relevan untuk jalur OAuth).
- **ProviderDetailView**: error simpan tampil inline di modal (bukan `alert`),
  `existingNames` + `proxyPools` + `isCompatible`/`isAnthropic` diteruskan, dan
  model default node compatible disimpan sebagai
  `providerSpecificData.assignedModel` (dibaca proxy, lihat `strict_model_test.go`).

**Beda yang disengaja** (jangan "diperbaiki" tanpa alasan):
- Provider di luar `KnownProviders` (mis. node compatible custom) → 400, dan UI
  menampilkan catatan *"Auto-check is not available for this provider"* alih-alih
  mengklaim "Invalid" palsu. Provider registry yang punya `BaseURL` selalu bisa
  di-Check.

### Lanjutan 2026-09-21 — probe cookie/PAT (grok-web, perplexity-web, qoder, iflow)

Empat provider yang tadinya di-blocklist sudah diport dari `validate/route.js`,
jadi **tidak ada lagi provider registry yang "unsupported"**:
- **`grok-web`**: POST `grok.com/rest/app-chat/conversations/new` dengan header
  fingerprint browser lengkap (UA Chrome 136, `Sec-Ch-*`, `Origin`/`Referer`
  grok.com, `x-statsig-id` = base64 dari literal upstream, `traceparent`
  `00-<32hex>-<16hex>-00`, `x-xai-request-id` UUID) + `Cookie: sso=<token>`
  (prefix `sso=` di-strip, tidak digandakan). Valid = non-401/403 (400/429 = cookie
  diterima). Pesan gagal menyalin panduan upstream.
- **`perplexity-web`**: POST `perplexity.ai/rest/sse/perplexity_ask` dengan
  `Cookie: __Secure-next-auth.session-token=<token>` (prefix di-strip), header
  `X-App-ApiClient`/`X-App-ApiVersion: 2.18`, body `params` lengkap
  (`frontend_uuid`/`frontend_context_uuid` UUID, `version` 2.18, `timezone` dari
  lokasi server — upstream pakai TZ browser). Valid = non-401/403.
- **`qoder`**: PAT (`pt-`) → `POST openapi.qoder.sh/api/v1/jobToken/exchange`
  (`personal_token`, header `User-Agent: qodercli/1.0.0` + `Cosy-Version`/
  `Cosy-ClientType`) → `GET /api/v1/userinfo` (best-effort userId; fallback
  `providerSpecificData.userId`) → `GET /algo/api/v2/model/list` **COSY-signed**,
  ke **api2.qoder.sh** untuk token `jt-` (api3 balas 403 "Login expired") dan
  api3 untuk `dt-`. Valid = `chat[]` punya minimal satu entri `key` yang tidak
  `enable:false` (sama seperti `models.length` upstream). Tanpa userId → invalid
  dengan pesan "user ID missing".
- **`iflow`**: tidak punya cabang khusus di upstream, jadi ikut jalur generic
  (`GET apis.iflow.cn/v1/models` + Bearer, fallback POST chat). Blocklist-nya
  memang salah tempat dan sudah dihapus.

Pendukung: `executor.BuildQoderCosyHeaders` sekarang **publik** dan menghitung
`sigPath` dari URL (strip prefix `/algo`) seperti `computeSigPath` upstream —
sebelumnya hardcode `/api/v2/service/pro/sse/agent_chat_generation`. Nilai untuk
URL chat tidak berubah (dikunci `qoder_cosy_test.go`).

**Belum diport** (di luar key-validate): `vertex`/`vertex-partner` (SA JSON vs raw
key), `grok-web`-style STATSIG refresh, `commandcode`/`qoder` chat-probe khusus,
`blackbox`, `deepgram Token`, `opencode-go`, `xai` 403-as-valid, dan `validateUrl`
spesifik per provider (deepseek `/user/balance`, dll.). Untuk provider ini generic
`/models` + fallback chat dipakai; hasilnya tetap benar untuk 401/403 tapi pesan
gagalnya generik.
- Hint upstream *"Legacy manual proxy fields are still accepted by API…"* tidak
  ditampilkan: backend Go tidak punya field proxy legacy.
- `reorderInTx` upstream (renumber priority per provider setelah insert) tidak
  diport — priority disimpan apa adanya; renumber otomatis belum ada di Go.
- Provider-specific `validateUrl` upstream yang spesifik (deepseek `/user/balance`,
  dll.) belum diport; generic `/models` + fallback chat dipakai untuk semuanya.

**Verifikasi**: `go test ./internal/handlers/dashboard/` (validate: probe matrix,
provider-specific branch, alias, unsupported; create: priority/psd/testStatus/
proxyPoolId + jalur legacy) dan `bun test web/src/lib/bulk-add.test.ts` (15).
Tes live dengan key asli belum dilakukan — pakai tombol **Check** di dashboard.
