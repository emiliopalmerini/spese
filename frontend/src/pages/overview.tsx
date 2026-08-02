import { useQuery } from "@tanstack/react-query"
import { AlertTriangle, ArrowDownRight, ArrowRight, ArrowUpRight, ChevronLeft, ChevronRight, Plus } from "lucide-react"
import { useNavigate, useSearchParams } from "react-router"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { api, type Overview } from "@/lib/api"
import { formatCents } from "@/lib/format"

export function OverviewPage({ onAdd }: { onAdd: () => void }) {
  const navigate = useNavigate()
  const [search, setSearch] = useSearchParams()
  const month = search.get("month") ?? new Date().toISOString().slice(0, 7)
  const overview = useQuery({ queryKey: ["overview", month], queryFn: () => api<{ data: Overview }>(`/api/v1/analytics/overview?month=${month}`) })

  if (overview.isLoading) return <OverviewSkeleton />
  if (overview.isError) return <ErrorState retry={() => void overview.refetch()} />
  const data = overview.data?.data
  if (!data) return <ErrorState retry={() => void overview.refetch()} />
  const attentionTotal = Object.values(data.attention).reduce((sum, value) => sum + value, 0)

  return (
    <div className="page-wrap space-y-10">
      <header className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Il tuo mese</p>
          <div className="mt-2 flex items-center gap-1">
            <Button aria-label="Mese precedente" onClick={() => setSearch({ month: shiftMonth(month, -1) })} size="icon" variant="ghost"><ChevronLeft /></Button>
            <h1 className="min-w-44 text-center font-display text-2xl font-semibold capitalize sm:text-3xl">{monthLabel(month)}</h1>
            <Button aria-label="Mese successivo" onClick={() => setSearch({ month: shiftMonth(month, 1) })} size="icon" variant="ghost"><ChevronRight /></Button>
          </div>
        </div>
        <Button className="hidden md:inline-flex" onClick={onAdd}><Plus /> Nuovo movimento</Button>
      </header>

      <section aria-labelledby="headline-title" className="max-w-4xl">
        <p className="mb-3 text-sm text-muted-foreground">Risultato del mese</p>
        <h2 className="display-number text-5xl leading-[0.95] sm:text-7xl" id="headline-title">
          {data.savingsCents >= 0 ? "Sei a " : "Sei a "}<span className={data.savingsCents >= 0 ? "text-[#496126]" : "text-destructive"}>{formatCents(data.savingsCents)}</span>
        </h2>
        <p className="mt-5 max-w-2xl text-lg text-muted-foreground">
          {data.savingsCents >= 0
            ? `Hai trattenuto il ${data.incomeCents ? Math.round((data.savingsCents * 100) / data.incomeCents) : 0}% delle entrate. La parte ricorrente delle spese è ${formatCents(data.recurringExpenseCents)}.`
            : `Le uscite superano le entrate di ${formatCents(Math.abs(data.savingsCents))}. Apri le categorie per capire dove intervenire.`}
        </p>
      </section>

      <section aria-labelledby="waterfall-title" className="section-rule">
        <div className="mb-5 flex items-end justify-between">
          <div><p className="text-sm text-muted-foreground">Come ci arrivi</p><h2 className="font-display text-2xl font-semibold" id="waterfall-title">Entrate → risparmio</h2></div>
          <Button onClick={() => navigate(data.drilldown)} variant="ghost">Vedi movimenti <ArrowRight /></Button>
        </div>
        <div className="grid gap-3 md:grid-cols-[1fr_auto_1fr_auto_1fr_auto_1fr] md:items-center">
          <WaterfallStep icon={<ArrowUpRight />} label="Entrate" tone="positive" value={data.incomeCents} />
          <ArrowRight className="mx-auto hidden text-muted-foreground md:block" />
          <WaterfallStep icon={<ArrowDownRight />} label="Ricorrenti" tone="negative" value={data.recurringExpenseCents} />
          <ArrowRight className="mx-auto hidden text-muted-foreground md:block" />
          <WaterfallStep icon={<ArrowDownRight />} label="Altre spese" tone="negative" value={data.otherExpenseCents} />
          <ArrowRight className="mx-auto hidden text-muted-foreground md:block" />
          <WaterfallStep icon={<ArrowRight />} label="Risparmio" tone="result" value={data.savingsCents} />
        </div>
      </section>

      {attentionTotal > 0 ? (
        <section aria-labelledby="attention-title" className="surface-strong p-5 sm:p-7">
          <div className="flex items-center gap-3"><span className="flex size-11 items-center justify-center rounded-full bg-primary text-foreground"><AlertTriangle /></span><div><p className="text-sm text-[#d4cfc2]">Richiede attenzione</p><h2 className="font-display text-2xl" id="attention-title">{attentionTotal} cose da sistemare</h2></div></div>
          <div className="mt-5 grid gap-2 sm:grid-cols-3">
            <AttentionLink label="Da categorizzare" onClick={() => navigate("/movimenti?queue=uncategorized")} value={data.attention.uncategorized} />
            <AttentionLink label="Conti non riconciliati" onClick={() => navigate("/conti?reconcile=true")} value={data.attention.unreconciledAccounts} />
            <AttentionLink label="Ricorrenze da confermare" onClick={() => navigate("/ricorrenti?status=draft")} value={data.attention.recurringToReview} />
          </div>
        </section>
      ) : null}

      <section aria-labelledby="categories-title" className="section-rule grid gap-8 lg:grid-cols-[1.25fr_0.75fr]">
        <div>
          <div className="mb-5 flex items-end justify-between"><div><p className="text-sm text-muted-foreground">Dove sono andati</p><h2 className="font-display text-2xl font-semibold" id="categories-title">Principali categorie</h2></div><Button onClick={() => navigate(`/analisi?area=spese&month=${month}`)} variant="ghost">Analizza <ArrowRight /></Button></div>
          {data.topCategories?.length ? <div className="space-y-4">{data.topCategories.map((category, index) => <CategoryBar category={category} index={index} key={category.id} onClick={() => navigate(category.drilldown)} />)}</div> : <EmptyInline text="Nessuna spesa in questo mese." />}
        </div>
        <div className="surface p-5">
          <p className="text-sm text-muted-foreground">Patrimonio stimato</p>
          <p className="display-number mt-2 break-words text-4xl">{formatCents(data.netWorthCents)}</p>
          <div className="mt-6 space-y-3">
            {data.accounts?.slice(0, 4).map((account) => (
              <button className="pressable flex w-full items-center justify-between border-t pt-3 text-left" key={account.id} onClick={() => navigate(`/conti?account=${account.id}`)} type="button">
                <span>{account.name}<small className="block text-muted-foreground">{account.balance.reconciled ? "Riconciliato" : "Calcolato"}</small></span>
                <strong className="tabular-nums">{formatCents(account.balance.balanceCents)}</strong>
              </button>
            ))}
          </div>
        </div>
      </section>
    </div>
  )
}

