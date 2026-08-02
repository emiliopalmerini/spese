import {
  ChartNoAxesCombined,
  CircleDollarSign,
  FolderTree,
  House,
  Landmark,
  MoreHorizontal,
  Plus,
  Repeat2,
  Settings,
  WalletCards,
} from "lucide-react"
import { lazy, Suspense, useState } from "react"
import { NavLink, Navigate, Route, Routes } from "react-router"

import { Button } from "@/components/ui/button"
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle, DrawerTrigger } from "@/components/ui/drawer"
import { cn } from "@/lib/utils"

const QuickAdd = lazy(() => import("@/features/quick-add/quick-add").then((module) => ({ default: module.QuickAdd })))
const AccountsPage = lazy(() => import("@/pages/accounts").then((module) => ({ default: module.AccountsPage })))
const AnalyticsPage = lazy(() => import("@/pages/analytics").then((module) => ({ default: module.AnalyticsPage })))
const CategoriesPage = lazy(() => import("@/pages/categories").then((module) => ({ default: module.CategoriesPage })))
const MovementsPage = lazy(() => import("@/pages/movements").then((module) => ({ default: module.MovementsPage })))
const OverviewPage = lazy(() => import("@/pages/overview").then((module) => ({ default: module.OverviewPage })))
const RecurringPage = lazy(() => import("@/pages/recurring").then((module) => ({ default: module.RecurringPage })))
const SettingsPage = lazy(() => import("@/pages/settings").then((module) => ({ default: module.SettingsPage })))

const primaryNavigation = [
  { to: "/", label: "Panoramica", icon: House, end: true },
  { to: "/movimenti", label: "Movimenti", icon: WalletCards },
  { to: "/analisi", label: "Analisi", icon: ChartNoAxesCombined },
]

const secondaryNavigation = [
  { to: "/ricorrenti", label: "Ricorrenti", icon: Repeat2 },
  { to: "/conti", label: "Conti", icon: Landmark },
  { to: "/categorie", label: "Categorie", icon: FolderTree },
  { to: "/impostazioni", label: "Impostazioni", icon: Settings },
]

export function App() {
  const [quickAddOpen, setQuickAddOpen] = useState(false)
  return (
    <div className="app-shell">
      <aside className="sticky top-0 hidden h-screen flex-col border-r bg-card/70 p-4 backdrop-blur-md md:flex">
        <NavLink className="mb-8 flex min-h-11 items-center gap-3 px-2 font-display text-2xl font-semibold" to="/">
          <span className="flex size-9 items-center justify-center rounded-xl bg-primary text-foreground"><CircleDollarSign className="size-5" /></span>
          Spese
        </NavLink>
        <Button className="mb-6 w-full justify-start" onClick={() => setQuickAddOpen(true)} size="lg"><Plus /> Nuovo movimento</Button>
        <nav aria-label="Navigazione principale" className="space-y-1">
          {[...primaryNavigation, ...secondaryNavigation].map((item) => <SidebarLink key={item.to} {...item} />)}
        </nav>
      </aside>

      <main className="app-main" id="contenuto" tabIndex={-1}>
        <Suspense fallback={<div className="page-wrap h-64 animate-pulse rounded-2xl bg-muted motion-reduce:animate-none" aria-label="Caricamento pagina" />}>
          <Routes>
            <Route element={<OverviewPage onAdd={() => setQuickAddOpen(true)} />} index />
            <Route element={<MovementsPage onAdd={() => setQuickAddOpen(true)} />} path="movimenti" />
            <Route element={<AnalyticsPage />} path="analisi" />
            <Route element={<RecurringPage />} path="ricorrenti" />
            <Route element={<AccountsPage />} path="conti" />
            <Route element={<CategoriesPage />} path="categorie" />
            <Route element={<SettingsPage />} path="impostazioni" />
            <Route element={<Navigate replace to="/" />} path="*" />
          </Routes>
        </Suspense>
      </main>

      <MobileNavigation onAdd={() => setQuickAddOpen(true)} />
      {quickAddOpen ? <Suspense fallback={null}><QuickAdd open onOpenChange={setQuickAddOpen} /></Suspense> : null}
    </div>
  )
}

function SidebarLink({ to, label, icon: Icon, end }: (typeof primaryNavigation)[number]) {
  return (
    <NavLink
      className={({ isActive }) => cn("pressable flex items-center gap-3 rounded-xl px-3 text-sm font-semibold text-muted-foreground hover:bg-secondary hover:text-foreground", isActive && "bg-foreground text-card hover:bg-foreground hover:text-card")}
      end={end}
      to={to}
    >
      <Icon className="size-5" /> {label}
    </NavLink>
  )
}

function MobileNavigation({ onAdd }: { onAdd: () => void }) {
  return (
    <nav aria-label="Navigazione mobile" className="mobile-nav fixed inset-x-0 bottom-0 z-40 grid grid-cols-5 border-t bg-card/95 px-1 pt-2 backdrop-blur-md md:hidden">
      <MobileLink {...primaryNavigation[0]!} />
      <MobileLink {...primaryNavigation[1]!} />
      <button className="pressable mobile-nav-add mx-auto flex size-14 items-center justify-center rounded-full bg-primary text-foreground shadow-md" onClick={onAdd} type="button"><Plus className="size-6" /><span className="sr-only">Nuovo movimento</span></button>
      <MobileLink {...primaryNavigation[2]!} />
      <Drawer>
        <DrawerTrigger asChild>
          <button className="pressable flex flex-col items-center justify-center gap-1 text-[0.68rem] font-semibold text-muted-foreground" type="button"><MoreHorizontal className="size-5" />Altro</button>
        </DrawerTrigger>
        <DrawerContent>
          <DrawerHeader><DrawerTitle>Altro</DrawerTitle><DrawerDescription>Gestione e impostazioni</DrawerDescription></DrawerHeader>
          <div className="grid gap-2 px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
            {secondaryNavigation.map(({ to, label, icon: Icon }) => (
              <NavLink className="pressable flex items-center gap-3 rounded-xl border px-4 font-semibold" key={to} to={to}><Icon className="size-5" />{label}</NavLink>
            ))}
          </div>
        </DrawerContent>
      </Drawer>
    </nav>
  )
}

function MobileLink({ to, label, icon: Icon, end }: (typeof primaryNavigation)[number]) {
  return (
    <NavLink className={({ isActive }) => cn("pressable flex flex-col items-center justify-center gap-1 text-[0.68rem] font-semibold text-muted-foreground", isActive && "text-foreground")} end={end} to={to}>
      <Icon className="size-5" />{label}
    </NavLink>
  )
}
