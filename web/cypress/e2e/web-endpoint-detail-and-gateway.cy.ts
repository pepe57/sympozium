// Test: verify web-endpoint skill surfaces correctly on the instance detail
// page (Web Endpoint tab) and on the Gateway routes card.

const INSTANCE = `cy-webep-detail-${Date.now()}`;

describe("Web-endpoint — detail page and gateway", () => {
  before(() => {
    cy.createLMStudioInstance(INSTANCE, { skills: ["web-endpoint"] });
    cy.wait(2000);
  });

  after(() => {
    cy.deleteInstance(INSTANCE);
  });

  it("shows the Web Endpoint tab with skill config", () => {
    cy.visit(`/instances/${INSTANCE}`);

    // The Web Endpoint tab should exist.
    cy.contains("button", "Web Endpoint", { timeout: 20000 }).click();

    // Tab content shows rate limit and hostname fields.
    cy.contains("Rate Limit").should("be.visible");
    cy.contains("60 req/min").should("be.visible");
    cy.contains("auto from gateway").should("be.visible");
  });

  it("instance without web-endpoint shows disabled message", () => {
    // Create a bare instance (no skills).
    const BARE = `cy-bare-${Date.now()}`;
    cy.createLMStudioInstance(BARE);
    cy.wait(2000);

    cy.visit(`/instances/${BARE}`);
    cy.contains("button", "Web Endpoint", { timeout: 20000 }).click();
    cy.contains("Web endpoint is not enabled").should("be.visible");

    cy.deleteInstance(BARE);
  });

  it("instance appears in the Gateway routes card", () => {
    cy.visit("/gateway");

    // Routes card may be below the fold — scroll to it.
    cy.contains("Routes", { timeout: 20000 }).scrollIntoView().should("be.visible");
    cy.contains(INSTANCE, { timeout: 20000 }).scrollIntoView().should("be.visible");
  });
});

export {};
