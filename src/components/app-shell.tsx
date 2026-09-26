"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { ArrowLeft, Bell, CaretDown, ChartLineUp, FileArrowUp, Files, GearSix, MagnifyingGlass, SignOut, UserCircle } from "@phosphor-icons/react";
import { Avatar, useMe } from "@/components/me-context";
import { can, roleLabel } from "@/lib/permissions";
import { ExpiryNotification, formatThaiDate, getExpiryNotifications } from "@/lib/api";

const navigation = [
  { href: "/", label: "ภาพรวม", shortLabel: "ภาพรวม", icon: ChartLineUp },
  { href: "/upload", label: "นำเข้าเอกสาร", shortLabel: "นำเข้า", icon: FileArrowUp },
  { href: "/documents", label: "เอกสารทั้งหมด", shortLabel: "เอกสาร", icon: Files },
  { href: "/search", label: "ค้นหาข้อมูล", shortLabel: "ค้นหา", icon: MagnifyingGlass },
  { href: "/settings", label: "ตั้งค่า", shortLabel: "ตั้งค่า", icon: GearSix },
];

type AppShellProps = {
  children: React.ReactNode;
  title?: string;
  breadcrumb?: string;
  description?: string;
  actions?: React.ReactNode;
  meta?: React.ReactNode;
  backHref?: string;
  // แถบหัวข้อสูงขึ้นเพื่อให้การ์ดด้านล่างซ้อนขึ้นมาบนแถบได้
  heroBand?: boolean;
};

function isActive(pathname: string, href: string) {
  return href === "/" ? pathname === "/" : pathname.startsWith(href);
}

export function AppShell({ children, title, breadcrumb, description, actions, meta, backHref, heroBand = false }: AppShellProps) {
  const pathname = usePathname();

  return (
    <div className="flex min-h-[100dvh] flex-col bg-[var(--canvas)]">
      <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center gap-4 bg-[var(--bar)] px-4 sm:px-6 lg:h-[72px] lg:gap-6 lg:px-12">
        <Link href="/" aria-label="DocFlow หน้าหลัก" className="focus-ring flex shrink-0 items-center gap-3 rounded-md text-white lg:gap-4">
          <Image src="/brand/college-of-computing.png" alt="College of Computing, Khon Kaen University" width={305} height={89} priority className="h-8 w-auto lg:h-10" />
          <span className="h-6 w-px bg-[#4a545e] lg:h-8" />
          <span className="flex flex-col">
            <span className="text-[15px] font-semibold leading-tight tracking-wide lg:text-[17px]">DocFlow</span>
            <span className="hidden text-[11px] font-light leading-tight text-[var(--bar-muted)] sm:block">ระบบเอกสารคำสั่งแต่งตั้ง</span>
          </span>
        </Link>
        <nav aria-label="เมนูหลัก" className="hidden flex-1 items-center gap-1 lg:flex">
          {navigation.map((item) => {
            const Icon = item.icon;
            const active = isActive(pathname, item.href);
            return <Link key={item.href} href={item.href} aria-current={active ? "page" : undefined} className={`focus-ring flex h-11 items-center gap-2 whitespace-nowrap rounded-[5px] px-4 text-sm transition ${active ? "bg-[var(--bar-hover)] font-medium text-white shadow-[inset_0_-3px_0_var(--accent)]" : "text-[var(--bar-text)] hover:bg-[var(--bar-hover)] hover:text-white"}`}><Icon size={18} weight={active ? "fill" : "regular"} />{item.label}</Link>;
          })}
        </nav>
        <div className="ml-auto flex items-center gap-2 lg:ml-0">
          <NotificationBell />
          <UserMenu />
        </div>
      </header>

      {title && <section className={`shrink-0 bg-[var(--band)] px-5 text-white sm:px-8 lg:px-12 xl:px-16 ${heroBand ? "pb-24 pt-8 lg:pt-10" : "py-6 lg:py-7"}`}>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="flex min-w-0 items-start gap-4">
            {backHref && <Link href={backHref} aria-label="ย้อนกลับ" className="focus-ring mt-1 flex h-11 w-11 shrink-0 items-center justify-center rounded-md border border-white/45 hover:bg-white/10"><ArrowLeft size={18} weight="bold" /></Link>}
            <div className="min-w-0">
              {breadcrumb && <p className="text-[13px] text-[var(--band-text)]">{breadcrumb}</p>}
              <h1 className={`mt-1 font-semibold leading-snug ${heroBand ? "text-[28px] lg:text-[36px]" : "text-[22px] lg:text-[28px]"}`}>{title}</h1>
              {description && <p className="mt-1.5 max-w-2xl text-sm font-light leading-6 text-[#e6f1fa] lg:text-[15px]">{description}</p>}
            </div>
          </div>
          {(actions || meta) && <div className="flex shrink-0 flex-wrap items-center gap-2.5">{meta}{actions}</div>}
        </div>
      </section>}

      <main className="flex-1 px-5 pb-28 pt-6 sm:px-8 lg:px-12 lg:pb-12 lg:pt-8 xl:px-16">{children}</main>

      <footer className="shrink-0 border-t-[10px] border-[var(--accent)] bg-[var(--bar)] pb-[72px] text-xs font-light text-[var(--bar-muted)] lg:pb-0">
        <div className="flex min-h-11 flex-col justify-center gap-0.5 px-5 py-2.5 sm:flex-row sm:items-center sm:justify-between sm:px-8 lg:px-12 xl:px-16"><span><span className="font-medium text-white">DocFlow</span> · วิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น</span><span>123 ถ.มิตรภาพ ต.ในเมือง อ.เมือง จ.ขอนแก่น 40002</span></div>
      </footer>

      <nav aria-label="เมนูหลัก" className="fixed inset-x-0 bottom-0 z-30 grid h-[72px] grid-cols-5 bg-[var(--bar)] px-1 lg:hidden">
        {navigation.map((item) => {
          const Icon = item.icon;
          const active = isActive(pathname, item.href);
          return <Link key={item.href} href={item.href} aria-current={active ? "page" : undefined} className={`focus-ring flex flex-col items-center justify-center gap-1 text-[11px] ${active ? "font-medium text-white shadow-[inset_0_3px_0_var(--accent)]" : "text-[var(--bar-muted)]"}`}><Icon size={22} weight={active ? "fill" : "regular"} />{item.shortLabel}</Link>;
        })}
      </nav>
    </div>
  );
}

