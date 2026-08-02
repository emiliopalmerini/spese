import { Database, Download, Globe2, LockKeyhole, Radio } from "lucide-react"

import { Button } from "@/components/ui/button"

export function SettingsPage() {
  return (
    <div className="page-wrap max-w-4xl space-y-8">
      <header><p className="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">Sistema</p><h1 className="mt-1 font-display text-4xl font-semibold">Impostazioni</h1></header>
      <section className="surface divide-y">
        <Setting icon={<Globe2 />} label="Lingua e formato" value="Italiano · EUR · Europe/Rome" />
        <Setting icon={<Database />} label="Archivio principale" value="SQLite locale" />
        <Setting icon={<Radio />} label="Mirror esterno" value="Google Sheets via outbox e RabbitMQ" />
      </section>
      <section className="surface-strong p-6"><div className="flex items-start gap-4"><LockKeyhole className="mt-1 shrink-0 text-primary" /><div><h2 className="font-display text-2xl">Accesso single-user</h2><p className="mt-2 text-[#d4cfc2]">Spese non implementa account applicativi o multi-tenancy. In produzione deve restare dietro un reverse proxy autenticato e TLS.</p></div></div></section>
      <section className="section-rule"><h2 className="font-display text-2xl font-semibold">Dati e ripristino</h2><p className="mt-2 text-muted-foreground">Il backup SQLite è l’unità di rollback. Il foglio è un export derivato, non una sorgente da cui calcolare i saldi.</p><Button className="mt-4" variant="outline"><Download /> Procedura di backup</Button></section>
    </div>
  )
}

function Setting({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-4 p-5"><span className="flex size-11 items-center justify-center rounded-xl bg-secondary">{icon}</span><div><h2 className="font-semibold">{label}</h2><p className="mt-1 text-sm text-muted-foreground">{value}</p></div></div>
}
