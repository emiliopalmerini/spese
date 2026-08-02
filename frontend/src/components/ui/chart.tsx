import { ResponsiveContainer, Tooltip } from "recharts"
import type * as React from "react"

import { cn } from "@/lib/utils"

type ChartConfig = Record<string, { label: string; color: string }>

function ChartContainer({ config, className, children, ...props }: React.ComponentProps<"div"> & { config: ChartConfig }) {
  const style = Object.fromEntries(Object.entries(config).map(([key, item]) => [`--color-${key}`, item.color])) as React.CSSProperties
  return (
    <div className={cn("min-h-64 w-full text-xs [&_.recharts-cartesian-axis-tick_text]:fill-muted-foreground [&_.recharts-cartesian-grid_line]:stroke-border", className)} style={style} {...props}>
      <ResponsiveContainer>{children}</ResponsiveContainer>
    </div>
  )
}

const ChartTooltip = Tooltip

interface ChartTooltipContentProps {
  active?: boolean
  payload?: ReadonlyArray<{ dataKey?: string | number; name?: string | number; value?: number | string }>
  label?: string | number
  formatter?: (value: number) => React.ReactNode
}

function ChartTooltipContent({ active, payload, label, formatter }: ChartTooltipContentProps) {
  if (!active || !payload?.length) return null
  return (
    <div className="min-w-36 rounded-xl border bg-popover p-3 text-sm shadow-lg" role="status">
      {label ? <p className="mb-2 font-semibold">{String(label)}</p> : null}
      <div className="space-y-1">
        {payload.map((item) => (
          <div className="flex items-center justify-between gap-4" key={String(item.dataKey)}>
            <span className="text-muted-foreground">{item.name}</span>
            <span className="tabular-nums font-semibold">{formatter ? formatter(Number(item.value)) : String(item.value)}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

export { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig }
