const NS = "default";

function visitInNamespace(path: string) {
  cy.visit(path, {
    onBeforeLoad(win) {
      win.localStorage.setItem("sympozium_namespace", NS);
    },
  });
}

describe("Native Celln harness wizard", () => {
  it("offers the native runtime and pre-selects all compatible borrowed tools", () => {
    visitInNamespace("/agents?create=1&kind=harness");
    cy.get('[role="dialog"]').within(() => {
      cy.get('input[placeholder="my-agent"]').type(`celln-wizard-${Date.now()}`);
      cy.wizardNext();
      cy.get('[data-testid="create-agent-execution-environment"]').contains("button", "Celln").click();
      cy.contains("Enduring Celln parent").should("be.visible");
      cy.wizardNext();
      cy.contains("Choose a native Celln runtime").should("be.visible");
      cy.get('[role="combobox"]').click();
    });
    cy.contains('[role="option"]', "celln-native").click();
    cy.get('[role="dialog"]').within(() => {
      cy.wizardNext();
      cy.get('[data-testid="create-agent-borrowed-tools"]').should("be.visible");
      cy.contains("label", "workspace-read@").find('input[type="checkbox"]').should("be.checked");
      cy.contains("label", "workspace-write@").find('input[type="checkbox"]').should("be.checked");
      cy.contains("label", "https-fetch@").find('input[type="checkbox"]').should("be.checked");
      cy.wizardNext();
      cy.get("#native-model").should("have.value", "deepseek-chat");
      cy.wizardNext();
      cy.get('[data-testid="execution-confirmation"]').should("contain", "Celln");
    });
  });
});
