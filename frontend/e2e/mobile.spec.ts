import { expect, test } from "@playwright/test"

test("mobile navigation and bottom-sheet quick add", async ({ page }) => {
  await page.goto("/")
  await expect(page.getByRole("navigation", { name: "Navigazione mobile" })).toBeVisible()
  await page.getByRole("button", { name: "Nuovo movimento" }).click()
  await expect(page.getByRole("dialog")).toBeVisible()
  await expect(page.getByLabel("Importo")).toBeFocused()
  await page.getByRole("button", { name: "Chiudi" }).click()
  await page.getByRole("link", { name: "Movimenti" }).click()
  await expect(page).toHaveURL(/movimenti/)
})
