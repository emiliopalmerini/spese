import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowDownLeft, ArrowLeftRight, ArrowUpRight, Filter, Plus, RotateCcw, Search, SlidersHorizontal } from "lucide-react"
import { useState } from "react"
import { useSearchParams } from "react-router"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { api, type Account, type Category, type Movement, mutationHeaders } from "@/lib/api"
import { formatCents } from "@/lib/format"
import { cn } from "@/lib/utils"

export function MovementsPage({ onAdd }: { onAdd: () => void }) {
  const [searchParams, setSearchParams] = useSearchParams()
  const [selected, setSelected] = useState<Movement | null>(null)
  const query = searchParams.toString()
  const movements = useQuery({ queryKey: ["movements", query], queryFn: () => api<{ data: Movement[]; nextCursor: string }>(`/api/v1/movements?${query}`) })
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api<{ data: Account[] }>("/api/v1/accounts") })
  const categories = useQuery({ queryKey: ["categories", "all"], queryFn: () => api<{ data: Category[] }>("/api/v1/categories") })
  const accountNames = new Map(accounts.data?.data.map((account) => [account.id, account.name]))
  const categoryNames = new Map(categories.data?.data.map((category) => [category.id, category.name]))

  const updateFilter = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    next.delete("cursor")
    setSearchParams(next)
  }

  return (
    <div className="page-wrap space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div><p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Registro</p><h1 className="mt-1 font-display text-4xl font-semibold">Movimenti</h1></div>
        <Button className="hidden md:inline-flex" onClick={onAdd}><Plus /> Nuovo movimento</Button>
      </header>

      <section aria-label="Filtri movimenti" className="surface p-3">
        <div className="grid gap-2 md:grid-cols-[minmax(14rem,1fr)_repeat(3,minmax(9rem,auto))]">
          <label className="relative"><span className="sr-only">Cerca movimenti</span><Search className="pointer-events-none absolute left-3 top-3.5 size-4 text-muted-foreground" /><Input className="pl-9" defaultValue={searchParams.get("q") ?? ""} onChange={(event) => updateFilter("q", event.target.value)} placeholder="Cerca esercente, nota…" /></label>
          <FilterSelect label="Tipo" onChange={(value) => updateFilter("kind", value === "all" ? "" : value)} value={searchParams.get("kind") ?? "all"} values={[{ value: "all", label: "Tutti i tipi" }, { value: "expense", label: "Spese" }, { value: "income", label: "Entrate" }, { value: "refund", label: "Rimborsi" }, { value: "transfer", label: "Trasferimenti" }]} />
          <FilterSelect label="Conto" onChange={(value) => updateFilter("account", value === "all" ? "" : value)} value={searchParams.get("account") ?? "all"} values={[{ value: "all", label: "Tutti i conti" }, ...(accounts.data?.data.map((account) => ({ value: account.id, label: account.name })) ?? [])]} />
          <Button className="justify-start" variant="outline"><SlidersHorizontal /> Altri filtri</Button>
        </div>
        <div className="mt-2 flex flex-wrap gap-2">
          <Input aria-label="Data iniziale" className="w-auto" onChange={(event) => updateFilter("from", event.target.value)} type="date" value={searchParams.get("from") ?? ""} />
          <Input aria-label="Data finale" className="w-auto" onChange={(event) => updateFilter("to", event.target.value)} type="date" value={searchParams.get("to") ?? ""} />
          {searchParams.toString() ? <Button onClick={() => setSearchParams({})} variant="ghost"><RotateCcw /> Azzera</Button> : null}
        </div>
      </section>

      {movements.isLoading ? <MovementSkeleton /> : movements.isError ? <div className="surface p-8 text-center" role="alert">Impossibile caricare i movimenti. <Button onClick={() => void movements.refetch()} variant="ghost">Riprova</Button></div> : movements.data?.data.length ? (
        <>
          <div className="surface hidden overflow-hidden md:block">
            <table className="w-full text-sm">
              <thead className="border-b bg-secondary/60 text-left text-xs uppercase tracking-wider text-muted-foreground"><tr><th className="px-4 py-3">Data</th><th className="px-4 py-3">Movimento</th><th className="px-4 py-3">Categoria</th><th className="px-4 py-3">Conto</th><th className="px-4 py-3 text-right">Importo</th></tr></thead>
              <tbody>{movements.data.data.map((movement) => <MovementRow accountNames={accountNames} categoryNames={categoryNames} key={movement.id} movement={movement} onSelect={setSelected} />)}</tbody>
            </table>
          </div>
          <div className="space-y-6 md:hidden">{groupByDate(movements.data.data).map(([date, items]) => <section key={date}><h2 className="mb-3 text-sm font-semibold capitalize text-muted-foreground">{dateLabel(date)}</h2><div className="space-y-1">{items.map((movement) => <MovementCard accountNames={accountNames} key={movement.id} movement={movement} onSelect={setSelected} />)}</div></section>)}</div>
          {movements.data.nextCursor ? <Button className="mx-auto flex" onClick={() => updateFilter("cursor", movements.data.nextCursor)} variant="outline">Carica altri</Button> : null}
        </>
      ) : <div className="rounded-2xl border border-dashed p-12 text-center"><Filter className="mx-auto mb-3 text-muted-foreground" /><h2 className="font-display text-2xl">Nessun movimento</h2><p className="mt-2 text-muted-foreground">Prova ad azzerare i filtri oppure registra il primo.</p><Button className="mt-5" onClick={onAdd}><Plus /> Nuovo movimento</Button></div>}

      <MovementDetail movement={selected} onClose={() => setSelected(null)} />
    </div>
  )
}

