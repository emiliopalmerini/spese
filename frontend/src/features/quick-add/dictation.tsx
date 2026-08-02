import { Mic, Square, Trash2 } from "lucide-react"
import { useRef, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { todayInRome } from "@/lib/format"

interface Draft {
  id: string
  kind: string
  date: string
  account: string
  amount: string
  payee: string
  description: string
  category: string
  subcategory: string
  note: string
  issues: string[]
}

export function DictationPanel({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [recording, setRecording] = useState(false)
  const [loading, setLoading] = useState(false)
  const [drafts, setDrafts] = useState<Draft[]>([])
  const recorder = useRef<MediaRecorder | null>(null)
  const chunks = useRef<Blob[]>([])

  const start = async () => {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    const next = new MediaRecorder(stream, { mimeType: supportedMimeType() })
    chunks.current = []
    next.ondataavailable = (event) => { if (event.data.size) chunks.current.push(event.data) }
    next.onstop = () => {
      stream.getTracks().forEach((track) => track.stop())
      void transcribe(new Blob(chunks.current, { type: next.mimeType }))
    }
    next.start()
    recorder.current = next
    setRecording(true)
  }

  const transcribe = async (audio: Blob) => {
    setLoading(true)
    try {
      const data = new FormData()
      data.append("audio", audio, "dettatura.webm")
      const response = await fetch("/api/v1/dictation/fallback", { method: "POST", headers: { "X-Spese-CSRF": "1" }, body: data })
      if (!response.ok) throw new Error("Dettatura non disponibile")
      const result = await response.json() as { extraction?: { movements?: Draft[] } }
      setDrafts(result.extraction?.movements ?? [])
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Dettatura non disponibile")
    } finally {
      setLoading(false)
    }
  }

  const save = async () => {
    const body = new URLSearchParams()
    for (const draft of drafts) {
      for (const key of ["id", "kind", "date", "account", "amount", "payee", "description", "category", "subcategory", "note"] as const) body.append(key, draft[key])
    }
    const response = await fetch("/api/v1/dictation/confirm", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Spese-CSRF": "1" }, body })
    if (!response.ok) {
      toast.error(await response.text())
      return
    }
    toast.success(`Salvati ${drafts.length} movimenti dettati`)
    setDrafts([])
    onOpenChange(false)
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader><SheetTitle>Dettatura multipla</SheetTitle><SheetDescription>Parla liberamente. Prima del salvataggio puoi correggere o eliminare ogni riga.</SheetDescription></SheetHeader>
        <div className="space-y-5 overflow-y-auto p-5">
          <Button className="w-full" disabled={loading} onClick={() => { if (recording) { recorder.current?.stop(); setRecording(false) } else { void start() } }} size="lg" variant={recording ? "destructive" : "charcoal"}>
            {recording ? <><Square /> Ferma e interpreta</> : <><Mic /> Inizia dettatura</>}
          </Button>
          {loading ? <p className="rounded-xl bg-secondary p-4 text-center" role="status">Trascrizione e interpretazione…</p> : null}
          <div className="space-y-4">{drafts.map((draft, index) => <DraftRow draft={draft} index={index} key={draft.id} onChange={(next) => setDrafts((current) => current.map((item) => item.id === next.id ? next : item))} onRemove={() => setDrafts((current) => current.filter((item) => item.id !== draft.id))} />)}</div>
          {drafts.length ? <Button className="w-full" onClick={() => void save()} size="lg">Salva {drafts.length} movimenti</Button> : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function DraftRow({ draft, index, onChange, onRemove }: { draft: Draft; index: number; onChange: (draft: Draft) => void; onRemove: () => void }) {
  const change = (field: keyof Draft, value: string) => onChange({ ...draft, [field]: value })
  return <article className={`rounded-2xl border p-4 ${draft.issues.length ? "border-[#9a4f66] bg-red-50" : "bg-card"}`}><div className="mb-4 flex items-center justify-between"><h2 className="font-semibold">Movimento {index + 1}</h2><Button aria-label={`Elimina movimento ${index + 1}`} onClick={onRemove} size="icon" variant="ghost"><Trash2 /></Button></div>{draft.issues.length ? <ul className="mb-3 text-sm text-destructive">{draft.issues.map((issue) => <li key={issue}>{issue}</li>)}</ul> : null}<div className="grid gap-3 sm:grid-cols-2"><Field label="Importo" onChange={(value) => change("amount", value)} value={draft.amount} /><Field label={draft.kind === "Income" ? "Provenienza del denaro" : "Esercente"} onChange={(value) => change("payee", value)} value={draft.payee} /><Field label="Descrizione" onChange={(value) => change("description", value)} value={draft.description} /><Field label="Categoria" onChange={(value) => change("category", value)} value={draft.category} /><Field label="Sottocategoria" onChange={(value) => change("subcategory", value)} value={draft.subcategory} /><Field label="Conto" onChange={(value) => change("account", value)} value={draft.account} /><Field label="Data" onChange={(value) => change("date", value)} value={draft.date || todayInRome()} /></div></article>
}

function Field({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return <label><Label>{label}</Label><Input className="mt-1" onChange={(event) => onChange(event.target.value)} value={value} /></label>
}

function supportedMimeType(): string {
  for (const value of ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"]) if (MediaRecorder.isTypeSupported(value)) return value
  return ""
}
