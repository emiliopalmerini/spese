import { useQuery } from "@tanstack/react-query"
import { ArrowRight, CalendarRange, TrendingDown, TrendingUp } from "lucide-react"
import { useNavigate, useSearchParams } from "react-router"
import { Bar, BarChart, CartesianGrid, Cell, ComposedChart, Line, ReferenceLine, XAxis, YAxis } from "recharts"

import { Button } from "@/components/ui/button"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { Input } from "@/components/ui/input"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api, type CategoryMetric } from "@/lib/api"
import { formatCents } from "@/lib/format"

interface Flow { month: string; incomeCents: number; expenseCents: number; refundCents: number; netCents: number; drilldown: string }

const flowConfig = {
  income: { label: "Entrate", color: "#6A7B35" },
  expense: { label: "Uscite", color: "#9A4F66" },
  net: { label: "Netto", color: "#2E6F95" },
} satisfies ChartConfig

export function AnalyticsPage() {
  const navigate = useNavigate()
  const [search, setSearch] = useSearchParams()
  const area = search.get("area") ?? "spese"
  const from = search.get("from") ?? new Date(Date.UTC(new Date().getUTCFullYear(), new Date().getUTCMonth() - 11, 1)).toISOString().slice(0, 10)
  const to = search.get("to") ?? new Date().toISOString().slice(0, 10)
  const range = `from=${from}&to=${to}`
  const flow = useQuery({ queryKey: ["analytics-flow", range], queryFn: () => api<{ data: Flow[] }>(`/api/v1/analytics/cash-flow?${range}`) })
  const categories = useQuery({ queryKey: ["analytics-categories", range], queryFn: () => api<{ data: CategoryMetric[] }>(`/api/v1/analytics/categories?${range}`) })
  const netWorth = useQuery({ queryKey: ["analytics-net-worth", range], queryFn: () => api<{ data: { netWorthCents: number; accounts: Array<{ id: string; name: string; type: string; balance: { balanceCents: number } }>; markers: Array<{ date: string; balanceCents: number }> } }>(`/api/v1/analytics/net-worth?${range}`) })
  const forecast = useQuery({ queryKey: ["forecast", range], queryFn: () => api<{ data: { certainCents: number; estimatedCents: number; items: unknown[] } }>(`/api/v1/analytics/recurring-forecast?${range}`) })

  const setParam = (key: string, value: string) => { const next = new URLSearchParams(search); next.set(key, value); setSearch(next) }

  return (
    <div className="page-wrap space-y-7">
      <header><p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Esplora</p><h1 className="mt-1 font-display text-4xl font-semibold">Analisi</h1><p className="mt-2 max-w-2xl text-muted-foreground">Ogni segno apre i movimenti che lo compongono. I filtri restano nell’URL.</p></header>
      <div className="surface flex flex-wrap items-center gap-3 p-3">
        <CalendarRange className="ml-2 size-5 text-muted-foreground" />
        <Input aria-label="Dal" className="w-auto" onChange={(event) => setParam("from", event.target.value)} type="date" value={from} />
        <span className="text-muted-foreground">→</span>
        <Input aria-label="Al" className="w-auto" onChange={(event) => setParam("to", event.target.value)} type="date" value={to} />
        <Button className="ml-auto" variant="outline">Confronta <span className="hidden sm:inline">con periodo precedente</span></Button>
      </div>

      <Tabs onValueChange={(value) => setParam("area", value)} value={area}>
        <TabsList aria-label="Area di analisi"><TabsTrigger value="spese">Spese</TabsTrigger><TabsTrigger value="flusso">Flusso</TabsTrigger><TabsTrigger value="patrimonio">Patrimonio</TabsTrigger></TabsList>
        <TabsContent value="spese">
          <section className="surface p-4 sm:p-6">
            <div className="mb-6"><p className="text-sm text-muted-foreground">Composizione</p><h2 className="font-display text-2xl font-semibold">Spese per categoria</h2></div>
            {categories.isLoading ? <div className="h-72 animate-pulse rounded-xl bg-muted" /> : categories.data?.data.length ? <>
              <ChartContainer className="h-[22rem]" config={{ category: { label: "Spese", color: "#2E6F95" } }}>
                <BarChart data={categories.data.data} layout="vertical" margin={{ left: 12, right: 30 }}>
                  <CartesianGrid horizontal={false} />
                  <XAxis hide type="number" />
                  <YAxis dataKey="path" tickLine={false} type="category" width={115} />
                  <ChartTooltip content={<ChartTooltipContent formatter={(value) => formatCents(Number(value))} />} cursor={{ fill: "rgba(30,27,28,.05)" }} />
                  <Bar dataKey="amountCents" name="Spese" onClick={(entry) => { const category = (entry as unknown as { payload?: CategoryMetric }).payload; if (category) navigate(category.drilldown) }} radius={[0, 6, 6, 0]}>{categories.data.data.map((entry) => <Cell className="chart-focus cursor-pointer" fill={entry.color} key={entry.id} tabIndex={0} />)}</Bar>
                </BarChart>
              </ChartContainer>
              <AccessibleCategoryTable data={categories.data.data} onOpen={(path) => navigate(path)} />
            </> : <EmptyChart text="Nessuna spesa nel periodo selezionato." />}
          </section>
        </TabsContent>

        <TabsContent value="flusso">
          <section className="surface p-4 sm:p-6">
            <div className="mb-6 grid gap-4 sm:grid-cols-[1fr_auto]"><div><p className="text-sm text-muted-foreground">Entrate, uscite e netto</p><h2 className="font-display text-2xl font-semibold">Flusso mensile</h2></div><div className="flex gap-4 text-sm"><span className="flex items-center gap-1"><TrendingUp className="size-4 text-[#6A7B35]" /> Entrate</span><span className="flex items-center gap-1"><TrendingDown className="size-4 text-[#9A4F66]" /> Uscite</span></div></div>
            {flow.data?.data.length ? <>
              <ChartContainer className="h-[22rem]" config={flowConfig}>
                <ComposedChart data={flow.data.data} margin={{ left: 5, right: 8 }}>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey="month" tickLine={false} />
                  <YAxis tickFormatter={(value: number) => `${Math.round(value / 100_00)}k`} tickLine={false} width={42} />
                  <ReferenceLine stroke="rgba(30,27,28,.35)" y={0} />
                  <ChartTooltip content={<ChartTooltipContent formatter={(value) => formatCents(Number(value))} />} />
                  <Bar dataKey="incomeCents" fill="var(--color-income)" name="Entrate" radius={[5, 5, 0, 0]} />
                  <Bar dataKey="expenseCents" fill="var(--color-expense)" name="Uscite" radius={[5, 5, 0, 0]} />
                  <Line dataKey="netCents" dot={{ r: 4, tabIndex: 0 }} name="Netto" stroke="var(--color-net)" strokeWidth={3} />
                </ComposedChart>
              </ChartContainer>
              <table className="mt-6 w-full text-sm"><caption className="sr-only">Valori mensili equivalenti al grafico</caption><thead><tr className="border-b text-left text-muted-foreground"><th className="py-2">Mese</th><th>Entrate</th><th>Uscite</th><th>Netto</th><th /></tr></thead><tbody>{flow.data.data.map((item) => <tr className="border-b" key={item.month}><td className="py-3">{item.month}</td><td>{formatCents(item.incomeCents)}</td><td>{formatCents(item.expenseCents)}</td><td>{formatCents(item.netCents)}</td><td><Button aria-label={`Apri movimenti di ${item.month}`} onClick={() => navigate(item.drilldown)} size="icon" variant="ghost"><ArrowRight /></Button></td></tr>)}</tbody></table>
            </> : <EmptyChart text="Servono movimenti per disegnare il flusso." />}
          </section>
          <section className="mt-5 grid gap-4 sm:grid-cols-2"><Metric label="Forecast certo" value={forecast.data?.data.certainCents ?? 0} /><Metric label="Forecast stimato" value={forecast.data?.data.estimatedCents ?? 0} /></section>
        </TabsContent>

        <TabsContent value="patrimonio">
          <section className="surface-strong overflow-hidden p-5 sm:p-8">
            <p className="text-sm text-[#d4cfc2]">Patrimonio al {to}</p><p className="display-number mt-2 break-words text-5xl text-primary sm:text-6xl">{formatCents(netWorth.data?.data.netWorthCents ?? 0)}</p>
            <div className="mt-8 grid gap-3 sm:grid-cols-2">{netWorth.data?.data.accounts.map((account) => <button className="pressable flex items-center justify-between rounded-xl border border-white/15 px-4 text-left" key={account.id} onClick={() => navigate(`/conti?account=${account.id}`)} type="button"><span>{account.name}<small className="block text-[#aaa59b]">{account.type}</small></span><strong className="tabular-nums">{formatCents(account.balance.balanceCents)}</strong></button>)}</div>
            <p className="mt-6 border-t border-white/15 pt-4 text-sm text-[#d4cfc2]"><span className="mr-2 inline-block size-2 rounded-full bg-primary" /> Marker pieno: saldo reale riconciliato. Dopo l’ultimo marker il valore è calcolato dai posting.</p>
          </section>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function AccessibleCategoryTable({ data, onOpen }: { data: CategoryMetric[]; onOpen: (path: string) => void }) {
  return <table className="mt-6 w-full text-sm"><caption className="sr-only">Valori equivalenti del grafico categorie</caption><thead><tr className="border-b text-left text-muted-foreground"><th className="py-2">Categoria</th><th>Importo</th><th>Movimenti</th><th /></tr></thead><tbody>{data.map((item) => <tr className="border-b" key={item.id}><td className="py-3 font-semibold">{item.path}</td><td className="tabular-nums">{formatCents(item.amountCents)}</td><td>{item.movementCount}</td><td><Button aria-label={`Apri ${item.path}`} onClick={() => onOpen(item.drilldown)} size="icon" variant="ghost"><ArrowRight /></Button></td></tr>)}</tbody></table>
}

function Metric({ label, value }: { label: string; value: number }) { return <div className="surface p-5"><p className="text-sm text-muted-foreground">{label}</p><p className="display-number mt-2 text-3xl">{formatCents(value)}</p></div> }
function EmptyChart({ text }: { text: string }) { return <div className="flex h-64 items-center justify-center rounded-xl border border-dashed text-muted-foreground">{text}</div> }
