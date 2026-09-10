// Live UI/API integration: no intercepted/fabricated provider responses.
const name = `hermes-connection-ui-${Date.now()}`;
const model = Cypress.env("TEST_MODEL") || "Qwen3.8-27B-UD-Q4_K_XL.gguf";
const baseURL = Cypress.env("LLAMA_SERVER_URL") || "http://192.168.1.237:8080/v1";
const connection = `${name}-connection`;
function request(method: string, path: string, body?: object) {
  return cy.request({ method, url: `/api/v1/${path}?namespace=default`, body,
    headers: { Authorization: `Bearer ${Cypress.env("API_TOKEN")}` }, timeout: 300000, log: false, failOnStatusCode: false });
}

describe("Persistent Hermes model connections", () => {
  after(() => {
    request("DELETE", `harness-sessions/${name}-chat`);
    cy.deleteAgent(name);
  });

  it("persists Provider -> Auth -> Model as a connection wired into the Agent", () => {
    cy.visit("/agents?create=1&kind=agent");
    cy.get('input[placeholder="my-agent"]').type(name);
    cy.wizardNext(); // execution plane
    cy.get('[data-testid="create-agent-execution-environment"]').contains("button", "Kubernetes").click();
    cy.wizardNext(); // runtime
    cy.get('[role="dialog"]').find('[role="combobox"]').first().click();
    cy.get('[role="option"]').contains("Hermes").first().click();
    cy.wizardNext(); // skills
    cy.get('[role="dialog"]').then(($dialog) => { const checked = $dialog.find('input[type="checkbox"]:checked'); if (checked.length) cy.wrap(checked).uncheck({ force: true }); });
    cy.wizardNext(); // provider
    // Provider -> Auth -> Model mirrors the run flow; no separate connection form.
    cy.get('[role="dialog"]').should("not.contain", "Add connection");
    cy.get('[role="dialog"]').find('button[role="combobox"]').first().click({ force: true });
    cy.get('[data-radix-popper-content-wrapper]').contains("llama-server").click({ force: true });
    cy.contains("label", "Base URL").parent().find("input").clear().type(baseURL);
    cy.wizardNext(); // auth (keyless local server)
    cy.wizardNext(); // model
    cy.get('[role="dialog"]').find("input[placeholder='gpt-4o']").clear().type(model);
    cy.wizardNext(); // heartbeat
    cy.wizardNext(); // channels
    cy.wizardNext(); // confirmation
    cy.intercept("POST", "/api/v1/model-connections*").as("createConnection");
    cy.intercept("POST", "/api/v1/agents*").as("createAgent");
    cy.get('[role="dialog"]').contains("button", /Create\s*$/).click();
    cy.wait("@createConnection").its("response.statusCode").should("be.oneOf", [200, 201]);
    cy.wait("@createAgent").its("response.statusCode").should("be.oneOf", [200, 201]);
    cy.get('[role="dialog"]').should("not.exist");

    request("GET", `agents/${name}`).then(({ body }) => {
      expect(body.spec.execution.modelConnectionRef).to.equal(connection);
      expect(body.spec.agents.default.model).to.equal(model);
      expect(body.spec.authRefs || []).to.have.length(0);
    });
    request("GET", "model-connections").then(({ status, body }) => {
      expect(status).to.equal(200);
      const created = body.find((item: { metadata: { name: string } }) => item.metadata.name === connection);
      expect(created, "connection persisted").to.exist;
      expect(created.spec.provider).to.equal("llama-server");
      expect(created.spec.models).to.deep.equal([model]);
    });
    request("GET", "harness-sessions").then(({ status, body }) => {
      expect(status).to.equal(200);
      expect(body.some((session: { metadata: { name: string } }) => session.metadata.name === `${name}-chat`)).to.equal(true);
    });
    cy.screenshot("hermes-model-connection-created");
    cy.writeFile("/tmp/hermes-model-connection-ui-result.json", { agent: name, connection, model, baseURL, passed: true });
  });
});
