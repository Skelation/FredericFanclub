# Frederic Fanclub 🚀

This is a fully vibecoded project.
This is the official codebase for the Frederic Fanclub website and betting platform. It features a static frontend UI and a Go backend powered by SQLite, Docker, and Discord OAuth.

## 📁 Repository Structure

This project is a **Monorepo**, meaning the frontend and backend live in the same repository but operate completely independently.

```text
frederic-fanclub/
├── frontend/             # HTML, CSS, JS, Audio, and Images (The Waiter)
├── server/               # Go API, Docker configurations, and Databases (The Kitchen)
├── .gitignore            # Protects secrets and databases from being uploaded
└── .env.example          # Safe blueprint for environment variables
```

---

## 🔐 Environment Variables

For security, API keys and databases are **never** pushed to GitHub. You must create your own `.env` files locally.

Use the provided `.env.example` to create two files inside your `server/` folder:

1. **`.env`** (For Local Development)
   * `PORT=8080`
   * `DB_PATH=./fred-dev.db`
   * `FRONTEND_URL=http://localhost:3000`
   * `CORS_ORIGINS=http://localhost:3000,http://127.0.0.1:3000`
   * `DISCORD_REDIRECT_URI=http://localhost:8080/api/auth/discord/callback`

2. **`.env.prod`** (For the Live Docker Vault)
   * `PORT=8080`
   * `DB_PATH=./fred.db`
   * `FRONTEND_URL=https://fredericfan.club`
   * `CORS_ORIGINS=https://fredericfan.club,https://www.fredericfan.club`
   * `DISCORD_REDIRECT_URI=https://api.fredericfan.club/api/auth/discord/callback`

---

## 🛠️ Development Environment (Local Sandbox)

Use this setup to test new features without affecting the live casino or real user balances. It runs on a completely separate test database (`fred-dev.db`).

**1. Start the Backend (Port 8080)**
Open a terminal in the `server/` folder and run:
```bash
go run .
```

**2. Start the Frontend (Port 3000)**
Open a second terminal in the `frontend/` folder and run:
```bash
npm start
```
*Your browser will open `localhost:3000`. The frontend will automatically detect the local environment and route API calls to your local Go server.*

---

## 🌍 Production Environment (Live Vault)

The live environment uses **Docker** to containerize the app. To prevent port collisions with your dev environment, the live backend maps to **Port 8081**, and the live frontend uses Nginx on **Port 80**.

**1. Start the Live Backend (Docker Go Server)**
Open a terminal in the `server/` folder and run:
```bash
# Stop any old instances
docker stop fred-live
docker rm fred-live

# Rebuild and run
docker build -t fred-casino .
docker run -d --name fred-live -p 8081:8080 --env-file .env.prod -v "${PWD}/fred.db:/app/fred.db" -v "${PWD}/data:/app/data" fred-casino
```

**2. Start the Live Frontend (Docker Nginx)**
Open a terminal in the **root** folder and run:
```bash
# Stop any old instances
docker stop fred-live-frontend
docker rm fred-live-frontend

# Run Nginx (Replace the C:\... path with your exact path to the frontend folder)
docker run -d --name fred-live-frontend -p 80:80 -v "C:\YOUR\EXACT\PATH\frontend:/usr/share/nginx/html" nginx:alpine
```

**3. Cloudflare Routing**
Ensure your Cloudflare Tunnels are pointing to the correct local ports:
* `fredericfan.club` ➔ `http://localhost:80`
* `api.fredericfan.club` ➔ `http://localhost:8081`