// oauth2-proxy (ที่ส่ง X-Auth-Request-*) ใช้ /oauth2/sign_out เป็นค่าเริ่มต้น
const logoutUrl = process.env.NEXT_PUBLIC_LOGOUT_URL ?? "/oauth2/sign_out";

function UserMenu() {
  const { me } = useMe();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => { if (!containerRef.current?.contains(event.target as Node)) setOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", escape);
    return () => { window.removeEventListener("mousedown", close); window.removeEventListener("keydown", escape); };
  }, [open]);

  const role = me?.role ?? "";
  return <div ref={containerRef} className="relative">
    <button type="button" aria-haspopup="menu" aria-expanded={open} aria-label="เมนูโปรไฟล์" onClick={() => setOpen((value) => !value)} className={`focus-ring flex h-11 items-center gap-2.5 rounded-[5px] px-1.5 text-left text-white ${open ? "bg-[var(--bar-hover)]" : "hover:bg-[var(--bar-hover)]"}`}>
      <Avatar me={me} size={34} />
      <span className="hidden xl:block"><span className="block max-w-[160px] truncate text-[13px] font-medium leading-tight">{me?.displayName ?? "กำลังโหลด..."}</span><span className="block text-[11px] font-light leading-tight text-[var(--bar-muted)]">{role ? roleLabel(role) : ""}</span></span>
      <CaretDown size={14} className="hidden text-[var(--bar-muted)] xl:block" />
    </button>
    {open && <div role="menu" aria-label="เมนูโปรไฟล์" className="absolute right-0 top-[52px] w-[300px] overflow-hidden rounded-[10px] bg-white text-[var(--ink)] shadow-[0_12px_32px_rgba(0,0,0,0.18)]">
      <div className="flex items-center gap-3 border-b border-[var(--line)] px-4 py-4">
        <Avatar me={me} size={44} />
        <div className="min-w-0"><p className="truncate font-semibold text-[var(--navy)]">{me?.displayName ?? "-"}</p><p className="truncate text-xs font-light text-[var(--muted)]">{me?.email || me?.userId || ""}</p>{role && <span className="mt-1 inline-flex rounded-full bg-[var(--accent-soft)] px-2 py-0.5 text-[11px] font-medium text-[#155e97]">{role}</span>}</div>
      </div>
      <div className="p-1.5">
        <Link role="menuitem" href="/profile" onClick={() => setOpen(false)} className="focus-ring flex h-11 items-center gap-3 rounded-md px-3 text-sm hover:bg-[var(--canvas)]"><UserCircle size={18} className="text-[var(--muted)]" />โปรไฟล์ของฉัน</Link>
        {can(role, "view") && role.toUpperCase() !== "VIEWER" && <Link role="menuitem" href="/settings" onClick={() => setOpen(false)} className="focus-ring flex h-11 items-center gap-3 rounded-md px-3 text-sm hover:bg-[var(--canvas)]"><GearSix size={18} className="text-[var(--muted)]" />ตั้งค่าระบบ</Link>}
      </div>
      <div className="border-t border-[var(--line)] p-1.5">
        <a role="menuitem" href={logoutUrl} className="focus-ring flex h-11 items-center gap-3 rounded-md px-3 text-sm hover:bg-[var(--canvas)]"><SignOut size={18} className="text-[var(--muted)]" />ออกจากระบบ</a>
      </div>
    </div>}
  </div>;
}

