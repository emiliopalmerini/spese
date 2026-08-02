import { describe, expect, it } from "vitest"

import { formatCents, monthBounds, parseItalianCents } from "./format"

describe("parseItalianCents", () => {
  it.each([
    ["12", 1200],
    ["12,3", 1230],
    ["12,34", 1234],
    ["1.234,56 €", 123456],
    ["1 234,56", 123456],
    ["-0,99", -99],
    ["5.086", 508600],
  ])("converts %s without floating point arithmetic", (input, expected) => {
    expect(parseItalianCents(input)).toBe(expected)
  })

  it.each(["", "ciao", "1,234,56", "12,345"])("rejects ambiguous value %s", (input) => {
    expect(() => parseItalianCents(input)).toThrow()
  })
})

it("formats cents using the Italian locale", () => {
  expect(formatCents(123456)).toBe("1.234,56 €")
})

it("builds explicit ISO month boundaries", () => {
  expect(monthBounds("2024-02")).toEqual({ from: "2024-02-01", to: "2024-02-29" })
  expect(monthBounds("2026-02")).toEqual({ from: "2026-02-01", to: "2026-02-28" })
})
