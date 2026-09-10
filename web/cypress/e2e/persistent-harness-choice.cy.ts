// Exercise the installed catalogue without creating agents or model calls.
const NS = "default";

function visitInNamespace(path: string) {
  cy.visit(path, {
    onBeforeLoad(win) {
      win.localStorage.setItem("sympozium_namespace", NS);
    },
  });
}

describe("Create menu: Harness and Run", () => {
  it("explains the difference and opens the Harness wizard", () => {
    visitInNamespace("/");
    cy.get("header").contains("button", "Create").click();
    cy.get('[role="dialog"]').within(() => {
      cy.contains("Harness").should("be.visible");
      cy.contains("Ongoing work that keeps its context").should("be.visible");
      cy.contains("One-shot work that finishes and exits").should("be.visible");
      cy.contains("button", "Harness").click();
    });
    cy.get('[role="dialog"]').within(() => {
      cy.get('input[placeholder="my-agent"]').should("be.visible").type("create-menu-check");
      cy.contains("button", "Next").click();
      cy.get('[data-testid="create-agent-execution-environment"]').should("be.visible");
      cy.contains("Execution plane").should("be.visible");
    });
  });

  it("offers Pi and Hermes after choosing the Kubernetes plane", () => {
    visitInNamespace("/agents?create=1&kind=harness");
    cy.get('[role="dialog"]').within(() => {
      cy.get('input[placeholder="my-agent"]').type("persistent-choice-check");
      cy.wizardNext();
      cy.get('[data-testid="create-agent-execution-environment"]').contains("button", "Kubernetes").click();
      cy.wizardNext();
      cy.contains("Choose a persistent harness").should("be.visible");
      cy.get('[role="combobox"]').click();
    });
    cy.get('[role="option"]').should("have.length", 2);
    cy.get('[role="option"]').each(($option) => {
      expect($option.text()).to.match(/^(Pi|Hermes) — persistent chat$/);
    });
    cy.contains('[role="option"]', "Pi — persistent chat").click();
    cy.get('[role="dialog"]').within(() => {
      cy.contains("Harness selected: Pi.").should("be.visible");
      cy.contains("persistent Kubernetes session").should("be.visible");
      cy.contains("button", "Next").click();
      cy.contains("Select SkillPacks to attach").should("be.visible");
    });
  });

  it("keeps one-shot Run on the provider-first wizard with SkillPacks", () => {
    visitInNamespace("/agents?create=1&kind=run");
    cy.get('[role="dialog"]').within(() => {
      cy.get('input[placeholder="my-agent"]').type("one-shot-check");
      cy.wizardNext();
      cy.contains("AI Provider").should("be.visible");
      cy.contains("Choose a persistent harness").should("not.exist");
      cy.contains("Execution plane").should("not.exist");
    });
  });

  it("clears harness-incompatible skills so Next is never blocked", () => {
    visitInNamespace("/agents?create=1&kind=harness");
    cy.get('[role="dialog"]').within(() => {
      cy.get('input[placeholder="my-agent"]').type("incompatible-skill-check");
      cy.wizardNext();
      cy.get('[data-testid="create-agent-execution-environment"]').contains("button", "Kubernetes").click();
      cy.wizardNext();
      cy.get('[role="combobox"]').click();
    });
    cy.contains('[role="option"]', "Pi — persistent chat").click();
    cy.get('[role="dialog"]').within(() => {
      cy.wizardNext();
      cy.contains("Select SkillPacks to attach").should("be.visible");
      // llmfit requires host access and is removed once the harness is chosen.
      cy.contains("button", "llmfit").should("be.disabled").and("not.contain", "Selected");
      cy.contains("2 skills selected").should("be.visible");
      cy.contains("button", "Next").should("be.enabled");
    });
  });
});
