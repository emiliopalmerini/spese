import { zodResolver } from "@hookform/resolvers/zod"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ChevronDown, Mic, Plus, Sparkles, Split } from "lucide-react"
import { useDeferredValue, useEffect, useState, useSyncExternalStore } from "react"
import { useForm } from "react-hook-form"
import { toast } from "sonner"
import { z } from "zod"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from "@/components/ui/drawer"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Textarea } from "@/components/ui/textarea"
import { api, type Account, type Category, type Movement, mutationHeaders } from "@/lib/api"
import { formatCents, parseItalianCents, todayInRome } from "@/lib/format"
import { cn } from "@/lib/utils"

import { DictationPanel } from "./dictation"

const schema = z.object({
  kind: z.enum(["expense", "income", "transfer"]),
  amount: z.string().min(1, "Inserisci un importo"),
  merchant: z.string().trim().max(300),
  description: z.string().trim().max(500),
  categoryId: z.string(),
  accountId: z.string().min(1, "Seleziona un conto"),
  destinationAccountId: z.string(),
  date: z.string().date(),
  note: z.string().max(2000),
  recurring: z.boolean(),
})

type FormValues = z.infer<typeof schema>

interface QuickAddProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function QuickAdd({ open, onOpenChange }: QuickAddProps) {
  const mobile = useMediaQuery("(max-width: 767px)")
  const content = <QuickAddForm onSaved={() => onOpenChange(false)} />
  if (mobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>Nuovo movimento</DrawerTitle>
            <DrawerDescription>Registra una spesa in pochi secondi.</DrawerDescription>
          </DrawerHeader>
          {content}
        </DrawerContent>
      </Drawer>
    )
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Nuovo movimento</SheetTitle>
          <SheetDescription>Registra una spesa in pochi secondi.</SheetDescription>
        </SheetHeader>
        {content}
      </SheetContent>
    </Sheet>
  )
}

