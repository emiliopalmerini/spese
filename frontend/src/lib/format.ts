const integerFormatter = new Intl.NumberFormat("it-IT", {
  maximumFractionDigits: 0,
  useGrouping: true,
})

export function parseItalianCents(raw: string): number {
  let value = raw.trim().replace(/[€\s\u00a0]/g, "")
  if (!value || !/^[+-]?[0-9][0-9.,]*$/.test(value)) {
    throw new Error("Importo non valido")
  }

  let sign = 1n
  if (value[0] === "-" || value[0] === "+") {
    if (value[0] === "-") sign = -1n
    value = value.slice(1)
  }

  const commaCount = count(value, ",")
  const dotCount = count(value, ".")
  let euros: string
  let cents = ""

  if (commaCount > 0 && dotCount > 0) {
    const decimal = value.lastIndexOf(",") > value.lastIndexOf(".") ? "," : "."
    const parts = value.split(decimal)
    const integerPart = parts[0]
    const fractionalPart = parts[1]
    if (parts.length !== 2 || integerPart === undefined || fractionalPart === undefined || fractionalPart.length < 1 || fractionalPart.length > 2) {
      throw new Error("Importo non valido")
    }
    const grouping = decimal === "," ? "." : ","
    if (!validGroupedInteger(integerPart, grouping)) throw new Error("Importo non valido")
    euros = integerPart.split(grouping).join("")
    cents = fractionalPart
  } else if (commaCount > 0) {
    if (commaCount !== 1) throw new Error("Importo non valido")
    const parts = value.split(",")
    const integerPart = parts[0]
    const fractionalPart = parts[1]
    if (integerPart === undefined || fractionalPart === undefined || fractionalPart.length < 1 || fractionalPart.length > 2) throw new Error("Importo non valido")
    euros = integerPart
    cents = fractionalPart
  } else if (dotCount > 0) {
    const parts = value.split(".")
    const integerPart = parts[0]
    const fractionalPart = parts[1]
    if (dotCount === 1 && integerPart !== undefined && fractionalPart !== undefined && fractionalPart.length >= 1 && fractionalPart.length <= 2) {
      euros = integerPart
      cents = fractionalPart
    } else {
      if (!validGroupedInteger(value, ".")) throw new Error("Importo non valido")
      euros = parts.join("")
    }
  } else {
    euros = value
  }

  const normalizedCents = (cents + "00").slice(0, 2)
  const result = sign * (BigInt(euros) * 100n + BigInt(normalizedCents))
  if (result > BigInt(Number.MAX_SAFE_INTEGER) || result < BigInt(Number.MIN_SAFE_INTEGER)) {
    throw new Error("Importo troppo grande")
  }
  return Number(result)
}

export function formatCents(cents: number): string {
  if (!Number.isSafeInteger(cents)) throw new Error("Importo non valido")
  const sign = cents < 0 ? "-" : ""
  const absolute = Math.abs(cents)
  const euros = Math.floor(absolute / 100)
  const decimals = String(absolute % 100).padStart(2, "0")
  return `${sign}${integerFormatter.format(euros)},${decimals}\u00a0€`
}

export function monthBounds(month: string): { from: string; to: string } {
  if (!/^\d{4}-(0[1-9]|1[0-2])$/.test(month)) throw new Error("Mese non valido")
  const [year, monthNumber] = month.split("-").map(Number)
  if (year === undefined || monthNumber === undefined) throw new Error("Mese non valido")
  const lastDay = new Date(Date.UTC(year, monthNumber, 0)).getUTCDate()
  return { from: `${month}-01`, to: `${month}-${String(lastDay).padStart(2, "0")}` }
}

export function todayInRome(now = new Date()): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Rome",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now)
  const get = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? ""
  return `${get("year")}-${get("month")}-${get("day")}`
}

function validGroupedInteger(value: string, separator: string): boolean {
  const groups = value.split(separator)
  const first = groups[0]
  return first !== undefined && /^[0-9]{1,3}$/.test(first) && groups.slice(1).every((group) => /^[0-9]{3}$/.test(group))
}

function count(value: string, character: string): number {
  return value.split(character).length - 1
}
