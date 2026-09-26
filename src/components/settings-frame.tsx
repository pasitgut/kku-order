"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowsClockwise, Scan, UsersThree } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";

const sections = [
  { group: "ผู้ใช้งาน", items: [{ href: "/settings", label: "ผู้ใช้งานและสิทธิ์", icon: UsersThree }] },
  { group: "การทำงานของระบบ", items: [{ href: "/settings/ocr", label: "OCR และการนำเข้า", icon: Scan }, { href: "/settings/system", label: "sync และการแจ้งเตือน", icon: ArrowsClockwise }] },
];

export function SettingsFrame({ children, actions, note }: { children: React.ReactNode; actions?: React.ReactNode; note: string }) {
  const pathname = usePathname();
  return <AppShell breadcrumb="หน้าหลัก / ตั้งค่า" title="ตั้งค่าระบบ" description="ผู้ใช้งาน สิทธิ์ตามบทบาท และค่าการทำงานของระบบ" actions={actions}>
    <div className="grid items-start gap-6 lg:grid-cols-[248px_minmax(0,1fr)]">
      <aside className="space-y-4 lg:sticky lg:top-24">
        <nav aria-label="หมวดการตั้งค่า" className="card p-2.5">
          {sections.map((section) => <div key={section.group} className="mb-2 last:mb-0">
            <p className="px-2.5 pb-1 pt-1.5 text-[11px] font-medium tracking-wide text-[var(--subtle)]">{section.group}</p>
            {section.items.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.href;
              return <Link key={item.href} href={item.href} aria-current={active ? "page" : undefined} className={`focus-ring flex h-11 items-center gap-2.5 rounded-md px-3 text-sm ${active ? "bg-[var(--accent-soft)] font-medium text-[#155e97]" : "hover:bg-[var(--canvas)]"}`}><Icon size={18} className={active ? "" : "text-[var(--muted)]"} />{item.label}</Link>;
            })}
          </div>)}
        </nav>
        <p className="px-1.5 text-xs font-light leading-6 text-[var(--muted)]">{note}</p>
      </aside>
      <div className="min-w-0 space-y-6">{children}</div>
    </div>
  </AppShell>;
}

export function SettingsCard({ title, description, action, children, id }: { title: string; description?: string; action?: React.ReactNode; children: React.ReactNode; id?: string }) {
  return <section id={id} className="card overflow-hidden">
    <div className="flex flex-col gap-3 border-b border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
      <div><h2 className="text-lg font-semibold text-[var(--navy)]">{title}</h2>{description && <p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">{description}</p>}</div>
      {action}
    </div>
    {children}
  </section>;
}

export function StatusDot({ tone }: { tone: "good" | "warn" | "bad" | "idle" | "info" }) {
  const colors = { good: "bg-[var(--success)]", warn: "bg-[var(--warning-dot)]", bad: "bg-[var(--highlight)]", idle: "bg-[#c9d1d9]", info: "bg-[var(--accent)]" };
  return <span aria-hidden="true" className={`inline-block h-2 w-2 shrink-0 rounded-full ${colors[tone]}`} />;
}
