const NS = "default";

function visitInNamespace(path: string) {
  cy.visit(path, {
    onBeforeLoad(win) {
      win.localStorage.setItem("sympozium_namespace", NS);
    },
  });
}

describe("Native Celln harness wizard", () => {
  it("skips runtime selection and pre-selects all compatible borrowed tools", () => {
    visitInNamespace("/agents?create=1&kind=agent");
    cy.get('[role="dialog"]').within(() => {
      cy.get('input[placeholder="my-agent"]').type(`celln-wizard-${Date.now()}`);
      cy.wizardNext();
      cy.get('[data-testid="create-agent-execution-environment"]').contains("button", "Celln").click();
      cy.contains("Enduring Celln parent").should("be.visible");
      // The single native runtime is implicit, so Next goes straight to tools.
      cy.wizardNext();
      cy.get('[data-testid="create-agent-borrowed-tools"]').should("be.visible");
      cy.contains("Choose a native Celln runtime").should("not.exist");
      cy.contains("label", "workspace-read@").find('input[type="checkbox"]').should("be.checked");
      cy.contains("label", "workspace-write@").find('input[type="checkbox"]').should("be.checked");
      cy.contains("label", "https-fetch@").find('input[type="checkbox"]').should("be.checked");
      cy.wizardNext();
      cy.get("#native-model").should("have.value", "deepseek-chat");
      cy.wizardNext();
      cy.get('[data-testid="execution-confirmation"]').should("contain", "Celln");
    });
  });

  it("does not list the native Celln runtime as a Kubernetes harness", () => {
    visitInNamespace("/harnesses");
    cy.contains("Harnesses").should("be.visible");
    cy.contains("pi-session-v0-84-4").should("be.visible");
    cy.contains("celln-native").should("not.exist");
  });
});