function WaterfallStep({ label, value, tone, icon }: { label: string; value: number; tone: "positive" | "negative" | "result"; icon: React.ReactNode }) {
  const classes = { positive: "bg-[#dce7d0]", negative: "bg-[#eadbd6]", result: "bg-primary" }
  return <button className={`${classes[tone]} pressable rounded-2xl p-4 text-left`} type="button"><span className="flex items-center gap-2 text-sm font-semibold">{icon}{label}</span><strong className="display-number mt-3 block break-words text-2xl">{formatCents(value)}</strong></button>
}

function AttentionLink({ label, value, onClick }: { label: string; value: number; onClick: () => void }) {
  return <button className="pressable flex items-center justify-between rounded-xl border border-white/15 px-4 text-left hover:bg-white/10" onClick={onClick} type="button"><span>{label}</span><strong className="tabular-nums text-primary">{value}</strong></button>
}

function CategoryBar({ category, index, onClick }: { category: NonNullable<Overview["topCategories"]>[number]; index: number; onClick: () => void }) {
  return (
    <button className="chart-focus grid w-full grid-cols-[1.5rem_minmax(0,1fr)_auto] items-center gap-3 rounded-lg text-left" onClick={onClick} type="button">
      <span className="text-sm text-muted-foreground">{index + 1}</span>
      <span><span className="mb-1 flex items-center justify-between gap-3"><strong>{category.path}</strong><span className="tabular-nums">{formatCents(category.amountCents)}</span></span><span className="block h-2 overflow-hidden rounded-full bg-secondary"><span className="block h-full rounded-full" style={{ background: category.color, width: `${Math.max(4, Math.min(100, category.amountCents / 10_00))}%` }} /></span></span>
      <ArrowRight className="size-4" />
    </button>
  )
}

function OverviewSkeleton() {
  return <div className="page-wrap space-y-8" aria-label="Caricamento panoramica"><Skeleton className="h-12 w-56" /><Skeleton className="h-36 max-w-3xl" /><Skeleton className="h-44 w-full" /><Skeleton className="h-72 w-full" /></div>
}

function ErrorState({ retry }: { retry: () => void }) {
  return <div className="surface mx-auto max-w-lg p-8 text-center" role="alert"><AlertTriangle className="mx-auto mb-3" /><h1 className="font-display text-2xl">Panoramica non disponibile</h1><p className="mt-2 text-muted-foreground">I dati restano al sicuro. Riprova tra poco.</p><Button className="mt-5" onClick={retry}>Riprova</Button></div>
}

function EmptyInline({ text }: { text: string }) {
  return <div className="rounded-2xl border border-dashed p-8 text-center text-muted-foreground">{text}</div>
}

function monthLabel(month: string): string {
  const [year, number] = month.split("-").map(Number)
  return new Intl.DateTimeFormat("it-IT", { month: "long", year: "numeric", timeZone: "Europe/Rome" }).format(new Date(Date.UTC(year!, number! - 1, 1)))
}

function shiftMonth(month: string, delta: number): string {
  const [year, number] = month.split("-").map(Number)
  return new Date(Date.UTC(year!, number! - 1 + delta, 1)).toISOString().slice(0, 7)
}
