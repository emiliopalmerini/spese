import { mkdir } from "node:fs/promises"
import path from "node:path"

import { expect, test } from "@playwright/test"

const output = path.resolve(import.meta.dirname, "../../tmp/screenshots")

test("capture PR evidence", async ({ page }) => {
  await mkdir(output, { recursive: true })
  await page.goto("/")
  await expect(page.getByRole("heading", { name: /Sei a/ })).toBeVisible()
  await page.screenshot({ path: path.join(output, "overview-desktop.png"), fullPage: true })

  await page.getByRole("button", { name: "Nuovo movimento" }).first().click()
  await expect(page.getByLabel("Esercente")).toBeVisible()
  await page.screenshot({ path: path.join(output, "quick-add-desktop.png"), fullPage: true })
  await page.getByRole("button", { name: "Chiudi" }).click()

  await page.goto("/analisi?area=flusso")
  await expect(page.getByRole("heading", { name: "Flusso mensile" })).toBeVisible()
  await page.screenshot({ path: path.join(output, "analytics-desktop.png"), fullPage: true })

  await page.goto("/categorie")
  await expect(page.getByRole("heading", { name: "Categorie" })).toBeVisible()
  await page.screenshot({ path: path.join(output, "categories-desktop.png"), fullPage: true })

  await page.goto("/ricorrenti")
  await expect(page.getByRole("heading", { name: "Ricorrenti" })).toBeVisible()
  await page.screenshot({ path: path.join(output, "recurring-desktop.png"), fullPage: true })

  await page.goto("/conti")
  await page.getByRole("button", { name: "Riconcilia mese" }).click()
  await expect(page.getByRole("dialog")).toBeVisible()
  await page.screenshot({ path: path.join(output, "reconciliation-desktop.png"), fullPage: true })
})
