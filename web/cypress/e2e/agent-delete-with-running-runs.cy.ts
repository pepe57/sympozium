// Delete an instance while it has a run that has recently completed.
// Verify the instance disappears from the list and the associated runs
// are either removed or remain visible with the agentRef intact
// (no broken UI state).

const INSTANCE = `cy-delrun-${Date.now()}`;
let RUN_NAME = "";

describe("Instance delete — with recent runs", () => {
  before(() => {
    cy.createLMStudioAgent(INSTANCE);
    cy.dispatchRun(INSTANCE, "Reply with exactly: DELETE_OK").then((name) => {
      RUN_NAME = name;
    });
    cy.then(() => cy.waitForRunTerminal(RUN_NAME));
  });

  after(() => {
    if (RUN_NAME) cy.deleteRun(RUN_NAME);
    cy.deleteAgent(INSTANCE);
  });

  it("deletes instance cleanly and removes it from the list", () => {
    cy.visit("/agents");
    cy.contains(INSTANCE, { timeout: 20000 }).should("be.visible");

    cy.deleteAgent(INSTANCE);

    cy.visit("/agents");
    cy.contains(INSTANCE, { timeout: 20000 }).should("not.exist");

    // Runs page should still render without errors, and the old run row
    // should NOT cause a broken UI (either gone, or shown with "orphan" ref).
    cy.visit("/runs");
    cy.get("table", { timeout: 20000 }).should("exist");
  });
});

export {};
