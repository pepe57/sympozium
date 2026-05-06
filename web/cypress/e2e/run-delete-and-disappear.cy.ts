// Delete a run — verify it is removed from /runs AND direct URL returns
// 404 via the API. Guard against phantom rows that survive deletion.

const INSTANCE = `cy-rundel-${Date.now()}`;
let RUN_NAME = "";

function authHeaders(): Record<string, string> {
  const token = Cypress.env("API_TOKEN");
  const h: Record<string, string> = { "Content-Type": "application/json" };
  if (token) h["Authorization"] = `Bearer ${token}`;
  return h;
}

describe("Run — delete", () => {
  before(() => {
    cy.createLMStudioAgent(INSTANCE);
    cy.dispatchRun(INSTANCE, "Reply with exactly: DEL_OK").then((name) => {
      RUN_NAME = name;
    });
    cy.then(() => cy.waitForRunTerminal(RUN_NAME));
  });

  after(() => {
    cy.deleteAgent(INSTANCE);
  });

  it("removes the run from the list and returns 404 on direct GET", () => {
    cy.visit("/runs");
    cy.contains(RUN_NAME, { timeout: 20000 }).scrollIntoView().should("be.visible");

    cy.deleteRun(RUN_NAME);

    cy.waitForDeleted(`/api/v1/runs/${RUN_NAME}?namespace=default`);

    cy.visit("/runs");
    cy.contains(RUN_NAME, { timeout: 20000 }).should("not.exist");
  });
});

export {};
