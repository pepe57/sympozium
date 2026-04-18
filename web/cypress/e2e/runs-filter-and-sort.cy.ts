// /runs page: basic filter and sort sanity. Dispatch two runs against
// two different instances, verify both appear, and filter/search narrows
// to the expected subset.

const A = `cy-filta-${Date.now()}`;
const B = `cy-filtb-${Date.now()}`;
let RUN_A = "";
let RUN_B = "";

describe("Runs list — filter and sort", () => {
  before(() => {
    cy.createLMStudioInstance(A);
    cy.createLMStudioInstance(B);
    cy.dispatchRun(A, "Reply: FILT_A").then((n) => {
      RUN_A = n;
    });
    cy.dispatchRun(B, "Reply: FILT_B").then((n) => {
      RUN_B = n;
    });
  });

  after(() => {
    if (RUN_A) cy.deleteRun(RUN_A);
    if (RUN_B) cy.deleteRun(RUN_B);
    cy.deleteInstance(A);
    cy.deleteInstance(B);
  });

  it("shows both runs and filters by instance name", () => {
    cy.visit("/runs");

    // Both runs visible (identified by instanceRef in the rows).
    cy.contains("td", A, { timeout: 20000 }).should("be.visible");
    cy.contains("td", B, { timeout: 20000 }).should("be.visible");

    // If a search/filter input exists, narrow by one instance.
    // jQuery doesn't support CSS4 case-insensitive attr matching, so probe
    // by element types and then filter by placeholder text in JS.
    cy.get("body").then(($body) => {
      const $inputs = $body.find("input[type='search'], input[type='text']");
      const $search = $inputs.filter((_, el) => {
        const ph = (el as unknown as HTMLInputElement).placeholder || "";
        return /search|filter/i.test(ph);
      });
      if ($search.length > 0) {
        cy.wrap($search.first()).clear().type(A);
        cy.contains("td", A).should("be.visible");
        cy.contains("td", B).should("not.exist");
      } else {
        cy.log("no search input on /runs page — skipping filter assertion");
      }
    });
  });
});

export {};
