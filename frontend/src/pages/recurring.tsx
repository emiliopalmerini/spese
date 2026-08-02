import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CalendarClock, Check, MoreHorizontal, Pause, Play, Plus, SkipForward } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { formatCents } from "@/lib/format"

interface Rule { id: string; kind: "expense" | "income"; frequency: string; interval: number; startDate: string; endDate: string; amountCents: number; amountMode: "fixed" | "variable"; merchant: string; state: string; mode: string; nextDue: string; version: number }
interface Occurrence { id: string; ruleId: string; scheduledFor: string; status: "draft" | "posted" | "skipped" | "failed"; amountCents: number; amountCertainty: string; movementId?: string }

export function RecurringPage() {
  const queryClient = useQueryClient()
  const rules = useQuery({ queryKey: ["recurring-rules"], queryFn: () => api<{ data: Rule[] }>("/api/v1/recurring-rules") })
  const action = useMutation({ mutationFn: ({ id, action }: { id: string; action: string }) => api(`/api/v1/occurrences/${id}/${action}`, { method: "POST", body: "{}" }), onSuccess: () => { toast.success("Occorrenza aggiornata"); void queryClient.invalidateQueries({ queryKey: ["occurrences"] }) } })
  return (
    <div className="page-wrap space-y-8">
      <header className="flex flex-wrap items-end justify-between gap-4"><div><p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Automazioni</p><h1 className="mt-1 font-display text-4xl font-semibold">Ricorrenti</h1><p className="mt-2 max-w-2xl text-muted-foreground">Gli importi fissi si registrano da soli. Quelli variabili restano bozze e compaiono separati nel forecast.</p></div><Button><Plus /> Nuova ricorrenza</Button></header>
      <section className="grid gap-4 lg:grid-cols-[1fr_22rem]">
        <div className="space-y-3">{rules.data?.data.map((rule) => <RuleCard key={rule.id} rule={rule} />)}{!rules.isLoading && !rules.data?.data.length ? <div className="rounded-2xl border border-dashed p-12 text-center"><CalendarClock className="mx-auto mb-3" /><h2 className="font-display text-2xl">Nessuna ricorrenza</h2><p className="mt-2 text-muted-foreground">Crea affitto, stipendio o qualsiasi movimento periodico.</p></div> : null}</div>
        <aside className="surface h-fit p-5"><p className="text-sm text-muted-foreground">Prossime azioni</p><h2 className="mt-1 font-display text-2xl font-semibold">Da confermare</h2><OccurrenceQueue rules={rules.data?.data ?? []} onAction={(id, name) => action.mutate({ id, action: name })} /></aside>
      </section>
    </div>
  )
}

function RuleCard({ rule }: { rule: Rule }) {
  return <article className="surface grid gap-4 p-5 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center"><span className={`flex size-11 items-center justify-center rounded-xl ${rule.kind === "income" ? "bg-[#dce7d0]" : "bg-[#eadbd6]"}`}><CalendarClock /></span><div><div className="flex flex-wrap items-center gap-2"><h2 className="font-semibold">{rule.merchant || (rule.kind === "income" ? "Entrata ricorrente" : "Spesa ricorrente")}</h2><span className="rounded-full bg-secondary px-2 py-1 text-xs">{rule.state === "active" ? "Attiva" : rule.state}</span></div><p className="mt-1 text-sm text-muted-foreground">Ogni {frequencyLabel(rule.frequency, rule.interval)} · prossima {rule.nextDue} · {rule.mode === "auto_post" ? "automatica" : "da confermare"}</p></div><div className="flex items-center justify-between gap-2 sm:block sm:text-right"><strong className="tabular-nums">{formatCents(rule.amountCents)}</strong><div className="mt-2 flex justify-end gap-1"><Button aria-label={rule.state === "active" ? "Sospendi" : "Riattiva"} size="icon" variant="ghost">{rule.state === "active" ? <Pause /> : <Play />}</Button><Button aria-label="Altre azioni" size="icon" variant="ghost"><MoreHorizontal /></Button></div></div></article>
}

function OccurrenceQueue({ rules, onAction }: { rules: Rule[]; onAction: (id: string, action: string) => void }) {
  const ids = rules.map((rule) => rule.id)
  const occurrences = useQuery({ queryKey: ["occurrences", ids], queryFn: async () => { const groups = await Promise.all(ids.map((id) => api<{ data: Occurrence[] }>(`/api/v1/recurring-rules/${id}/occurrences`))); return groups.flatMap((group) => group.data).filter((item) => item.status === "draft") }, enabled: ids.length > 0 })
  if (!occurrences.data?.length) return <p className="mt-5 rounded-xl border border-dashed p-5 text-center text-sm text-muted-foreground">Tutto in ordine.</p>
  return <div className="mt-4 space-y-3">{occurrences.data.map((item) => <div className="rounded-xl bg-secondary/70 p-3" key={item.id}><div className="flex items-center justify-between"><span>{item.scheduledFor}</span><strong>{formatCents(item.amountCents)}</strong></div><p className="mt-1 text-xs text-muted-foreground">Importo {item.amountCertainty === "estimated" ? "stimato" : "certo"}</p><div className="mt-3 grid grid-cols-3 gap-1"><Button aria-label="Conferma" onClick={() => onAction(item.id, "confirm")} size="icon"><Check /></Button><Button aria-label="Salta" onClick={() => onAction(item.id, "skip")} size="icon" variant="outline"><SkipForward /></Button><Button aria-label="Registra ora" onClick={() => onAction(item.id, "post")} size="icon" variant="outline"><Play /></Button></div></div>)}</div>
}

function frequencyLabel(frequency: string, interval: number): string {
  const singular: Record<string, string> = { daily: "giorno", weekly: "settimana", monthly: "mese", yearly: "anno" }
  const plural: Record<string, string> = { daily: "giorni", weekly: "settimane", monthly: "mesi", yearly: "anni" }
  return interval === 1 ? singular[frequency] ?? frequency : `${interval} ${plural[frequency] ?? frequency}`
}