const noticeGroups = [
  { type: "1_MONTH", label: "เหลือ 1 เดือน", dot: "bg-[var(--highlight)]", text: "text-[var(--danger)]" },
  { type: "2_MONTHS", label: "เหลือ 2 เดือน", dot: "bg-[var(--warning-dot)]", text: "text-[var(--warning)]" },
  { type: "3_MONTHS", label: "เหลือ 3 เดือน", dot: "bg-[var(--brand-blue-light)]", text: "text-[var(--muted)]" },
];

function NotificationBell() {
  const [open, setOpen] = useState(false);
  const [notifications, setNotifications] = useState<ExpiryNotification[]>([]);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    getExpiryNotifications().then((response) => setNotifications(response.data)).catch(() => setNotifications([]));
  }, []);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => { if (!containerRef.current?.contains(event.target as Node)) setOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", escape);
    return () => { window.removeEventListener("mousedown", close); window.removeEventListener("keydown", escape); };
  }, [open]);

  // backend สร้างแจ้งเตือนทั้งระดับเอกสารและรายบุคคล กระดิ่งแสดงแค่ระดับเอกสาร
  const seen = new Set<string>();
  const documentNotices = notifications.filter((notice) => {
    const key = `${notice.documentId}-${notice.noticeType}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });

  return <div ref={containerRef} className="relative">
    <button type="button" aria-label={`การแจ้งเตือน ${documentNotices.length} รายการ`} aria-expanded={open} onClick={() => setOpen((value) => !value)} className={`focus-ring relative flex h-11 w-11 items-center justify-center rounded-[5px] transition ${open ? "bg-[var(--bar-hover)] text-white" : "text-[var(--bar-text)] hover:bg-[var(--bar-hover)] hover:text-white"}`}>
      <Bell size={20} />
      {documentNotices.length > 0 && <span className="absolute right-1.5 top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-[var(--highlight)] px-1 text-[10px] font-semibold text-white">{documentNotices.length > 9 ? "9+" : documentNotices.length}</span>}
    </button>
    {open && <div role="dialog" aria-label="การแจ้งเตือน" className="absolute right-0 top-[52px] w-[min(92vw,400px)] overflow-hidden rounded-[10px] bg-white text-[var(--ink)] shadow-[0_12px_32px_rgba(0,0,0,0.18)]">
      <div className="border-b border-[var(--line)] px-5 py-4"><p className="font-semibold text-[var(--navy)]">การแจ้งเตือน</p><p className="mt-0.5 text-xs text-[var(--muted)]">เอกสารที่ใกล้หมดวาระ</p></div>
      <div className="shell-scrollbar max-h-[420px] overflow-y-auto">
        {noticeGroups.map((group) => {
          const items = documentNotices.filter((notice) => notice.noticeType === group.type);
          if (!items.length) return null;
          return <div key={group.type} className="border-b border-[var(--line-soft)] last:border-0">
            <p className={`px-5 pb-1 pt-3 text-[11px] font-medium tracking-wide ${group.text}`}>{group.label}</p>
            {items.map((notice) => <Link key={notice.id} href={`/documents/${notice.documentId}`} onClick={() => setOpen(false)} className="focus-ring flex gap-3 px-5 py-3 hover:bg-[var(--brand-blue-pale)]">
              <span className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full ${group.dot}`} />
              <span className="min-w-0"><span className="block text-sm font-medium">เอกสาร #{notice.documentId}</span><span className="block text-xs text-[var(--muted)]">ครบกำหนด {formatThaiDate(notice.dueAt)}</span></span>
            </Link>)}
          </div>;
        })}
        {documentNotices.length === 0 && <p className="px-5 py-10 text-center text-sm text-[var(--muted)]">ยังไม่มีเอกสารใกล้หมดวาระ</p>}
      </div>
    </div>}
  </div>;
}

export function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    "รอตรวจสอบ": "bg-[var(--warning-soft)] text-[var(--warning)]",
    "กำลังประมวลผล": "bg-[var(--accent-soft)] text-[#155e97]",
    "ยืนยันแล้ว": "bg-[var(--success-soft)] text-[var(--success)]",
    "ต้องตรวจสอบ": "bg-[var(--danger-soft)] text-[var(--danger)]",
    "ต้องแก้ไข": "bg-[var(--danger-soft)] text-[var(--danger)]",
    "ประมวลผลไม่สำเร็จ": "bg-[var(--danger-soft)] text-[var(--danger)]",
  };
  return <span className={`inline-flex whitespace-nowrap rounded-full px-3 py-1 text-xs font-medium ${styles[status] ?? "bg-[var(--line-soft)] text-[var(--muted)]"}`}>{status}</span>;
}

export function PanelHeader({ title, description, action }: { title: string; description?: string; action?: React.ReactNode }) {
  return <div className="flex flex-col gap-3 border-b border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
    <div><h2 className="text-base font-semibold text-[var(--navy)] lg:text-lg">{title}</h2>{description && <p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">{description}</p>}</div>
    {action}
  </div>;
}
