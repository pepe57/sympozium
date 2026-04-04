// Support file — loaded before every spec.

// ── Auth: inject API token via visit callback ───────────────────────────────
// Overrides cy.visit to inject the token into localStorage before the app
// reads it. Token is read from CYPRESS_API_TOKEN env var.
Cypress.Commands.overwrite("visit", (originalFn, url, options) => {
  const token = Cypress.env("API_TOKEN");
  if (!token) return originalFn(url, options);

  const opts = { ...options } as Cypress.VisitObject;
  const originalOnBeforeLoad = opts.onBeforeLoad;
  opts.onBeforeLoad = (win) => {
    win.localStorage.setItem("sympozium_token", token);
    win.localStorage.setItem("sympozium_namespace", "default");
    if (originalOnBeforeLoad) originalOnBeforeLoad(win);
  };
  return originalFn(url, opts);
});

// ── Custom commands ─────────────────────────────────────────────────────────
declare global {
  namespace Cypress {
    interface Chainable {
      /** Click the Next button in the onboarding wizard. */
      wizardNext(): Chainable<void>;
      /** Click the Back button in the onboarding wizard. */
      wizardBack(): Chainable<void>;
      /** Delete an instance by name via API (cleanup helper). */
      deleteInstance(name: string): Chainable<void>;
    }
  }
}

Cypress.Commands.add("wizardNext", () => {
  cy.contains("button", "Next").should("not.be.disabled").click({ force: true });
});

Cypress.Commands.add("wizardBack", () => {
  cy.contains("button", "Back").click({ force: true });
});

Cypress.Commands.add("deleteInstance", (name: string) => {
  const token = Cypress.env("API_TOKEN");
  cy.request({
    method: "DELETE",
    url: `/api/v1/instances/${name}?namespace=default`,
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    failOnStatusCode: false,
  });
});

export {};
