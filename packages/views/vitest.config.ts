import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./test/setup.ts"],
    include: ["**/*.test.{ts,tsx}"],
    // The full run shares CPUs across ~11 jsdom workers, and the first render
    // of a heavy component can take well over vitest's 5s default under that
    // load (each file passes alone in about 2s). The per-test budget absorbs
    // that; a real hang still fails, just later.
    testTimeout: 20_000,
    hookTimeout: 20_000,
  },
});
