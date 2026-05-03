// Ensemble channel binding: enable a pack with a channel configured →
// verify the channel binding shows up on the stamped instance detail page
// and persists across a reload.

const PACK = `cy-ppch-${Date.now()}`;
const PERSONA = "notifier";
const STAMPED_INSTANCE = `${PACK}-${PERSONA}`;

describe("Ensemble — channel binding", () => {
  after(() => {
    cy.deleteEnsemble(PACK);
    cy.deleteAgent(STAMPED_INSTANCE);
  });

  it("stamps a persona with a channel binding and surfaces it on instance detail", () => {
    const manifest = `apiVersion: sympozium.ai/v1alpha1
kind: Ensemble
metadata:
  name: ${PACK}
  namespace: default
spec:
  enabled: true
  description: channel binding test
  baseURL: http://host.docker.internal:1234/v1
  authRefs:
    - provider: lm-studio
      secret: ""
  agentConfigs:
    - name: ${PERSONA}
      systemPrompt: You notify via channel.
      model: ${Cypress.env("TEST_MODEL")}
      channels:
        - slack
`;
    cy.writeFile(`cypress/tmp/${PACK}.yaml`, manifest);
    cy.exec(`kubectl apply -f cypress/tmp/${PACK}.yaml`);

    cy.visit("/agents");
    cy.contains(STAMPED_INSTANCE, { timeout: 30000 })
      .should("be.visible")
      .click();

    // Navigate to the Channels tab on instance detail.
    cy.contains("button", "Channels", { timeout: 20000 }).click();
    cy.contains(/slack/i, { timeout: 20000 }).should("exist");

    // Reload — the binding must still be there (persistence).
    cy.reload();
    cy.contains("button", "Channels", { timeout: 20000 }).click();
    cy.contains(/slack/i, { timeout: 20000 }).should("exist");
  });
});

export {};
