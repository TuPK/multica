import { defineConfig } from "astro/config";

// Same port convention as the web app: 30000 is the Next.js dev server, so the
// landing page takes the next one. LANDING_PORT overrides it for `astro dev`
// and `astro preview`; the Docker image listens on 80 and Compose publishes it
// on LANDING_PORT (see docker-compose.selfhost.yml).
const port = Number(process.env.LANDING_PORT) || 30002;

export default defineConfig({
  server: { port },
  preview: { port },
  devToolbar: { enabled: false },
});
