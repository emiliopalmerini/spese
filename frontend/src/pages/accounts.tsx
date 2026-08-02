import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CheckCircle2, Landmark, Plus, Scale, WalletCards } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { api, type Account, type Overview, mutationHeaders } from "@/lib/api"
import { formatCents, parseItalianCents } from "@/lib/format"

interface Preview {
  id: string
  period: string
  closedThrough: string
  accounts: Array<{ accountId: string; expectedBalanceCents: number; actualBalanceCents: number; differenceCents: number }>
}

export function AccountsPage() {
  const [reconcileOpen, setReconcileOpen] = useState(false)
  const month = new Date().toISOString().slice(0, 7)
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api<{ data: Account[] }>("/api/v1/accounts") })
  const overview = useQuery({ queryKey: ["overview", month], queryFn: () => api<{ data: Overview }>(`/api/v1/analytics/overview?month=${month}`) })
  const balances = new Map(overview.data?.data.accounts?.map((account) => [account.id, account.balance]))
  return (
    <div className="page-wrap space-y-8">
      <header className="flex flex-wrap items-end justify-between gap-4"><div><p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Patrimonio</p><h1 className="mt-1 font-display text-4xl font-semibold">Conti</h1><p className="mt-2 text-muted-foreground">Il saldo non è modificabile: nasce dal saldo iniziale, dagli anchor reali e dai posting.</p></div><div className="flex gap-2"><Button onClick={() => setReconcileOpen(true)} variant="charcoal"><Scale /> Riconcilia mese</Button><Button variant="outline"><Plus /> Nuovo conto</Button></div></header>
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {accounts.data?.data.map((account) => {
          const balance = balances.get(account.id)
          return <article className="surface p-5" key={account.id}><div className="flex items-start justify-between"><span className="flex size-11 items-center justify-center rounded-xl bg-secondary">{account.class === "Cash" ? <WalletCards /> : <Landmark />}</span>{balance?.reconciled ? <span className="flex items-center gap-1 text-xs font-semibold text-[#496126]"><CheckCircle2 className="size-4" /> Riconciliato</span> : <span className="text-xs text-muted-foreground">Calcolato</span>}</div><h2 className="mt-5 font-semibold">{account.name}</h2><p className="display-number mt-1 break-words text-3xl">{formatCents(balance?.balanceCents ?? account.initialBalanceCents)}</p><p className="mt-4 border-t pt-3 text-sm text-muted-foreground">{account.type === "Asset" ? "Attività" : "Passività"} · {account.class}</p></article>
        })}
      </section>
      {!accounts.isLoading && !accounts.data?.data.length ? <div className="rounded-2xl border border-dashed p-12 text-center"><Landmark className="mx-auto mb-3" /><h2 className="font-display text-2xl">Aggiungi il primo conto</h2></div> : null}
      <ReconciliationSheet accounts={accounts.data?.data ?? []} month={month} onOpenChange={setReconcileOpen} open={reconcileOpen} />
    </div>
  )
}

export function ReconciliationSheet({ accounts, month, open, onOpenChange }: { accounts: Account[]; month: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>({})
  const [preview, setPreview] = useState<Preview | null>(null)
  const previewMutation = useMutation({
    mutationFn: () => api<{ data: Preview }>("/api/v1/reconciliations/preview", { method: "POST", body: JSON.stringify({ period: month, accounts: Object.entries(values).filter(([, value]) => value.trim()).map(([accountId, value]) => ({ accountId, actualBalanceCents: parseItalianCents(value) })) }) }),
    onSuccess: ({ data }) => setPreview(data),
  })
  const commitMutation = useMutation({
    mutationFn: () => api("/api/v1/reconciliations", { method: "POST", headers: mutationHeaders(), body: JSON.stringify(preview) }),
    onSuccess: () => { toast.success("Riconciliazione confermata"); setPreview(null); setValues({}); onOpenChange(false); void queryClient.invalidateQueries({ queryKey: ["overview"] }) },
  })
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent><SheetHeader><SheetTitle>Riconcilia {month}</SheetTitle><SheetDescription>Inserisci solo i saldi reali disponibili. Puoi riconciliare i conti in modo parziale.</SheetDescription></SheetHeader><div className="space-y-5 overflow-y-auto p-5">{accounts.map((account) => { const row = preview?.accounts.find((item) => item.accountId === account.id); return <div className="rounded-xl border p-4" key={account.id}><div className="flex items-center justify-between gap-3"><Label htmlFor={`balance-${account.id}`}>{account.name}</Label>{row ? <span className={row.differenceCents === 0 ? "text-[#496126]" : "text-destructive"}>{row.differenceCents === 0 ? "Allineato" : `Differenza ${formatCents(row.differenceCents)}`}</span> : null}</div><Input className="display-number mt-2 text-2xl" id={`balance-${account.id}`} inputMode="decimal" onChange={(event) => setValues((current) => ({ ...current, [account.id]: event.target.value }))} placeholder="Saldo reale" value={values[account.id] ?? ""} />{row ? <p className="mt-2 text-sm text-muted-foreground">Atteso {formatCents(row.expectedBalanceCents)} · Reale {formatCents(row.actualBalanceCents)}</p> : null}</div> })}{preview ? <div className="surface-strong p-4 text-sm"><p className="font-semibold">Anteprima pronta</p><p className="mt-1 text-[#d4cfc2]">L’anchor chiude il periodo al {preview.closedThrough}. I movimenti retrodatati cambieranno l’analisi, non i saldi successivi.</p></div> : null}{previewMutation.error || commitMutation.error ? <p className="text-sm text-destructive" role="alert">Impossibile completare la riconciliazione. I valori inseriti sono conservati.</p> : null}<div className="grid gap-2 sm:grid-cols-2">{preview ? <><Button onClick={() => setPreview(null)} variant="outline">Modifica saldi</Button><Button disabled={commitMutation.isPending} onClick={() => commitMutation.mutate()}>Conferma</Button></> : <Button className="sm:col-span-2" disabled={previewMutation.isPending || !Object.values(values).some(Boolean)} onClick={() => previewMutation.mutate()}>{previewMutation.isPending ? "Calcolo…" : "Mostra anteprima"}</Button>}</div></div></SheetContent></Sheet>
}
