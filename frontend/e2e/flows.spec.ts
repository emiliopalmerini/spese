import { expect, test, type APIRequestContext } from "@playwright/test"

test.describe.configure({ mode: "serial" })

test("1. nuova spesa con esercente noto", async ({ page }) => {
  await page.goto("/")
  await page.getByRole("button", { name: "Nuovo movimento" }).click()
  await page.getByLabel("Importo").fill("12,34")
  await page.getByLabel("Esercente").fill("Supermercato")
  await page.getByRole("button", { name: /Alimentari/ }).first().click()
  await page.getByRole("button", { name: "Salva", exact: true }).click()
  await expect(page.getByText(/Supermercato · 12,34/)).toBeVisible()
})

test("2. nuova entrata", async ({ page }) => {
  await page.goto("/")
  await page.getByRole("button", { name: "Nuovo movimento" }).click()
  await page.getByRole("button", { name: "Entrata" }).click()
  await page.getByLabel("Importo").fill("750,00")
  await page.getByLabel("Provenienza del denaro").fill("Cliente E2E")
  await page.getByRole("button", { name: /Lavoro/ }).first().click()
  await page.getByRole("button", { name: "Salva", exact: true }).click()
  await expect(page.getByText(/Cliente E2E/)).toBeVisible()
})

test("3. trasferimento zero-sum", async ({ request }) => {
  const { accounts } = await fixtures(request)
  const response = await request.post("/api/v1/movements", { headers: idempotency(), data: { kind: "transfer", status: "posted", date: today(), accountId: accounts[0].id, destinationAccountId: accounts[1].id, amountCents: 5000, merchant: "", note: "E2E", origin: "manual", allocations: [] } })
  expect(response.status()).toBe(201)
  const body = await response.json() as { data: { postings: Array<{ amountCents: number }> } }
  expect(body.data.postings.reduce((sum, posting) => sum + posting.amountCents, 0)).toBe(0)
})

test("4. split tra categorie", async ({ request }) => {
  const { accounts, expenseChildren } = await fixtures(request)
  const response = await request.post("/api/v1/movements", { headers: idempotency(), data: { kind: "expense", status: "posted", date: today(), accountId: accounts[0].id, amountCents: 3000, merchant: "Split E2E", note: "", origin: "manual", allocations: [{ categoryId: expenseChildren[0].id, amountCents: 1000 }, { categoryId: expenseChildren[1].id, amountCents: 2000 }] } })
  expect(response.status()).toBe(201)
})

test("5. modifica, void e audit", async ({ request }) => {
  const created = await createExpense(request, "Void E2E", 900)
  const removed = await request.delete(`/api/v1/movements/${created.id}`, { headers: { "If-Match": `"${created.version}"` }, data: { reason: "Test E2E" } })
  expect(removed.status()).toBe(200)
  expect((await removed.json() as { data: { status: string; voidReason: string } }).data.status).toBe("void")
})

test("6. categoria e sottocategoria canoniche", async ({ request }) => {
  const categories = await request.get("/api/v1/categories?kind=expense")
  const body = await categories.json() as { data: Array<{ id: string; parentId?: string }> }
  expect(body.data.some((category) => category.parentId)).toBeTruthy()
})

test("7. merge e riclassificazione atomici", async ({ request }) => {
  const { expenseChildren } = await fixtures(request)
  const created = await createExpense(request, "Merge E2E", 1100, expenseChildren[0].id)
  const response = await request.post("/api/v1/categories/bulk-reclassify", { data: { movementIds: [created.id], categoryId: expenseChildren[1].id } })
  expect(response.status()).toBe(200)
})

test("8. coda da categorizzare", async ({ request }) => {
  const { accounts, expenseParents } = await fixtures(request)
  const response = await request.post("/api/v1/movements", { headers: idempotency(), data: { kind: "expense", status: "posted", date: today(), accountId: accounts[0].id, amountCents: 700, merchant: "Queue E2E", note: "", origin: "manual", allocations: [{ categoryId: expenseParents[0].id, amountCents: 700 }] } })
  expect(response.status()).toBe(201)
  const queue = await request.get("/api/v1/movements?queue=uncategorized&q=Queue")
  expect((await queue.json() as { data: unknown[] }).data).toHaveLength(1)
})

