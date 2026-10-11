"use client";

import Link from "next/link";
import { Fragment, useEffect, useState } from "react";
import { ArrowUpRight, CaretLeft, CaretRight, FilePdf, MagnifyingGlass, Plus, WarningCircle } from "@phosphor-icons/react";
import { DocumentCancellationForm } from "@/components/document-cancellation-form";
import { useMe } from "@/components/me-context";
import { can } from "@/lib/permissions";
import { AppShell, StatusBadge } from "@/components/app-shell";
import { ApiDocument, formatFileSize, formatThaiDate, getDocuments, statusLabel } from "@/lib/api";
import { confidenceLabel, processingNote } from "@/lib/processing-status";

const filters = [
  { label: "ทั้งหมด", value: "" },
  { label: "รอตรวจสอบ", value: "REVIEW" },
  { label: "กำลังประมวลผล", value: "PROCESSING" },
  { label: "ยืนยันแล้ว", value: "CONFIRMED" },
  { label: "ยกเลิกแล้ว", value: "CANCELLED" },
  { label: "ต้องตรวจสอบ", value: "NEEDS_REVIEW" },
];

export default function DocumentsPage() {
  const { me } = useMe();
  const canCancel = can(me?.role ?? "", "cancel");
  const [cancelId, setCancelId] = useState<number | null>(null);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("");
  const [documents, setDocuments] = useState<ApiDocument[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    const timer = window.setTimeout(() => {
      setLoading(true);
      getDocuments({ q: query, status: filter, page, limit: 25 })
        .then((response) => { if (!active) return; setDocuments(response.data); setTotal(response.pagination?.total ?? response.data.length); setError(""); })
        .catch((reason: Error) => { if (active) setError(reason.message); })
        .finally(() => { if (active) setLoading(false); });
    }, 250);
    return () => { active = false; window.clearTimeout(timer); };
  }, [query, filter, page, refreshVersion]);

  const onDocumentCancelled = (updated: ApiDocument) => {
    setCancelId((current) => current === updated.id ? null : current);
    setDocuments((current) => current.flatMap((item) => item.id !== updated.id ? [item] : filter && filter !== updated.status ? [] : [updated]));
    if (filter && filter !== updated.status && documents.length === 1 && page > 1) setPage((current) => current - 1);
    setLoading(true);
    setRefreshVersion((current) => current + 1);
  };

  const selectFilter = (value: string) => { setFilter(value); setPage(1); setCancelId(null); };

  return <AppShell breadcrumb="หน้าหลัก / เอกสารทั้งหมด" title="เอกสารทั้งหมด" description="จัดการเอกสารคำสั่งแต่งตั้งในฐานข้อมูล ติดตามสถานะ และเปิดตรวจสอบผลการสกัดข้อมูล" actions={<Link href="/upload" className="focus-ring inline-flex h-12 items-center gap-2 rounded-md bg-white px-6 text-[15px] font-medium text-[var(--accent)] transition hover:bg-[var(--accent-soft)]"><Plus size={18} weight="bold" /> นำเข้าเอกสาร</Link>}><div className="space-y-5">
    <section className="card flex flex-col gap-4 p-4 lg:flex-row lg:items-center lg:px-5"><div className="relative w-full lg:max-w-[440px]"><MagnifyingGlass size={18} className="absolute left-4 top-1/2 -translate-y-1/2 text-[var(--muted)]" /><input value={query} onChange={(event) => { setQuery(event.target.value); setPage(1); setCancelId(null); }} placeholder="ค้นหาชื่อเอกสาร เลขที่คำสั่ง หรือบุคลากร..." className="focus-ring h-11 w-full rounded-full border-0 bg-[var(--pill)] pl-11 pr-4 text-sm text-[var(--navy)] outline-none placeholder:text-[var(--subtle)]" /></div><div role="group" aria-label="กรองตามสถานะ" className="flex gap-2 overflow-x-auto">{filters.map((item) => <button type="button" key={item.label} aria-pressed={filter === item.value} onClick={() => selectFilter(item.value)} className={`focus-ring h-10 whitespace-nowrap rounded-full border px-4 text-[13px] transition ${filter === item.value ? "border-[var(--accent)] bg-[var(--accent)] font-medium text-white" : "border-[var(--line-strong)] bg-white text-[var(--ink)] hover:border-[var(--accent)] hover:text-[var(--accent)]"}`}>{item.label}</button>)}</div></section>
    <section className="card overflow-hidden">
      {error && <div className="m-5 rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div>}
      <div className="overflow-x-auto"><table className="data-grid w-full min-w-[970px] text-left text-sm"><thead><tr><th className="px-5 py-3">เอกสาร</th><th className="px-4 py-3">เลขที่คำสั่ง</th><th className="px-4 py-3">วันที่ออกคำสั่ง</th><th className="px-4 py-3">นำเข้าโดย</th><th className="px-4 py-3">ความมั่นใจ</th><th className="px-5 py-3">สถานะ</th><th className="px-3 py-3">จัดการ</th></tr></thead><tbody>{loading ? Array.from({ length: 5 }, (_, index) => <tr key={index}><td colSpan={7} className="px-5 py-5"><div className="skeleton h-5 w-3/4" /></td></tr>) : documents.map((document) => <Fragment key={document.id}><tr className="group transition hover:bg-[var(--brand-blue-pale)]"><td className="px-5 py-4"><Link href={`/documents/${document.id}`} className="focus-ring flex items-center gap-3 rounded-md"><span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[var(--accent)]"><FilePdf size={19} weight="duotone" /></span><span className="min-w-0"><span className="block max-w-[310px] truncate font-medium text-[var(--ink)]">{document.title || document.originalFilename}</span><span className="mt-1 block text-xs text-[var(--muted)]">{document.pageCount || "-"} หน้า · {formatFileSize(document.fileSizeBytes)}</span></span></Link></td><td className="px-4 py-4 font-medium text-[var(--ink)]">{document.orderNo || "ยังไม่พบ"}</td><td className="px-4 py-4 text-[var(--muted)]">{formatThaiDate(document.issuedDate)}</td><td className="px-4 py-4 text-[var(--muted)]">{document.importedBy || "ไม่ระบุ"}</td><td className="px-4 py-4"><span className={`font-semibold ${document.confidence >= 0.9 ? "text-[var(--success)]" : "text-[var(--warning)]"}`}>{confidenceLabel(document)}</span>{processingNote(document) && <span className="mt-1 block text-xs font-normal text-[var(--muted)]">{processingNote(document)}</span>}</td><td className="px-5 py-4"><StatusBadge status={statusLabel(document.status)} />{document.status === "CANCELLED" && <p title={document.cancellationReason} className="mt-1 max-w-[200px] truncate text-xs text-[var(--muted)]">{document.cancellationReason}</p>}</td><td className="px-3 py-4"><div className="flex items-center justify-end gap-2 whitespace-nowrap"><Link href={`/documents/${document.id}`} aria-label={`เปิด ${document.title || document.originalFilename}`} className="focus-ring inline-flex h-10 items-center gap-1 rounded-md px-2 text-xs text-[var(--accent)] hover:bg-[var(--accent-soft)]"><ArrowUpRight size={17} />เปิด</Link>{canCancel && document.status !== "CANCELLED" && <button id={`cancel-document-${document.id}-button`} type="button" aria-expanded={cancelId === document.id} aria-controls={cancelId === document.id ? `cancel-document-${document.id}` : undefined} aria-label={`ยกเลิก ${document.title || document.originalFilename}`} onClick={() => setCancelId((current) => current === document.id ? null : document.id)} className="focus-ring inline-flex h-10 items-center gap-1 rounded-md border border-[var(--danger)]/25 px-3 text-xs font-medium text-[var(--danger)] hover:bg-[var(--danger-soft)]"><WarningCircle size={15} />ยกเลิก</button>}</div></td></tr>{canCancel && cancelId === document.id && document.status !== "CANCELLED" && <tr><td colSpan={7} className="px-5 pb-5 pt-1"><DocumentCancellationForm document={document} onClose={() => setCancelId(null)} onCancelled={onDocumentCancelled} /></td></tr>}</Fragment>)}{!loading && documents.length === 0 && <tr><td colSpan={7} className="px-5 py-16 text-center"><MagnifyingGlass size={28} className="mx-auto text-[#a9b6c2]" /><p className="mt-3 text-sm font-semibold text-[var(--ink)]">ยังไม่มีเอกสารที่ตรงกับการค้นหา</p><p className="mt-1 text-xs text-[var(--muted)]">นำเข้า PDF ตัวอย่างหรือเปลี่ยนคำค้นหา</p></td></tr>}</tbody></table></div><div className="flex flex-col justify-between gap-3 border-t border-[var(--line)] px-5 py-4 text-xs text-[var(--muted)] sm:flex-row sm:items-center"><span>แสดง {documents.length} จาก {total.toLocaleString("th-TH")} รายการ</span><div className="flex items-center gap-2"><button type="button" disabled={page === 1} onClick={() => { setCancelId(null); setPage((current) => current - 1); }} className="focus-ring rounded-md border border-[var(--line)] px-3 py-2 font-semibold disabled:cursor-not-allowed disabled:opacity-40 hover:border-[var(--accent)]"><CaretLeft size={15} /></button><span className="rounded-md bg-[var(--accent)] px-3 py-2 font-medium text-white">{page}</span><button type="button" disabled={documents.length < 25} onClick={() => { setCancelId(null); setPage((current) => current + 1); }} className="focus-ring rounded-md border border-[var(--line)] px-3 py-2 font-semibold disabled:cursor-not-allowed disabled:opacity-40 hover:border-[var(--accent)]"><CaretRight size={15} /></button></div></div></section>
  </div></AppShell>;
}
