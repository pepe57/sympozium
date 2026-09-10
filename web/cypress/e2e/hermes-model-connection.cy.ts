// Live UI/API integration: no intercepted/fabricated provider responses.
const name = `hermes-connection-ui-${Date.now()}`;
const model = Cypress.env("TEST_MODEL");
const endpoint = `${Cypress.env("LLAMA_SERVER_URL") || "http://192.168.1.237:8080/v1"}/chat/completions`;
function request(method: string, path: string, body?: object) {
  return cy.request({ method, url: `/api/v1/${path}?namespace=default`, body,
    headers: { Authorization: `Bearer ${Cypress.env("API_TOKEN")}` }, timeout: 300000, log: false, failOnStatusCode: false });
}

describe("Persistent Hermes model connections", () => {
  it("creates a llama-server connection in the wizard and preserves it in YAML and the Agent", () => {
    cy.visit("/agents?create=1&kind=agent");
    cy.get('input[placeholder="my-agent"]').type(name);
    cy.wizardNext();
    cy.get('[data-testid="create-agent-execution-environment"]').contains("button", "Kubernetes").click();
    cy.wizardNext();
    cy.get('[role="dialog"]').find('[role="combobox"]').click();
    cy.get('[role="option"]').contains("Hermes — persistent chat").first().click();
    cy.wizardNext();
    cy.get('[role="dialog"]').then(($dialog) => { const checked = $dialog.find('input[type="checkbox"]:checked'); if (checked.length) cy.wrap(checked).uncheck({ force: true }); });
    cy.wizardNext();
    cy.contains("button", "Add connection").click();
    cy.get("#connection-name").type(name);
    cy.get("#connection-provider").type("llama-server");
    cy.get("#connection-endpoint").type(endpoint);
    cy.get("#connection-models").type(model);
    cy.contains("button", "Save connection").click();
    cy.get('[data-testid="add-model-connection"]').should("not.exist");
    cy.get('[aria-label="Model connection"]').should("contain", name);
    cy.wizardNext();
    cy.get("#native-model").should("contain", model);
    cy.contains("API Key").should("not.exist");
    cy.wizardNext(); // heartbeat
    cy.wizardNext(); // channels
    cy.wizardNext(); // confirmation
    cy.contains("button", "YAML").click();
    cy.get('[role="dialog"]').last().should("contain", `modelConnectionRef: ${name}`).and("contain", "backend: job");
    cy.get("body").type("{esc}");
    cy.intercept("POST", "/api/v1/agents*").as("createAgent");
    cy.get('[role="dialog"]').contains("button", /Create\s*$/).click();
    cy.wait("@createAgent").its("response.statusCode").should("be.oneOf", [200, 201]);
    cy.get('[role="dialog"]').should("not.exist");
    request("GET", `agents/${name}`).then(({ body }) => {
      expect(body.spec.execution.modelConnectionRef).to.equal(name);
      expect(body.spec.agents.default.model).to.equal(model);
      expect(body.spec.authRefs || []).to.have.length(0);
    });
    request("GET", "harness-sessions").then(({status, body}) => { expect(status).to.equal(200); expect(body.some((session: {metadata:{name:string}}) => session.metadata.name === `${name}-chat`)).to.equal(true); });
    cy.screenshot("hermes-model-connection-created");
    cy.writeFile("/tmp/hermes-model-connection-ui-result.json", { agent: name, connection: name, model, endpoint, passed: true });
  });
});