function QuickAddForm({ onSaved }: { onSaved: () => void }) {
  const queryClient = useQueryClient()
  const [saveAnother, setSaveAnother] = useState(false)
  const [dictationOpen, setDictationOpen] = useState(false)
  const accountsQuery = useQuery({ queryKey: ["accounts"], queryFn: () => api<{ data: Account[] }>("/api/v1/accounts") })
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      kind: "expense",
      amount: "",
      merchant: "",
      description: "",
      categoryId: "",
      accountId: "",
      destinationAccountId: "",
      date: todayInRome(),
      note: "",
      recurring: false,
    },
  })
  const kind = form.watch("kind")
  const merchant = useDeferredValue(form.watch("merchant"))
  const categoriesQuery = useQuery({
    queryKey: ["categories", kind],
    queryFn: () => api<{ data: Category[] }>(`/api/v1/categories?kind=${kind === "income" ? "income" : "expense"}`),
    enabled: kind !== "transfer",
  })
  const suggestionsQuery = useQuery({
    queryKey: ["merchant-suggestions", merchant],
    queryFn: () => api<{ data: Array<{ accountId: string; categoryId: string; reason: string }> }>(`/api/v1/suggestions/merchant?merchant=${encodeURIComponent(merchant)}`),
    enabled: merchant.trim().length >= 2,
  })

  useEffect(() => {
    const first = (accountsQuery.data?.data ?? [])[0]
    if (first && !form.getValues("accountId")) form.setValue("accountId", first.id)
  }, [accountsQuery.data, form])

  const mutation = useMutation({
    mutationFn: async (values: FormValues) => {
      const amountCents = parseItalianCents(values.amount)
      if (amountCents <= 0) throw new Error("L'importo deve essere positivo")
      const body = {
        kind: values.kind,
        status: "posted",
        date: values.date,
        accountId: values.accountId,
        destinationAccountId: values.kind === "transfer" ? values.destinationAccountId : "",
        amountCents,
        merchant: values.merchant,
        description: values.description,
        note: values.note,
        origin: "manual",
        allocations: values.kind === "transfer" || !values.categoryId ? [] : [{ categoryId: values.categoryId, amountCents }],
      }
      return api<{ data: Movement }>("/api/v1/movements", {
        method: "POST",
        headers: mutationHeaders(),
        body: JSON.stringify(body),
      })
    },
    onSuccess: ({ data }) => {
      void queryClient.invalidateQueries({ queryKey: ["movements"] })
      void queryClient.invalidateQueries({ queryKey: ["overview"] })
      toast.success(`${data.merchant || movementLabel(data.kind)} · ${formatCents(data.amountCents)}`, {
        description: "Movimento registrato",
        action: {
          label: "Annulla",
          onClick: () => {
            void api(`/api/v1/movements/${data.id}`, {
              method: "DELETE",
              headers: mutationHeaders(data.version),
              body: JSON.stringify({ reason: "Annullato dal toast" }),
            }).then(() => queryClient.invalidateQueries({ queryKey: ["movements"] }))
          },
        },
      })
      if (saveAnother) {
        const accountId = form.getValues("accountId")
        form.reset({ ...form.formState.defaultValues, accountId, date: todayInRome(), kind: "expense" })
        setSaveAnother(false)
        requestAnimationFrame(() => document.querySelector<HTMLInputElement>("#quick-amount")?.focus())
      } else {
        onSaved()
      }
    },
  })

  const submit = form.handleSubmit((values) => mutation.mutate(values))
  const selectedAccount = (accountsQuery.data?.data ?? []).find((account) => account.id === form.watch("accountId"))
  const selectedCategory = (categoriesQuery.data?.data ?? []).find((category) => category.id === form.watch("categoryId"))

  return (
    <form
      className="flex min-h-0 flex-1 flex-col overflow-y-auto px-5 pb-[max(1.25rem,env(safe-area-inset-bottom))]"
      onSubmit={(event) => void submit(event)}
      onKeyDown={(event) => {
        if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
          event.preventDefault()
          void submit()
        }
      }}
    >
      <div className="space-y-5 py-5">
        <fieldset>
          <legend className="sr-only">Tipo di movimento</legend>
          <div className="grid grid-cols-3 rounded-full bg-secondary p-1">
            {(["expense", "income", "transfer"] as const).map((value) => (
              <button
                className={cn("pressable rounded-full px-2 text-sm font-semibold", kind === value && "bg-foreground text-card")}
                key={value}
                onClick={() => form.setValue("kind", value)}
                type="button"
              >
                {{ expense: "Spesa", income: "Entrata", transfer: "Trasferimento" }[value]}
              </button>
            ))}
          </div>
        </fieldset>

        <div>
          <Label htmlFor="quick-amount">Importo</Label>
          <div className="relative mt-2">
            <Input
              autoComplete="off"
              autoFocus
              className="display-number h-16 pr-12 text-4xl"
              id="quick-amount"
              inputMode="decimal"
              placeholder="0,00"
              {...form.register("amount")}
            />
            <span className="absolute right-4 top-1/2 -translate-y-1/2 text-lg text-muted-foreground">€</span>
          </div>
          {form.formState.errors.amount ? <p className="mt-1 text-sm text-destructive">{form.formState.errors.amount.message}</p> : null}
        </div>

        {kind !== "transfer" ? (
          <div>
            <Label htmlFor="quick-merchant">{kind === "income" ? "Provenienza del denaro" : "Esercente"}</Label>
            <Input className="mt-2" id="quick-merchant" placeholder={kind === "income" ? "Es. Azienda o cliente" : "Es. Forno Roscioli"} {...form.register("merchant")} />
          </div>
        ) : null}

        <div>
          <Label htmlFor="quick-description">Descrizione</Label>
          <Input className="mt-2" id="quick-description" placeholder={kind === "transfer" ? "Es. Giroconto risparmi" : "Es. Cena con amici"} {...form.register("description")} />
        </div>

        {(suggestionsQuery.data?.data ?? []).map((suggestion) => (
          <button
            className="flex min-h-11 w-full items-start gap-3 rounded-xl border bg-[#fff6d7] p-3 text-left"
            key={`${suggestion.accountId}-${suggestion.categoryId}`}
            onClick={() => {
              form.setValue("accountId", suggestion.accountId)
              form.setValue("categoryId", suggestion.categoryId)
            }}
            type="button"
          >
            <Sparkles className="mt-0.5 size-4 shrink-0" />
            <span><strong>Suggerimento recente</strong><br /><span className="text-sm text-muted-foreground">{suggestion.reason}</span></span>
          </button>
        ))}

        {kind !== "transfer" ? (
          <CategoryPicker
            categories={categoriesQuery.data?.data ?? []}
            loading={categoriesQuery.isLoading}
            onChange={(id) => form.setValue("categoryId", id, { shouldValidate: true })}
            selected={selectedCategory}
          />
        ) : (
          <div>
            <Label>Conto destinazione</Label>
            <Select onValueChange={(id) => form.setValue("destinationAccountId", id)} value={form.watch("destinationAccountId")}>
              <SelectTrigger className="mt-2"><SelectValue placeholder="Scegli il conto" /></SelectTrigger>
              <SelectContent>
                {(accountsQuery.data?.data ?? []).filter((account) => account.id !== form.watch("accountId")).map((account) => <SelectItem key={account.id} value={account.id}>{account.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        )}

        <div className="flex min-h-11 items-center justify-between rounded-xl bg-secondary/70 px-3 text-sm">
          <span>{form.watch("date") === todayInRome() ? "Oggi" : form.watch("date")} · {selectedAccount?.name ?? "Scegli conto"}</span>
          <ChevronDown className="size-4" />
        </div>

        <details className="group rounded-xl border bg-card">
          <summary className="pressable flex cursor-pointer list-none items-center justify-between px-4 font-semibold">Altri dettagli <Plus className="size-4 group-open:rotate-45" /></summary>
          <div className="space-y-4 border-t p-4">
            <div><Label htmlFor="quick-date">Data</Label><Input className="mt-2" id="quick-date" type="date" {...form.register("date")} /></div>
            <div>
              <Label>Conto</Label>
              <Select onValueChange={(id) => form.setValue("accountId", id)} value={form.watch("accountId")}>
                <SelectTrigger className="mt-2"><SelectValue placeholder="Scegli il conto" /></SelectTrigger>
                <SelectContent>{(accountsQuery.data?.data ?? []).map((account) => <SelectItem key={account.id} value={account.id}>{account.name}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div><Label htmlFor="quick-note">Note</Label><Textarea className="mt-2" id="quick-note" {...form.register("note")} /></div>
            {kind !== "transfer" ? <Button className="w-full" type="button" variant="outline"><Split /> Suddividi tra categorie</Button> : null}
            <label className="flex min-h-11 items-center gap-3"><input className="size-5 accent-[#1e1b1c]" type="checkbox" {...form.register("recurring")} /> Rendi ricorrente</label>
          </div>
        </details>

        <Button className="w-full" onClick={() => setDictationOpen(true)} type="button" variant="outline"><Mic /> Dettatura multipla</Button>
        {mutation.error ? <div className="rounded-xl border border-destructive/30 bg-red-50 p-3 text-sm text-destructive" role="alert">{mutation.error.message}</div> : null}
      </div>

      <div className="sticky bottom-0 mt-auto grid gap-2 border-t bg-card/95 py-4 backdrop-blur-sm sm:grid-cols-[1fr_auto]">
        <Button disabled={mutation.isPending} size="lg" type="submit">{mutation.isPending ? "Salvataggio…" : "Salva"}</Button>
        <Button disabled={mutation.isPending} onClick={() => { setSaveAnother(true); void submit() }} type="button" variant="outline">Salva e aggiungi</Button>
      </div>
      <DictationPanel open={dictationOpen} onOpenChange={setDictationOpen} />
    </form>
  )
}

function CategoryPicker({ categories, loading, selected, onChange }: { categories: Category[]; loading: boolean; selected?: Category; onChange: (id: string) => void }) {
  const [open, setOpen] = useState(false)
  const parents = new Map(categories.filter((category) => !category.parentId).map((category) => [category.id, category.name]))
  return (
    <div>
      <Label>Categoria</Label>
      <div className="mt-2 flex flex-wrap gap-2">
        {categories.slice(0, 4).map((category) => (
          <Button key={category.id} onClick={() => onChange(category.id)} size="sm" type="button" variant={selected?.id === category.id ? "charcoal" : "outline"}>
            <span aria-hidden style={{ color: category.color }}>●</span> {category.name}
          </Button>
        ))}
      </div>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild><Button className="mt-2 w-full justify-between" type="button" variant="outline">{selected ? selected.name : loading ? "Caricamento…" : "Tutte le categorie"}<ChevronDown /></Button></PopoverTrigger>
        <PopoverContent className="p-0" align="start">
          <Command>
            <CommandInput placeholder="Cerca Casa › Utenze…" />
            <CommandList>
              <CommandEmpty>Nessuna categoria trovata.</CommandEmpty>
              <CommandGroup heading="Gerarchia completa">
                {categories.map((category) => (
                  <CommandItem key={category.id} onSelect={() => { onChange(category.id); setOpen(false) }} value={`${parents.get(category.parentId ?? "") ?? ""} ${category.name}`}>
                    <span aria-hidden style={{ color: category.color }}>●</span>
                    {category.parentId ? `${parents.get(category.parentId) ?? ""} › ` : ""}{category.name}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  )
}

function movementLabel(kind: Movement["kind"]): string {
  return { expense: "Spesa", income: "Entrata", refund: "Rimborso", transfer: "Trasferimento", adjustment: "Rettifica" }[kind]
}

function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (callback) => {
      const media = window.matchMedia(query)
      media.addEventListener("change", callback)
      return () => media.removeEventListener("change", callback)
    },
    () => window.matchMedia(query).matches,
    () => false,
  )
}
