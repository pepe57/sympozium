import { defineConfig } from "cypress";

export default defineConfig({
  e2e: {
    baseUrl: process.env.CYPRESS_BASE_URL || "http://localhost:5173",
    // Tests run against the live dev server + cluster — no mocking.
    supportFile: "cypress/support/e2e.ts",
    specPattern: "cypress/e2e/**/*.cy.ts",
    viewportWidth: 1280,
    viewportHeight: 800,
    defaultCommandTimeout: 15000,
    video: false,
  },
});