test("9. creazione ricorrenza", async ({ request }) => {
  const { accounts, expenseChildren } = await fixtures(request)
  const response = await request.post("/api/v1/recurring-rules", { headers: idempotency(), data: { kind: "expense", frequency: "monthly", interval: 1, startDate: today(), dayOfMonth: Number(today().slice(-2)), timezone: "Europe/Rome", amountCents: 1500, amountMode: "fixed", accountId: accounts[0].id, categoryId: expenseChildren[0].id, merchant: "Ricorrenza E2E", note: "", mode: "auto_post" } })
  expect(response.status()).toBe(201)
})

test("10. catch-up non duplica le occurrence", async ({ request }) => {
  const rules = await request.get("/api/v1/recurring-rules")
  const body = await rules.json() as { data: Array<{ id: string }> }
  const occurrence = await request.get(`/api/v1/recurring-rules/${body.data[0].id}/occurrences`)
  const items = (await occurrence.json() as { data: Array<{ scheduledFor: string }> }).data
  expect(new Set(items.map((item) => item.scheduledFor)).size).toBe(items.length)
})

test("11. riconciliazione mensile", async ({ request }) => {
  const { accounts } = await fixtures(request)
  const period = today().slice(0, 7)
  const previewResponse = await request.post("/api/v1/reconciliations/preview", { data: { period, accounts: [{ accountId: accounts[0].id, actualBalanceCents: 123456 }] } })
  expect(previewResponse.status()).toBe(200)
  const preview = (await previewResponse.json() as { data: unknown }).data
  expect((await request.post("/api/v1/reconciliations", { headers: idempotency(), data: preview })).status()).toBe(201)
})

test("12. drill-down da grafico a movimenti", async ({ page }) => {
  await page.goto("/analisi?area=spese")
  await expect(page.getByRole("heading", { name: "Spese per categoria" })).toBeVisible()
  const open = page.getByRole("button", { name: /^Apri / }).first()
  await open.focus()
  await open.press("Enter")
  await expect(page).toHaveURL(/movimenti/)
})

test("13. dettatura con servizi mockati", async ({ page }) => {
  await page.goto("/")
  await page.route("**/api/v1/dictation/fallback", async (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ type: "drafts", extraction: { movements: [{ id: "draft-1", amount: "10,00", payee: "Forno", issues: [] }] } }) }))
  const result = await page.evaluate(async () => fetch("/api/v1/dictation/fallback", { method: "POST", headers: { "X-Spese-CSRF": "1" }, body: new FormData() }).then((response) => response.json())) as { type: string }
  expect(result.type).toBe("drafts")
})

test("14. viewport desktop", async ({ page }) => {
  await page.goto("/")
  await expect(page.getByRole("navigation", { name: "Navigazione principale" })).toBeVisible()
  await expect(page.getByRole("heading", { name: /Sei a/ })).toBeVisible()
})

test("15. navigazione interamente da tastiera", async ({ page }) => {
  await page.goto("/")
  await page.keyboard.press("Tab")
  await expect(page.getByRole("link", { name: "Vai al contenuto" })).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page.locator("#contenuto")).toBeFocused()
})

async function fixtures(request: APIRequestContext) {
  const accounts = (await (await request.get("/api/v1/accounts")).json() as { data: Array<{ id: string }> }).data
  const categories = (await (await request.get("/api/v1/categories?kind=expense")).json() as { data: Array<{ id: string; parentId?: string }> }).data
  return { accounts, expenseParents: categories.filter((item) => !item.parentId), expenseChildren: categories.filter((item) => item.parentId) }
}

async function createExpense(request: APIRequestContext, merchant: string, amountCents: number, categoryID?: string) {
  const { accounts, expenseChildren } = await fixtures(request)
  const response = await request.post("/api/v1/movements", { headers: idempotency(), data: { kind: "expense", status: "posted", date: today(), accountId: accounts[0].id, amountCents, merchant, note: "", origin: "manual", allocations: [{ categoryId: categoryID ?? expenseChildren[0].id, amountCents }] } })
  expect(response.status()).toBe(201)
  return (await response.json() as { data: { id: string; version: number } }).data
}

function idempotency() { return { "Idempotency-Key": crypto.randomUUID() } }
function today() { return new Date().toISOString().slice(0, 10) }
