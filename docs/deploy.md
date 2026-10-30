# Deploying PullWise (Render + Vercel)

Three Render pieces (one free Postgres, the backend, the knowledge service) and one Vercel site.
Vercel serves the React app and forwards `/api` and `/auth` to the Render backend, so the browser
sees one origin and the sign-in cookie works unchanged.

```
browser -> Vercel (React app, rewrites /api and /auth)
              -> Render: pullwise-backend --> Render: pullwise-knowledge
                              \                    /
                               Render Postgres (public schema = PullWise, "knowledge" schema = vectors)
```

## 1. Before you start

- Both repos pushed to GitHub (`render.yaml` and `frontend/vercel.json` must be on `main`).
- Accounts on render.com and vercel.com, each connected to your GitHub.
- Your Groq and Cohere keys (they stay in the dashboards, never in chat or git).
- Make two secrets on your own machine (PowerShell):

```powershell
$b = New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)   # SESSION_SECRET
$b = New-Object byte[] 36; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)   # the shared token secret
```

## 2. Render

1. Dashboard, **New**, **Blueprint**, pick the PullWise repo. Render reads `render.yaml` and proposes the
   database and two services. If it refuses the knowledge service (it lives in another repo), create it by
   hand: **New**, **Web Service**, repo `pullwise-knowledge`, runtime **Docker**, and give it the variables
   listed under it in `render.yaml`.
2. Fill the prompted values:
   - `pullwise-backend`: `GROQ_API_KEY`, `SESSION_SECRET`, `KNOWLEDGE_JWT_SECRET` (the shared token secret),
     `KNOWLEDGE_URL` (`https://pullwise-knowledge.onrender.com`; use the real address Render shows).
     Leave `GITHUB_OAUTH_*` and `FRONTEND_URL` empty for now; `GITHUB_TOKEN` is optional (raises GitHub's limit).
   - `pullwise-knowledge`: `COHERE_API_KEY` and `JWT_SECRET` (**the same value** as the backend's `KNOWLEDGE_JWT_SECRET`).
3. Wait for both services to show **Live**. Check `https://<backend>.onrender.com/healthz` and
   `https://<knowledge>.onrender.com/healthz`.
4. If Render gave the backend a different address than `pullwise-backend.onrender.com` (the name was taken),
   edit the two `destination` lines in `frontend/vercel.json` and push.

## 3. Vercel

1. **Add New, Project**, import the PullWise repo, set **Root Directory** to `frontend`. The build settings
   come from `frontend/vercel.json`. Deploy.
2. Note the address Vercel gives you (for example `https://pullwise.vercel.app`).

## 4. GitHub sign-in

1. GitHub, Settings, Developer settings, **OAuth Apps**, New. Homepage: your Vercel address. Callback URL:
   `https://<your-vercel-address>/auth/github/callback`. Enable token expiry if offered. Generate a secret.
2. In Render, on `pullwise-backend`, set `GITHUB_OAUTH_CLIENT_ID`, `GITHUB_OAUTH_CLIENT_SECRET`,
   `FRONTEND_URL` (the Vercel address) and `GITHUB_OAUTH_REDIRECT_URL` (the callback URL above). Redeploy.

## 5. Check it

Open the Vercel address: review a public PR, sign in with GitHub, ask a question on the Ask page.

## What to expect on the free tiers

- Render free services sleep after about 15 minutes idle; the first request wakes them (around a minute,
  and the backend and knowledge service wake separately). Open the site once before a demo.
- Render's free Postgres expires after 30 days (you must replace it or upgrade), and offers one per workspace,
  which is why the knowledge service uses its own schema in the same database.
- The first Ask on a repository indexes it (1 to 2 minutes with a Cohere trial key). If the Vercel proxy cuts a
  request that long, the chat shows an error; asking again works once the repository is indexed.
- The Cohere trial key has monthly call limits.