function MovementRow({ movement, accountNames, categoryNames, onSelect }: { movement: Movement; accountNames: Map<string, string>; categoryNames: Map<string, string>; onSelect: (value: Movement) => void }) {
  return <tr className="cursor-pointer border-b last:border-0 hover:bg-secondary/40" onClick={() => onSelect(movement)}><td className="px-4 py-3 tabular-nums text-muted-foreground">{shortDate(movement.date)}</td><td className="px-4 py-3"><span className="flex items-center gap-3"><MovementIcon kind={movement.kind} /><span><strong className="block">{movement.merchant || movement.description || labelKind(movement.kind)}</strong><small className="text-muted-foreground">{movement.origin === "recurring" ? "Ricorrente" : movement.description || movement.note}</small></span></span></td><td className="px-4 py-3 text-muted-foreground">{movement.allocations.map((item) => categoryNames.get(item.categoryId)).filter(Boolean).join(" + ") || "—"}</td><td className="px-4 py-3 text-muted-foreground">{movement.postings.map((item) => accountNames.get(item.accountId)).filter(Boolean).join(" → ")}</td><td className={cn("px-4 py-3 text-right font-semibold tabular-nums", movement.kind === "income" || movement.kind === "refund" ? "text-[#496126]" : movement.status === "void" ? "text-muted-foreground line-through" : "")}>{movement.kind === "expense" ? "−" : movement.kind === "income" || movement.kind === "refund" ? "+" : ""}{formatCents(movement.amountCents)}</td></tr>
}

function MovementCard({ movement, accountNames, onSelect }: { movement: Movement; accountNames: Map<string, string>; onSelect: (value: Movement) => void }) {
  return <button className="pressable grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 rounded-xl bg-card px-3 py-2 text-left" onClick={() => onSelect(movement)} type="button"><MovementIcon kind={movement.kind} /><span className="min-w-0"><strong className="block truncate">{movement.merchant || movement.description || labelKind(movement.kind)}</strong><small className="block truncate text-muted-foreground">{movement.description || accountNames.get(movement.postings[0]?.accountId ?? "")}</small></span><strong className={cn("tabular-nums", movement.kind === "income" || movement.kind === "refund" ? "text-[#496126]" : "")}>{movement.kind === "expense" ? "−" : movement.kind === "income" || movement.kind === "refund" ? "+" : ""}{formatCents(movement.amountCents)}</strong></button>
}

