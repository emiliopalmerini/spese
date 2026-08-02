import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router"
import { expect, it, vi } from "vitest"

import { OverviewPage } from "./overview"

it("renders loading, empty and error states", async () => {
  let resolveRequest: ((response: Response) => void) | undefined
  vi.spyOn(globalThis, "fetch").mockImplementation(() => new Promise<Response>((resolve) => { resolveRequest = resolve }))
  const view = renderOverview()
  expect(screen.getByLabelText("Caricamento panoramica")).toBeVisible()
  resolveRequest?.(new Response(JSON.stringify({ data: emptyOverview }), { status: 200, headers: { "Content-Type": "application/json" } }))
  expect(await screen.findByText("Nessuna spesa in questo mese.")).toBeVisible()

  view.unmount()
  vi.restoreAllMocks()
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ code: "failure", message: "Errore", requestId: "request" }), { status: 500, headers: { "Content-Type": "application/json" } }))
  renderOverview()
  expect(await screen.findByRole("alert")).toHaveTextContent("Panoramica non disponibile")
})

const emptyOverview = {
  month: "2026-02",
  incomeCents: 0,
  expenseCents: 0,
  recurringExpenseCents: 0,
  otherExpenseCents: 0,
  savingsCents: 0,
  netWorthCents: 0,
  attention: { uncategorized: 0, unreconciledAccounts: 0, recurringToReview: 0 },
  topCategories: null,
  accounts: null,
  drilldown: "/movimenti?from=2026-02-01&to=2026-02-28",
}

function renderOverview() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={["/?month=2026-02"]}><OverviewPage onAdd={() => undefined} /></MemoryRouter>
    </QueryClientProvider>,
  )
}
