export interface Account {
  id: string
  name: string
  type: "Asset" | "Liability"
  class: "Cash" | "Investment" | "Property" | "Tax" | "Credit" | "Other"
  currency: "EUR"
  initialBalanceCents: number
  initialDate: string
  note?: string
  archivedAt?: string
  version: number
}

export interface Category {
  id: string
  parentId?: string
  kind: "expense" | "income"
  name: string
  icon: string
  color: string
  sortOrder: number
  archivedAt?: string
  version: number
}

export interface Posting {
  id: string
  accountId: string
  amountCents: number
}

export interface Allocation {
  id: string
  categoryId: string
  amountCents: number
}

export interface Movement {
  id: string
  kind: "expense" | "income" | "refund" | "transfer" | "adjustment"
  status: "draft" | "posted" | "void"
  date: string
  amountCents: number
  merchant: string
  description: string
  note: string
  origin: "manual" | "recurring" | "dictation" | "migration"
  version: number
  postings: Posting[]
  allocations: Allocation[]
}

export interface CategoryMetric {
  id: string
  name: string
  path: string
  color: string
  icon: string
  amountCents: number
  movementCount: number
  lastUsed: string
  merchantRuleCount: number
  drilldown: string
}

export interface Overview {
  month: string
  incomeCents: number
  expenseCents: number
  recurringExpenseCents: number
  otherExpenseCents: number
  savingsCents: number
  netWorthCents: number
  attention: { uncategorized: number; unreconciledAccounts: number; recurringToReview: number }
  topCategories: CategoryMetric[] | null
  accounts: Array<{ id: string; name: string; balance: { balanceCents: number; reconciled: boolean } }> | null
  drilldown: string
}

export class APIError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly fieldErrors?: Record<string, string>,
  ) {
    super(message)
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set("Accept", "application/json")
  if (init.body) headers.set("Content-Type", "application/json")
  const method = init.method?.toUpperCase() ?? "GET"
  if (["POST", "PATCH", "PUT", "DELETE"].includes(method)) headers.set("X-Spese-CSRF", "1")
  const response = await fetch(path, { ...init, headers, credentials: "same-origin" })
  const payload = (await response.json().catch(() => null)) as unknown
  if (!response.ok) {
    const error = payload as { code?: string; message?: string; fieldErrors?: Record<string, string> } | null
    throw new APIError(response.status, error?.code ?? "request_failed", error?.message ?? "Richiesta non riuscita", error?.fieldErrors)
  }
  return payload as T
}

export function mutationHeaders(version?: number): HeadersInit {
  const headers: Record<string, string> = { "Idempotency-Key": crypto.randomUUID() }
  if (version !== undefined) headers["If-Match"] = `"${String(version)}"`
  return headers
}
