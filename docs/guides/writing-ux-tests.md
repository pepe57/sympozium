# Writing UX Tests (Cypress)

Cypress specs in `web/cypress/e2e/` verify the React web UI end-to-end
against a running Sympozium cluster. They cover wizard flows, list/detail
pages, run dispatching, deletion, ensemble lifecycle, and regression
guards for UX-visible bugs.

## Prerequisites

- Kind (or other) cluster with Sympozium installed (`make install`).
- `kubectl` pointing at that cluster.
- Node dependencies installed: `make web-install` (or `cd web && npm install`).
- Optional but recommended for live LLM scenarios: **LM Studio running at
  `host.docker.internal:1234`** with at least one model loaded (the
  default specs target `qwen/qwen3.5-9b`).

## Running the Tests

There are two supported flows — pick whichever matches the server you
have running:

### A) Vite dev server flow

Best for active frontend development with hot reload.

```bash
make web-dev-serve     # terminal 1: vite on :5173 + port-forward apiserver
make ux-tests          # terminal 2: headless Cypress run
make ux-tests-open     #   …or launch the interactive Cypress runner
```

### B) `sympozium serve` flow

Best if you already have `sympozium serve` running against an installed
cluster (serves the embedded UI from the in-cluster apiserver).

```bash
sympozium serve             # terminal 1: port-forwards apiserver → :9090
make ux-tests-serve         # terminal 2: headless, against :9090
make ux-tests-serve-open    #   …or interactive
```

If you run `sympozium serve --port 8090`, pass the matching port to make:

```bash
make ux-tests-serve SERVE_PORT=8090
```

Both targets auto-retrieve the API token from the `sympozium-ui-token`
secret in the `sympozium-system` namespace and wire it into Cypress via
`CYPRESS_API_TOKEN`.

## What Gets Tested

| Area | Spec |
|---|---|
| Instance wizard (adhoc + LM Studio) | `instance-create-adhoc.cy.ts`, `instance-create-lmstudio.cy.ts`, `instance-multi-run-lmstudio.cy.ts` |
| Ensemble activation + lifecycle | `ensemble-enable.cy.ts`, `ensemble-full-lifecycle.cy.ts`, `ensemble-channel-bind.cy.ts` |
| Run detail regression guards | `run-detail-response-visible.cy.ts`, `run-thinking-indicator.cy.ts` |
| Deletion flows | `instance-delete-with-running-runs.cy.ts`, `run-delete-and-disappear.cy.ts`, `schedule-delete.cy.ts` |
| Runs list | `runs-filter-and-sort.cy.ts` |
| Schedules | `schedule-create-via-ui.cy.ts`, `schedule-pause-resume.cy.ts` |
| Auxiliary pages | `skills-catalog.cy.ts`, `policies-view.cy.ts`, `mcp-server-add.cy.ts`, `login-flow.cy.ts` |

## Writing a New Spec

Specs live in `web/cypress/e2e/<name>.cy.ts`. The support file at
`web/cypress/support/e2e.ts` provides shared helpers that remove a lot of
boilerplate:

| Helper | Purpose |
|---|---|
| `cy.createLMStudioInstance(name)` | POST to `/api/v1/instances` with an LM Studio + qwen3.5-9b config |
| `cy.dispatchRun(instanceRef, task)` | POST to `/api/v1/runs`; resolves with the created AgentRun name |
| `cy.waitForRunTerminal(runName)` | Polls `/api/v1/runs/:name` until `status.phase` is `Succeeded` or `Failed` |
| `cy.waitForDeleted(path)` | Polls until a GET returns 404 (handles finalizer delays) |
| `cy.deleteInstance(name)` / `cy.deleteRun(name)` / `cy.deleteSchedule(name)` / `cy.deleteEnsemble(name)` | API-level cleanup helpers |
| `cy.wizardNext()` / `cy.wizardBack()` | Click Next/Back buttons in onboarding wizards |

Minimal template for a live-cluster spec:

```ts
const INSTANCE = `cy-myspec-${Date.now()}`;
let RUN = "";

describe("My feature", () => {
  before(() => {
    cy.createLMStudioInstance(INSTANCE);
  });

  after(() => {
    if (RUN) cy.deleteRun(RUN);
    cy.deleteInstance(INSTANCE);
  });

  it("does the thing", () => {
    cy.dispatchRun(INSTANCE, "reply: HELLO").then((name) => {
      RUN = name;
    });
    cy.then(() => cy.waitForRunTerminal(RUN));
    cy.visit(`/runs/${RUN}`);
    cy.contains("HELLO", { timeout: 20000 }).should("be.visible");
  });
});

export {};
```

### Conventions

- **End every spec with `export {};`** so each file is a TS module and
  its top-level `const`s don't collide with sibling specs under `tsc`.
- **Use lowercase resource names** — Kubernetes rejects uppercase in
  object names (RFC 1123 subdomain rules).
- **Prefer `cy.waitForDeleted()` over direct 404 assertions** — finalizers
  can delay GC and a naïve `expect(200).to.eq(404)` is racey.
- **For operations without an apiserver endpoint** (e.g. schedule
  suspend, Ensemble creation), fall back to `cy.exec("kubectl ...")`
  via manifests written with `cy.writeFile("cypress/tmp/…yaml", …)`.
- **Don't block on the "thinking" indicator inside tight loops** — short
  tasks may finish before Cypress can observe the transient phase.

### Token Injection

`web/cypress/support/e2e.ts` overrides `cy.visit` to set
`localStorage.sympozium_token` from `CYPRESS_API_TOKEN` before the app
boots, so your specs can assume the user is authenticated. If you need
to test the unauthenticated path (see `login-flow.cy.ts`), pass an
`onBeforeLoad` hook that calls `win.localStorage.removeItem(
"sympozium_token")` — it runs after the token-injecting override, so
your removal wins.

## Troubleshooting

### `Error: Cannot find module 'cypress'`

Cypress is in `package.json` but `node_modules/` is stale. Fix:

```bash
make web-install
# or: cd web && npm install
```

### `nothing is listening on localhost:<port>`

The preflight check (`hack/check-ux-backend.sh`) couldn't reach
`/api/v1/namespaces` at the expected port. Either no dev server is up,
or a previous port-forward died and its local listener is still held
by a zombie process:

```bash
# inspect who is on the port
lsof -i :5173 -P -n          # vite
lsof -i :9090 -P -n          # sympozium serve
# kill if stale
kill $(lsof -t -i :5173) 2>/dev/null || true
```

### LM Studio-dependent specs hang

Some specs dispatch real AgentRuns (`run-detail-*`, `instance-delete-*`,
etc.). They need LM Studio reachable from inside Kind as
`host.docker.internal:1234` and a `NetworkPolicy` that allows egress on
port 1234. If your agent pods are failing to reach LM Studio, the
integration test `test/integration/test-lmstudio-response-regression.sh`
has the exact NetworkPolicy patch you need.

### "name, provider, and model are required"

Your helper is sending the wrong JSON shape. The apiserver's
`POST /api/v1/instances` expects flat top-level fields:

```json
{ "name": "…", "provider": "lm-studio", "model": "…", "baseURL": "…" }
```

Use `cy.createLMStudioInstance(name)` which handles this correctly.

## Adding a New Helper

Extend `web/cypress/support/e2e.ts`:

```ts
declare global {
  namespace Cypress {
    interface Chainable {
      myHelper(arg: string): Chainable<void>;
    }
  }
}

Cypress.Commands.add("myHelper", (arg: string) => {
  // …
});
```

Run `cd web/cypress && npx tsc --noEmit` to type-check.
