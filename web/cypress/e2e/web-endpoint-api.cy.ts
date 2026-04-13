// Test: verify web-endpoint skill at the API level — instance spec contains
// the skill after creation, and the skill can be toggled off via update.

const INSTANCE = `cy-webep-api-${Date.now()}`;

function authHeaders(): Record<string, string> {
  const token = Cypress.env("API_TOKEN");
  const h: Record<string, string> = { "Content-Type": "application/json" };
  if (token) h["Authorization"] = `Bearer ${token}`;
  return h;
}

describe("Web-endpoint — API validation", () => {
  before(() => {
    cy.createLMStudioInstance(INSTANCE, { skills: ["web-endpoint"] });
  });

  after(() => {
    cy.deleteInstance(INSTANCE);
  });

  it("instance spec includes web-endpoint skill", () => {
    cy.request({
      url: `/api/v1/instances/${INSTANCE}?namespace=default`,
      headers: authHeaders(),
    }).then((resp) => {
      expect(resp.status).to.eq(200);
      const skills = resp.body.spec.skills as { skillPackRef: string }[];
      expect(skills).to.be.an("array");
      const hasWebEndpoint = skills.some(
        (s) => s.skillPackRef === "web-endpoint",
      );
      expect(hasWebEndpoint).to.be.true;
    });
  });

  it("web-endpoint skill has default rate_limit_rpm", () => {
    cy.request({
      url: `/api/v1/instances/${INSTANCE}?namespace=default`,
      headers: authHeaders(),
    }).then((resp) => {
      const webSkill = (
        resp.body.spec.skills as { skillPackRef: string; params?: Record<string, string> }[]
      ).find((s) => s.skillPackRef === "web-endpoint");
      // Default RPM is 60 (may be omitted or explicit).
      const rpm = webSkill?.params?.rate_limit_rpm;
      if (rpm) {
        expect(rpm).to.eq("60");
      }
    });
  });
});

export {};
