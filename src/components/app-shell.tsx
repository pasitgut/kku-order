"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Bell,
  CaretDown,
  ChartLineUp,
  FileArrowUp,
  Files,
  GearSix,
  MagnifyingGlass,
  SealCheck,
  UserCircle,
} from "@phosphor-icons/react";

const navigation = [
  { href: "/", label: "ภาพรวม", icon: ChartLineUp },
  { href: "/upload", label: "นำเข้าเอกสาร", icon: FileArrowUp },
  { href: "/documents", label: "เอกสารทั้งหมด", icon: Files },
  { href: "/search", label: "ค้นหาข้อมูล", icon: MagnifyingGlass },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="flex min-h-[100dvh] bg-[var(--canvas)]">
      <aside className="hidden w-[250px] shrink-0 flex-col bg-[var(--navy)] text-white lg:flex">
        <div className="flex h-[84px] items-center border-b border-white/10 px-7">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--accent)] text-white"><SealCheck size={23} weight="fill" /></div>
            <div><p className="text-[15px] font-bold tracking-tight">DOCFLOW</p><p className="mt-0.5 text-[10px] font-medium tracking-[0.16em] text-[#aac1d3]">COMPUTER COLLEGE</p></div>
          </div>
        </div>
        <div className="flex-1 px-4 py-7">
          <p className="px-3 text-[10px] font-bold uppercase tracking-[0.18em] text-[#7390a9]">เมนูหลัก</p>
          <nav className="mt-3 space-y-1.5" aria-label="เมนูหลัก">
            {navigation.map((item) => {
              const Icon = item.icon;
              const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
              return <Link key={item.href} href={item.href} className={`focus-ring flex items-center gap-3 rounded-lg px-3 py-3 text-sm font-medium transition ${active ? "bg-white/12 text-white" : "text-[#a6bacb] hover:bg-white/7 hover:text-white"}`}><Icon size={20} weight={active ? "fill" : "regular"} /><span>{item.label}</span>{active && <span className="ml-auto h-1.5 w-1.5 rounded-full bg-[var(--brand-blue-light)]" />}</Link>;
            })}
          </nav>
          <p className="mt-9 px-3 text-[10px] font-bold uppercase tracking-[0.18em] text-[#7390a9]">ระบบ</p>
          <nav className="mt-3 space-y-1.5" aria-label="เมนูระบบ">
            <Link href="/settings" className={`focus-ring flex items-center gap-3 rounded-lg px-3 py-3 text-sm font-medium transition ${pathname.startsWith("/settings") ? "bg-white/12 text-white" : "text-[#a6bacb] hover:bg-white/7 hover:text-white"}`}><GearSix size={20} weight={pathname.startsWith("/settings") ? "fill" : "regular"} /><span>ผู้ใช้งานและสิทธิ์</span></Link>
          </nav>
        </div>
        <div className="border-t border-white/10 p-4"><div className="rounded-lg bg-white/7 p-3"><div className="flex items-center gap-2.5"><div className="flex h-8 w-8 items-center justify-center rounded-full bg-[var(--brand-blue-pale)] text-xs font-bold text-[var(--navy)]">กว</div><div className="min-w-0"><p className="truncate text-xs font-semibold">กฤตยชญ์ มัตกิจ</p><p className="mt-0.5 truncate text-[10px] text-[#9eb5c8]">เจ้าหน้าที่ตรวจสอบ</p></div><CaretDown size={14} className="ml-auto text-[#9eb5c8]" /></div></div></div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-[84px] items-center justify-between border-b border-[var(--line)] bg-white px-5 sm:px-8">
          <div className="flex items-center gap-3 lg:hidden"><div className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--navy)] text-[var(--brand-blue-light)]"><SealCheck size={22} weight="fill" /></div><p className="text-sm font-bold tracking-tight text-[var(--navy)]">DOCFLOW</p></div>
          <p className="hidden text-sm text-[var(--muted)] lg:block">วิทยาลัยการคอมพิวเตอร์ · มหาวิทยาลัยขอนแก่น</p>
          <div className="ml-auto flex items-center gap-3"><button type="button" aria-label="การแจ้งเตือน" className="focus-ring relative flex h-10 w-10 items-center justify-center rounded-lg text-[var(--muted)] transition hover:bg-[#f1f5f8] hover:text-[var(--navy)]"><Bell size={20} /><span className="absolute right-2.5 top-2 h-1.5 w-1.5 rounded-full bg-[#d26a4f]" /></button><div className="hidden h-7 w-px bg-[var(--line)] sm:block" /><button type="button" className="focus-ring flex items-center gap-2 rounded-lg px-1.5 py-1.5 text-left hover:bg-[#f6f8fa]"><UserCircle size={30} weight="duotone" className="text-[var(--accent)]" /><span className="hidden sm:block"><span className="block text-xs font-bold text-[var(--ink)]">กฤตยชญ์ มัตกิจ</span><span className="mt-0.5 block text-[10px] text-[var(--muted)]">ผู้ดูแลระบบเอกสาร</span></span><CaretDown size={14} className="hidden text-[var(--muted)] sm:block" /></button></div>
        </header>
        <main className="shell-scrollbar flex-1 overflow-y-auto px-5 py-8 sm:px-8 lg:px-10">{children}</main>
      </div>
    </div>
  );
}

export function SectionTitle({ eyebrow, title, description }: { eyebrow: string; title: string; description?: string }) {
  return <div><p className="mb-2 text-[11px] font-bold uppercase tracking-[0.16em] text-[var(--accent)]">{eyebrow}</p><h1 className="text-2xl font-bold tracking-tight text-[var(--ink)] sm:text-[30px]">{title}</h1>{description && <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--muted)]">{description}</p>}</div>;
}

export function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = { "รอตรวจสอบ": "bg-[var(--warning-soft)] text-[var(--warning)]", "กำลังประมวลผล": "bg-[var(--accent-soft)] text-[var(--accent)]", "ยืนยันแล้ว": "bg-[var(--success-soft)] text-[var(--success)]", "ต้องแก้ไข": "bg-[var(--danger-soft)] text-[var(--danger)]" };
  return <span className={`inline-flex whitespace-nowrap rounded-full px-2.5 py-1 text-[11px] font-bold ${styles[status] ?? "bg-slate-100 text-slate-600"}`}>{status}</span>;
}
