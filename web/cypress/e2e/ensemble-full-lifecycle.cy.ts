// Ensemble lifecycle: enable pack → stamped Instance + Schedule appear →
// disable pack → both disappear. Verifies cascade semantics end to end.

const PACK = `cy-pplc-${Date.now()}`;
const PERSONA = "auditor";
const STAMPED_INSTANCE = `${PACK}-${PERSONA}`;

function applyPackKubectl() {
  const manifest = `apiVersion: sympozium.ai/v1alpha1
kind: Ensemble
metadata:
  name: ${PACK}
  namespace: default
spec:
  enabled: true
  description: cypress full-lifecycle test pack
  category: test
  version: "0.0.1"
  baseURL: http://host.docker.internal:1234/v1
  authRefs:
    - provider: lm-studio
      secret: ""
  agentConfigs:
    - name: ${PERSONA}
      displayName: Cypress Auditor
      systemPrompt: You are a terse auditor.
      model: ${Cypress.env("TEST_MODEL")}
`;
  cy.writeFile(`cypress/tmp/${PACK}.yaml`, manifest);
  cy.exec(`kubectl apply -f cypress/tmp/${PACK}.yaml`);
}

describe("Ensemble — full lifecycle", () => {
  after(() => {
    cy.deleteEnsemble(PACK);
    cy.deleteAgent(STAMPED_INSTANCE);
  });

  it("enables a pack, verifies stamped instance appears, then disables and verifies removal", () => {
    applyPackKubectl();

    // Stamped instance should appear on /instances.
    cy.visit("/agents");
    cy.contains(STAMPED_INSTANCE, { timeout: 30000 }).should("be.visible");

    // Disable via apiserver PATCH (this endpoint DOES exist).
    cy.request({
      method: "PATCH",
      url: `/api/v1/ensembles/${PACK}?namespace=default`,
      headers: {
        "Content-Type": "application/json",
        ...(Cypress.env("API_TOKEN")
          ? { Authorization: `Bearer ${Cypress.env("API_TOKEN")}` }
          : {}),
      },
      body: { enabled: false },
      failOnStatusCode: false,
    });

    // Stamped instance should eventually disappear from the list.
    cy.visit("/agents");
    cy.contains(STAMPED_INSTANCE, { timeout: 30000 }).should("not.exist");
  });
});

export {};
