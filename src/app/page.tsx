"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowUpRight, CheckCircle, Clock, FileArrowUp, FilePdf, MagnifyingGlass, WarningCircle } from "@phosphor-icons/react";
import { AppShell, PanelHeader, StatusBadge } from "@/components/app-shell";
import { ApiDocument, Dashboard, ExpiryNotification, formatFileSize, formatThaiDate, getDashboard, getDocuments, getExpiryNotifications, statusLabel } from "@/lib/api";

const emptyDashboard: Dashboard = { total: 0, processing: 0, review: 0, confirmed: 0, needsReview: 0, expiring: 0 };

const noticeLabels: Record<string, string> = { "1_MONTH": "1 เดือน", "2_MONTHS": "2 เดือน", "3_MONTHS": "3 เดือน" };

export default function HomePage() {
  const [dashboard, setDashboard] = useState(emptyDashboard);
  const [reviewDocuments, setReviewDocuments] = useState<ApiDocument[]>([]);
  const [notifications, setNotifications] = useState<ExpiryNotification[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([getDashboard(), getDocuments({ status: "REVIEW", limit: 5 }), getExpiryNotifications()]).then(([summary, documents, expiry]) => { setDashboard(summary.data); setReviewDocuments(documents.data); setNotifications(expiry.data); }).catch((reason: Error) => setError(reason.message));
  }, []);

  const stats = [
    ["เอกสารทั้งหมด", dashboard.total, "คลังเอกสารใน MySQL", FilePdf, "text-[var(--accent)]", "bg-[var(--accent-soft)]"],
    ["รอตรวจสอบ", dashboard.review + dashboard.needsReview, "ต้องดำเนินการ", Clock, "text-[var(--warning)]", "bg-[var(--warning-soft)]"],
    ["ยืนยันแล้ว", dashboard.confirmed, "ผ่านการตรวจสอบ", CheckCircle, "text-[var(--success)]", "bg-[var(--success-soft)]"],
    ["ใกล้หมดวาระ", dashboard.expiring, "ภายใน 3 เดือน", WarningCircle, "text-[var(--danger)]", "bg-[var(--danger-soft)]"],
  ] as const;

  // แจ้งเตือนระดับเอกสารพอ ไม่ต้องซ้ำทุกคนในคำสั่ง
  const documentNotices = notifications.filter((notice, index, all) => all.findIndex((other) => other.documentId === notice.documentId && other.noticeType === notice.noticeType) === index).slice(0, 5);

  return <AppShell heroBand breadcrumb="หน้าหลัก / ภาพรวมระบบ" title="ศูนย์กลางเอกสารคำสั่งแต่งตั้ง" description="ติดตามคิวสกัดข้อมูล ตรวจสอบผลลัพธ์ และค้นหาประวัติการแต่งตั้งจากฐานข้อมูลจริง" actions={<Link href="/upload" className="focus-ring inline-flex h-12 items-center gap-2 rounded-md bg-white px-6 text-[15px] font-medium text-[var(--accent)] transition hover:bg-[var(--accent-soft)]"><FileArrowUp size={18} weight="bold" />นำเข้าเอกสาร</Link>}>
    <div className="-mt-[88px] space-y-6 lg:-mt-24">
      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4 xl:gap-6">{stats.map(([label, value, hint, Icon, iconTone, background]) => <div key={label} className="rounded-[10px] border border-transparent bg-white p-5 shadow-[0_1.6px_8px_rgba(0,0,0,0.2)] transition hover:border-dashed hover:border-[var(--band-strip)] lg:p-6"><div className="flex items-center justify-between"><p className="text-sm font-medium text-[var(--muted)]">{label}</p><span className={`flex h-10 w-10 items-center justify-center rounded-lg ${background} ${iconTone}`}><Icon size={20} weight="duotone" /></span></div><p className="mt-3 text-[34px] font-semibold leading-none text-[var(--navy)]">{value.toLocaleString("th-TH")}</p><p className="mt-3 text-xs text-[var(--muted)]">{hint}</p></div>)}</section>

      {error && <div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div>}

      <div className="grid items-start gap-6 xl:grid-cols-3">
        <section className="card overflow-hidden xl:col-span-2">
          <PanelHeader title="คิวตรวจสอบล่าสุด" description="เอกสารจาก one-ocr ที่รอเจ้าหน้าที่ตรวจสอบและยืนยัน" action={<Link href="/documents" className="focus-ring inline-flex h-9 items-center gap-1.5 rounded-md border border-[var(--accent)] px-3.5 text-[13px] font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]">ดูทั้งหมด <ArrowUpRight size={14} weight="bold" /></Link>} />
          <div className="overflow-x-auto"><table className="data-grid w-full min-w-[640px] text-left text-sm"><thead><tr><th className="px-6 py-3">เอกสาร</th><th className="px-4 py-3">เลขที่คำสั่ง</th><th className="px-4 py-3">นำเข้าเมื่อ</th><th className="px-6 py-3">สถานะ</th></tr></thead><tbody>{reviewDocuments.map((document) => <tr key={document.id} className="transition hover:bg-[var(--brand-blue-pale)]"><td className="px-6 py-3.5"><Link href={`/documents/${document.id}`} className="focus-ring block rounded-md"><p className="font-medium text-[var(--ink)]">{document.title || document.originalFilename}</p><p className="mt-0.5 text-xs font-light text-[var(--muted)]">{document.pageCount || "-"} หน้า · {formatFileSize(document.fileSizeBytes)}</p></Link></td><td className="px-4 py-3.5 text-[var(--muted)]">{document.orderNo || "ยังไม่พบ"}</td><td className="px-4 py-3.5 text-[var(--muted)]">{formatThaiDate(document.createdAt)}</td><td className="px-6 py-3.5"><StatusBadge status={statusLabel(document.status)} /></td></tr>)}{reviewDocuments.length === 0 && <tr><td colSpan={4} className="px-6 py-12 text-center text-sm text-[var(--muted)]">ยังไม่มีเอกสารรอตรวจสอบ</td></tr>}</tbody></table></div>
        </section>

        <div className="grid gap-6 sm:grid-cols-2 xl:grid-cols-1">
          <section className="card overflow-hidden sm:col-span-2 xl:col-span-1">
            <div className="flex items-center gap-3 border-b border-[var(--line)] px-5 py-4"><span className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--danger-soft)] text-[var(--danger)]"><WarningCircle size={18} weight="duotone" /></span><div><h2 className="font-semibold text-[var(--navy)]">ใกล้หมดวาระ</h2><p className="text-xs font-light text-[var(--muted)]">แจ้งเตือนตามเกณฑ์ 3, 2 และ 1 เดือน</p></div></div>
            {documentNotices.map((notice) => <Link key={notice.id} href={`/documents/${notice.documentId}`} className="focus-ring flex items-center justify-between gap-3 border-b border-[var(--line-soft)] px-5 py-3.5 text-sm last:border-0 hover:bg-[var(--brand-blue-pale)]"><span className="font-medium">เอกสาร #{notice.documentId}</span><span className="text-xs text-[var(--muted)]">{formatThaiDate(notice.dueAt)} · {noticeLabels[notice.noticeType] ?? notice.noticeType}</span></Link>)}
            {documentNotices.length === 0 && <p className="px-5 py-8 text-center text-sm text-[var(--muted)]">ยังไม่มีเอกสารใกล้หมดวาระ</p>}
          </section>
          <Link href="/upload" className="focus-ring group flex flex-col gap-2 rounded-[10px] bg-[var(--navy)] p-6 text-white transition hover:bg-[var(--navy-soft)]"><span className="mb-2 flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--accent)]"><FileArrowUp size={20} /></span><span className="text-xs text-[var(--brand-blue-light)]">งานที่ต้องทำ</span><span className="text-lg font-semibold">นำเข้าเอกสารชุดใหม่</span><span className="text-[13px] font-light leading-6 text-[var(--brand-blue-text)]">ไฟล์จะเข้า MySQL และคิว one-ocr อัตโนมัติ</span></Link>
          <Link href="/search" className="focus-ring card flex flex-col gap-2 border border-transparent p-6 transition hover:border-dashed hover:border-[var(--band-strip)]"><span className="mb-2 flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[var(--accent)]"><MagnifyingGlass size={20} /></span><span className="text-xs text-[var(--muted)]">การค้นหา</span><span className="text-lg font-semibold text-[var(--navy)]">ค้นหาประวัติการแต่งตั้ง</span><span className="text-[13px] font-light leading-6 text-[var(--muted)]">ค้นหาชื่อบุคคล ตำแหน่ง เลขที่คำสั่ง หรือข้อความ OCR</span></Link>
        </div>
      </div>
    </div>
  </AppShell>;
}
