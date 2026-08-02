import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { expect, it, vi } from "vitest"

import { ReconciliationSheet } from "./accounts"

const account = { id: "account-1", name: "Conto principale", type: "Asset", class: "Cash", currency: "EUR", initialBalanceCents: 0, initialDate: "2026-01-01", version: 1 } as const

it("previews and commits a partial reconciliation", async () => {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    const body = JSON.parse(String(init?.body)) as { period?: string; accounts?: unknown[]; id?: string }
    if (body.period) {
      return response({ data: { id: "batch-1", period: "2026-02", closedThrough: "2026-02-28", accounts: [{ accountId: account.id, expectedBalanceCents: 10000, actualBalanceCents: 10200, differenceCents: 200 }] } })
    }
    expect(body.id).toBe("batch-1")
    return response({ data: body }, 201)
  })
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <ReconciliationSheet accounts={[account]} month="2026-02" onOpenChange={onOpenChange} open />
    </QueryClientProvider>,
  )
  await user.type(screen.getByLabelText("Conto principale"), "102,00")
  await user.click(screen.getByRole("button", { name: "Mostra anteprima" }))
  expect(await screen.findByText(/Differenza 2,00/)).toBeVisible()
  await user.click(screen.getByRole("button", { name: "Conferma" }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

function response(body: unknown, status = 200): Promise<Response> {
  return Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }))
}
