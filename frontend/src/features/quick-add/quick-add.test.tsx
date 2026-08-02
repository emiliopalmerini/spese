import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { axe } from "jest-axe"
import { expect, it, vi } from "vitest"

import { QuickAdd } from "./quick-add"

const account = { id: "account-1", name: "Conto principale", type: "Asset", class: "Cash", currency: "EUR", initialBalanceCents: 0, initialDate: "2026-01-01", version: 1 }
const category = { id: "category-1", kind: "expense", name: "Alimentari", icon: "basket", color: "#2E6F95", sortOrder: 0, version: 1 }

it("saves an Italian monetary string as integer cents with the keyboard shortcut", async () => {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input)
    if (path.includes("/accounts")) return jsonResponse({ data: [account] })
    if (path.includes("/categories")) return jsonResponse({ data: [category] })
    if (path.includes("/suggestions")) return jsonResponse({ data: [] })
    if (path.includes("/movements") && init?.method === "POST") {
      const body = JSON.parse(String(init.body)) as { amountCents: number }
      expect(body.amountCents).toBe(123456)
      return jsonResponse({ data: { id: "movement-1", merchant: "Forno", amountCents: 123456 }, balances: [] }, 201)
    }
    throw new Error(`Unexpected request ${path}`)
  })

  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <QuickAdd open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  )

  await user.type(await screen.findByLabelText("Importo"), "1.234,56")
  await user.type(screen.getByLabelText("Esercente"), "Forno")
  await user.click(await screen.findByRole("button", { name: /Alimentari/ }))
  await user.keyboard("{Control>}{Enter}{/Control}")

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/v1/movements", expect.objectContaining({ method: "POST" })))
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

it("has no detectable accessibility violations in its initial state", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input)
    if (path.includes("/accounts")) return jsonResponse({ data: [account] })
    if (path.includes("/categories")) return jsonResponse({ data: [category] })
    return jsonResponse({ data: [] })
  })
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <QuickAdd open onOpenChange={() => undefined} />
    </QueryClientProvider>,
  )
  await screen.findByRole("button", { name: /Alimentari/ })
  const result = await axe(document.body)
  expect(result.violations).toHaveLength(0)
})

it("keeps provenance and description separate for income", async () => {
  const user = userEvent.setup()
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input)
    if (path.includes("/accounts")) return jsonResponse({ data: [account] })
    return jsonResponse({ data: [] })
  })
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <QuickAdd open onOpenChange={() => undefined} />
    </QueryClientProvider>,
  )
  await user.click(screen.getByRole("button", { name: "Entrata" }))
  expect(screen.getByLabelText("Provenienza del denaro")).toBeVisible()
  expect(screen.getByLabelText("Descrizione")).toBeVisible()
})

function jsonResponse(body: unknown, status = 200): Promise<Response> {
  return Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }))
}
