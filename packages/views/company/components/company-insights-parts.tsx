"use client";

import type { ReactNode } from "react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { Progress } from "@multica/ui/components/ui/progress";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@multica/ui/components/ui/chart";
import { cn } from "@multica/ui/lib/utils";

const costChartConfig = { cost: { label: "USD", color: "var(--chart-1)" } } satisfies ChartConfig;

const usd = new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 2 });
const usdWhole = new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 0 });
export const formatUsd = (value: number) => (Math.abs(value) >= 1000 ? usdWhole : usd).format(value);
export const percent = (share: number) => (Number.isFinite(share) ? `${Math.round(share * 100)}%` : "∞");

export function Kpi({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: "warning" | "danger";
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border p-4">
      <dt className="truncate text-xs text-muted-foreground">{label}</dt>
      <dd
        className={cn(
          "truncate text-2xl font-semibold tabular-nums",
          tone === "warning" && "text-warning",
          tone === "danger" && "text-destructive",
        )}
      >
        {value}
      </dd>
      {hint ? <p className="truncate text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}

export function Card({ title, className, children }: { title: string; className?: string; children: ReactNode }) {
  return (
    <section className={cn("flex min-w-0 flex-col gap-3 rounded-lg border p-4", className)}>
      <h3 className="text-sm font-medium">{title}</h3>
      {children}
    </section>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>;
}

export function CostBars({ lines }: { lines: { id: string; name: string; cost: number }[] }) {
  return (
    <ChartContainer config={costChartConfig} className="w-full" style={{ height: Math.max(80, lines.length * 36 + 16) }}>
      <BarChart data={lines} layout="vertical" margin={{ left: 0, right: 8, top: 0, bottom: 0 }}>
        <CartesianGrid horizontal={false} />
        <XAxis type="number" tickLine={false} axisLine={false} tickFormatter={(v: number) => formatUsd(v)} />
        <YAxis dataKey="name" type="category" tickLine={false} axisLine={false} width={96} />
        <ChartTooltip
          content={<ChartTooltipContent formatter={(value) => (typeof value === "number" ? formatUsd(value) : String(value))} />}
        />
        <Bar dataKey="cost" fill="var(--color-cost)" radius={4} />
      </BarChart>
    </ChartContainer>
  );
}

export function CostTable<T extends { id: string; name: string; cost: number }>({
  lines,
  total,
  detail,
}: {
  lines: T[];
  total: number;
  detail?: (line: T) => string;
}) {
  return (
    <ul className="flex flex-col gap-2">
      {lines.map((line) => (
        <li key={line.id} className="flex flex-col gap-1">
          <div className="flex items-baseline justify-between gap-2 text-sm">
            <span className="min-w-0 truncate">
              {line.name}
              {detail ? <span className="ml-2 text-xs text-muted-foreground">{detail(line)}</span> : null}
            </span>
            <span className="shrink-0 tabular-nums">{formatUsd(line.cost)}</span>
          </div>
          <Progress value={total > 0 ? (line.cost / total) * 100 : 0} />
        </li>
      ))}
    </ul>
  );
}

export function StackedBar({ segments }: { segments: { key: string; label: string; count: number; className: string }[] }) {
  const total = segments.reduce((sum, s) => sum + s.count, 0);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
        {segments
          .filter((s) => s.count > 0)
          .map((s) => (
            <div key={s.key} className={s.className} style={{ width: `${(s.count / total) * 100}%` }} />
          ))}
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
        {segments.map((s) => (
          <li key={s.key} className="flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", s.className)} />
            <span className="text-muted-foreground">{s.label}</span>
            <span className="tabular-nums">{s.count}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