function MovementDetail({ movement, onClose }: { movement: Movement | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const voidMutation = useMutation({ mutationFn: () => api(`/api/v1/movements/${movement?.id ?? ""}`, { method: "DELETE", headers: mutationHeaders(movement?.version), body: JSON.stringify({ reason: "Annullato dal dettaglio" }) }), onSuccess: () => { toast.success("Movimento annullato"); onClose(); void queryClient.invalidateQueries({ queryKey: ["movements"] }) } })
  return <Sheet open={movement !== null} onOpenChange={(open) => { if (!open) onClose() }}><SheetContent><SheetHeader><SheetTitle>{movement?.merchant || movement?.description || (movement ? labelKind(movement.kind) : "Movimento")}</SheetTitle><SheetDescription>{movement?.date} · {movement?.status === "void" ? "Annullato" : "Registrato"}</SheetDescription></SheetHeader>{movement ? <div className="space-y-6 p-5"><p className="display-number break-words text-5xl">{formatCents(movement.amountCents)}</p><dl className="grid grid-cols-2 gap-4 border-y py-5 text-sm"><div><dt className="text-muted-foreground">Tipo</dt><dd className="mt-1 font-semibold">{labelKind(movement.kind)}</dd></div><div><dt className="text-muted-foreground">Origine</dt><dd className="mt-1 font-semibold">{movement.origin}</dd></div><div className="col-span-2"><dt className="text-muted-foreground">Descrizione</dt><dd className="mt-1">{movement.description || "Nessuna descrizione"}</dd></div><div className="col-span-2"><dt className="text-muted-foreground">Note</dt><dd className="mt-1">{movement.note || "Nessuna nota"}</dd></div></dl><div className="grid gap-2 sm:grid-cols-2"><Button variant="outline">Modifica</Button>{movement.status !== "void" ? <Button disabled={voidMutation.isPending} onClick={() => voidMutation.mutate()} variant="destructive">Annulla movimento</Button> : null}</div></div> : null}</SheetContent></Sheet>
}

function MovementIcon({ kind }: { kind: Movement["kind"] }) {
  const Icon = kind === "transfer" ? ArrowLeftRight : kind === "income" || kind === "refund" ? ArrowDownLeft : ArrowUpRight
  return <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-full", kind === "income" || kind === "refund" ? "bg-[#dce7d0]" : kind === "transfer" ? "bg-[#dedbe8]" : "bg-[#eadbd6]")}><Icon className="size-4" /></span>
}

function FilterSelect({ label, value, values, onChange }: { label: string; value: string; values: Array<{ value: string; label: string }>; onChange: (value: string) => void }) {
  return <Select onValueChange={onChange} value={value}><SelectTrigger aria-label={label}><SelectValue /></SelectTrigger><SelectContent>{values.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
}

function MovementSkeleton() { return <div className="space-y-2"><Skeleton className="h-14" /><Skeleton className="h-14" /><Skeleton className="h-14" /><Skeleton className="h-14" /></div> }
function groupByDate(items: Movement[]): Array<[string, Movement[]]> {
  const groups = new Map<string, Movement[]>()
  for (const item of items) groups.set(item.date, [...(groups.get(item.date) ?? []), item])
  return Array.from(groups.entries())
}
function shortDate(value: string): string { return new Intl.DateTimeFormat("it-IT", { day: "2-digit", month: "short", timeZone: "UTC" }).format(new Date(`${value}T00:00:00Z`)) }
function dateLabel(value: string): string { return new Intl.DateTimeFormat("it-IT", { weekday: "long", day: "numeric", month: "long", timeZone: "UTC" }).format(new Date(`${value}T00:00:00Z`)) }
function labelKind(kind: Movement["kind"]): string { return { expense: "Spesa", income: "Entrata", refund: "Rimborso", transfer: "Trasferimento", adjustment: "Rettifica" }[kind] }
