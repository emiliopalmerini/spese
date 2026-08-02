import { useQuery } from "@tanstack/react-query"
import { ArrowRight, FolderTree, GripVertical, Merge, Plus, Tag } from "lucide-react"
import { useNavigate } from "react-router"

import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api, type Category, type CategoryMetric } from "@/lib/api"
import { formatCents } from "@/lib/format"

export function CategoriesPage() {
  const navigate = useNavigate()
  const categories = useQuery({ queryKey: ["categories", "all"], queryFn: () => api<{ data: Category[] }>("/api/v1/categories") })
  const metrics = useQuery({ queryKey: ["analytics-categories", "categories-page"], queryFn: () => api<{ data: CategoryMetric[] }>("/api/v1/analytics/categories") })
  const metricByID = new Map(metrics.data?.data.map((metric) => [metric.id, metric]))
  return (
    <div className="page-wrap space-y-7">
      <header className="flex flex-wrap items-end justify-between gap-4"><div><p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Tassonomia</p><h1 className="mt-1 font-display text-4xl font-semibold">Categorie</h1><p className="mt-2 max-w-2xl text-muted-foreground">Due livelli, colori stabili e regole esplicite. Lo storico non cambia mai identità durante una rinomina.</p></div><Button><Plus /> Nuova categoria</Button></header>
      <button className="surface-strong pressable flex w-full items-center justify-between gap-4 p-5 text-left" onClick={() => navigate("/movimenti?queue=uncategorized")} type="button"><span className="flex items-center gap-3"><span className="flex size-11 items-center justify-center rounded-full bg-primary text-foreground"><Tag /></span><span><strong className="block font-display text-xl">Da categorizzare</strong><small className="text-[#d4cfc2]">Movimenti assegnati solo a una categoria primaria</small></span></span><ArrowRight /></button>
      <Tabs defaultValue="expense"><TabsList><TabsTrigger value="expense">Spese</TabsTrigger><TabsTrigger value="income">Entrate</TabsTrigger></TabsList>{(["expense", "income"] as const).map((kind) => <TabsContent key={kind} value={kind}><CategoryTree categories={categories.data?.data.filter((category) => category.kind === kind) ?? []} metricByID={metricByID} /></TabsContent>)}</Tabs>
    </div>
  )
}

function CategoryTree({ categories, metricByID }: { categories: Category[]; metricByID: Map<string, CategoryMetric> }) {
  const parents = categories.filter((category) => !category.parentId)
  if (!parents.length) return <div className="rounded-2xl border border-dashed p-12 text-center"><FolderTree className="mx-auto mb-3" /><h2 className="font-display text-2xl">Nessuna categoria</h2></div>
  return <div className="space-y-4">{parents.map((parent) => { const children = categories.filter((category) => category.parentId === parent.id); const parentMetric = metricByID.get(parent.id); return <section className="surface overflow-hidden" key={parent.id}><div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 p-4 sm:p-5"><GripVertical className="size-5 text-muted-foreground" /><div><h2 className="flex items-center gap-2 font-semibold"><span className="size-3 rounded-full" style={{ background: parent.color }} />{parent.name}</h2><p className="mt-1 text-sm text-muted-foreground">{parentMetric?.movementCount ?? 0} movimenti · {parentMetric?.merchantRuleCount ?? 0} regole esercente</p></div><div className="text-right"><strong className="tabular-nums">{formatCents(parentMetric?.amountCents ?? 0)}</strong><small className="block text-muted-foreground">questo mese</small></div></div>{children.length ? <div className="border-t bg-background/50 pl-8 sm:pl-14">{children.map((child) => { const metric = metricByID.get(child.id); return <button className="pressable grid w-full grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-4 border-b px-4 text-left last:border-0" key={child.id} type="button"><span>{child.name}<small className="block text-muted-foreground">Ultimo uso {metric?.lastUsed || "—"}</small></span><span className="tabular-nums">{formatCents(metric?.amountCents ?? 0)}</span><ArrowRight className="size-4" /></button> })}</div> : null}<div className="flex flex-wrap gap-2 border-t p-3"><Button size="sm" variant="ghost"><Plus /> Sottocategoria</Button><Button size="sm" variant="ghost"><Merge /> Unisci</Button></div></section> })}</div>
}
