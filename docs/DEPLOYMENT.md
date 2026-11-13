# Zero-Cost Production Deployment Guide for Obsidian

This guide walks you through deploying the complete Obsidian stack (PostgreSQL database, Go API, Background Workers, and React Dashboard) **100% free forever** using tier-1 cloud platforms.

---

## Architecture Overview

```
                      ┌──────────────────────────────────────┐
                      │        Vercel (100% Free CDN)        │
                      │     Obsidian React 18 / Vite UI      │
                      └──────────────────┬───────────────────┘
                                         │ HTTPS / WSS
                                         ▼
                      ┌──────────────────────────────────────┐
                      │       Render.com (Free Tier)         │
                      │   - obsidian-api (Web Service)       │
                      │   - obsidian-worker (Worker Service) │
                      └──────────────────┬───────────────────┘
                                         │ TLS
                                         ▼
                      ┌──────────────────────────────────────┐
                      │        Neon.tech (Free Serverless)   │
                      │          PostgreSQL 16 Engine        │
                      └──────────────────────────────────────┘
```

---

## Step 1: Create Free Serverless PostgreSQL on Neon.tech (1 min)

[Neon](https://neon.tech) offers a generous, permanent free tier with native PostgreSQL 16, branching, and zero maintenance.

1. Go to [https://neon.tech](https://neon.tech) and sign up with GitHub or Google.
2. Click **Create Project** (Name: `obsidian-prod`, Postgres Version: `16`).
3. Under **Dashboard > Connection Details**, copy your pooled connection string:
   ```bash
   postgres://username:password@ep-xyz.us-east-2.aws.neon.tech/neondb?sslmode=require
   ```
4. Run the schema migrations (Obsidian applies migrations automatically on startup via `AutoMigrate`, or you can paste the SQL files from [internal/db/migrations](file:///Users/adeshkishordeshmukh/Documents/Adesh_project/obsidian/internal/db/migrations) into the Neon SQL Editor).

---

## Step 2: Deploy Go API & Worker Fleet to Render.com (3 min)

[Render](https://render.com) provides free Docker-based Web Services and Workers.

### Option A: 1-Click Blueprint (Recommended)
1. Fork or push this repository to GitHub: `https://github.com/AdeshDeshmukh/obsidian`.
2. In the Render Dashboard, click **New > Blueprint**.
3. Select your repository. Render will automatically detect [`render.yaml`](file:///Users/adeshkishordeshmukh/Documents/Adesh_project/obsidian/render.yaml).
4. Enter your `DATABASE_URL` from Neon when prompted.
5. Click **Apply**. Render will automatically build and start `obsidian-api` and `obsidian-worker`.

### Option B: Manual Web Service
1. In Render, click **New > Web Service**.
2. Connect your repo and set:
   - **Environment:** `Docker`
   - **Dockerfile Path:** `docker/api.Dockerfile`
   - **Instance Type:** `Free`
3. Add Environment Variables:
   - `PORT`: `8080`
   - `DATABASE_URL`: *(Your Neon Connection String)*
   - `JWT_SECRET`: *(Any secure 32-character string)*
4. Once deployed, note your public API URL (e.g. `https://obsidian-api-xyz.onrender.com`).

---

## Step 3: Deploy React Dashboard to Vercel (2 min)

[Vercel](https://vercel.com) provides unlimited, lightning-fast static hosting on global edge CDNs.

1. Go to [https://vercel.com](https://vercel.com) and click **Add New > Project**.
2. Import your GitHub repository: `https://github.com/AdeshDeshmukh/obsidian`.
3. In Project Settings:
   - **Framework Preset:** `Vite`
   - **Root Directory:** Click Edit and select `dashboard`
4. Add Environment Variable:
   - `VITE_API_URL`: `https://obsidian-api-xyz.onrender.com` *(from Step 2)*
5. Click **Deploy**.
6. Your live dashboard is ready at `https://obsidian-dashboard-xyz.vercel.app`!

---

## Alternative Free Platforms

* **Cloudflare Pages (Frontend):** 100% free with unlimited bandwidth. Connect `dashboard/` with build command `npm run build` and output directory `dist`.
* **Koyeb (Backend):** Free micro container instance running Docker with zero sleep timeouts.
* **Fly.io (Backend):** Free tier allows up to 3 shared CPU VMs using `fly launch`.

