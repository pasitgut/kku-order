"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { CaretLeft, CaretRight, DownloadSimple, MagnifyingGlass } from "@phosphor-icons/react";
import { useMe } from "@/components/me-context";
import { SettingsCard, SettingsFrame } from "@/components/settings-frame";
import { AuditLogItem, getApiUrl, getAuditActions, getAuditLogs } from "@/lib/api";
import { AUDIT_PAGE_SIZE, AuditFilters, auditDateRangeError, auditDocumentHref, auditExportPath, auditQueryString, emptyAuditFilters, formatAuditTime, truncateDetails } from "@/lib/audit-log";
import { can } from "@/lib/permissions";

const inputClass = "focus-ring h-10 w-full rounded-md border border-[var(--line-strong)] bg-white px-3 text-[13px]";

function DetailsCell({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  if (!text) return <span className="text-[var(--subtle)]">-</span>;
  const { preview, truncated } = truncateDetails(text);
  if (!truncated) return <span className="whitespace-pre-line break-words">{text}</span>;
  return <span className="whitespace-pre-line break-words">{open ? text : `${preview}…`}
    <button type="button" aria-expanded={open} onClick={() => setOpen((value) => !value)} className="focus-ring ml-1 whitespace-nowrap text-[var(--accent)] hover:underline">{open ? "ย่อ" : "ดูเพิ่ม"}</button>
  </span>;
}

function DocumentCell({ item }: { item: AuditLogItem }) {
  const href = auditDocumentHref(item);
  const title = item.documentTitle || (item.documentId ? `เอกสาร #${item.documentId}` : "-");
  if (href) return <Link href={href} className="focus-ring text-[var(--accent)] underline-offset-2 hover:underline">{title}</Link>;
  return <span className="text-[var(--muted)]">{item.documentTitle || "-"}</span>;
}

export default function AuditLogPage() {
  const { me, status } = useMe();
  const allowed = can(me?.role ?? "", "audit");
  const [draft, setDraft] = useState<AuditFilters>(emptyAuditFilters);
  const [applied, setApplied] = useState<AuditFilters>(emptyAuditFilters);
  const [page, setPage] = useState(1);
  const [rows, setRows] = useState<AuditLogItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionOptions, setActionOptions] = useState<{ action: string; label: string }[]>([]);

  useEffect(() => {
    if (!allowed) return;
    getAuditActions().then((response) => setActionOptions(response.data)).catch(() => setActionOptions([]));
  }, [allowed]);

  useEffect(() => {
    if (!allowed) return;
    let active = true;
    getAuditLogs(auditQueryString(applied, page, AUDIT_PAGE_SIZE))
      .then((response) => { if (active) { setRows(response.data); setTotal(response.total); setError(""); } })
      .catch((reason: Error) => { if (active) setError(reason.message); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [allowed, applied, page]);

  const dateError = auditDateRangeError(draft.from, draft.to);
  const pageCount = Math.max(1, Math.ceil(total / AUDIT_PAGE_SIZE));

  const search = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (dateError) return;
    setLoading(true);
    setApplied(draft);
    setPage(1);
  };
  const reset = () => {
    setDraft(emptyAuditFilters());
    setLoading(true);
    setApplied(emptyAuditFilters());
    setPage(1);
  };
  const changePage = (next: number) => {
    setLoading(true);
    setPage(next);
  };

  if (status === "loading") {
    return <SettingsFrame note="ประวัติการใช้งานบันทึกเพิ่มได้อย่างเดียว แก้ไขหรือลบไม่ได้"><div className="skeleton h-40 w-full" /></SettingsFrame>;
  }
  if (!allowed) {
    return <SettingsFrame note="ประวัติการใช้งานบันทึกเพิ่มได้อย่างเดียว แก้ไขหรือลบไม่ได้">
      <SettingsCard title="ประวัติการใช้งานระบบ"><p className="px-5 py-8 text-sm text-[var(--muted)] lg:px-6">หน้านี้สำหรับผู้ดูแลระบบและนักพัฒนาเท่านั้น</p></SettingsCard>
    </SettingsFrame>;
  }

  return <SettingsFrame note="ประวัติการใช้งานบันทึกเพิ่มได้อย่างเดียว แก้ไขหรือลบไม่ได้">
    <SettingsCard title="ประวัติการใช้งานระบบ" description="ทุกการเปลี่ยนแปลงข้อมูล การตั้งค่า และงานอัตโนมัติ เรียงจากล่าสุด">
      <form onSubmit={search} className="grid gap-3 px-5 py-4 sm:grid-cols-2 lg:grid-cols-3 lg:px-6">
        <div className="space-y-1.5 sm:col-span-2 lg:col-span-1">
          <label htmlFor="audit-q" className="text-[13px] font-medium">ค้นหาในรายละเอียดหรือชื่อเอกสาร</label>
          <input id="audit-q" type="search" value={draft.q} onChange={(event) => setDraft({ ...draft, q: event.target.value })} placeholder="เช่น ชื่อคำสั่ง" className={inputClass} />
        </div>
        <div className="space-y-1.5">
          <label htmlFor="audit-action" className="text-[13px] font-medium">การกระทำ</label>
          <select id="audit-action" value={draft.action} onChange={(event) => setDraft({ ...draft, action: event.target.value })} className={inputClass}>
            <option value="">ทุกการกระทำ</option>
            {actionOptions.map((option) => <option key={option.action} value={option.action}>{option.label}</option>)}
          </select>
        </div>
        <div className="space-y-1.5">
          <label htmlFor="audit-user" className="text-[13px] font-medium">ผู้ใช้ (รหัสบัญชี)</label>
          <input id="audit-user" type="text" value={draft.userId} onChange={(event) => setDraft({ ...draft, userId: event.target.value })} className={inputClass} />
        </div>
        <div className="space-y-1.5">
          <label htmlFor="audit-from" className="text-[13px] font-medium">ตั้งแต่วันที่</label>
          <input id="audit-from" type="date" value={draft.from} onChange={(event) => setDraft({ ...draft, from: event.target.value })} className={inputClass} />
        </div>
        <div className="space-y-1.5">
          <label htmlFor="audit-to" className="text-[13px] font-medium">ถึงวันที่</label>
          <input id="audit-to" type="date" value={draft.to} onChange={(event) => setDraft({ ...draft, to: event.target.value })} className={inputClass} />
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <button type="submit" disabled={Boolean(dateError)} className="focus-ring inline-flex h-10 items-center gap-2 rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white disabled:opacity-60"><MagnifyingGlass size={16} />ค้นหา</button>
          <button type="button" onClick={reset} className="focus-ring h-10 rounded-md border border-[var(--line-strong)] bg-white px-4 text-sm font-medium text-[var(--accent)] hover:bg-[var(--surface-muted)]">ล้างตัวกรอง</button>
        </div>
        {dateError && <p role="alert" className="text-[13px] font-medium text-[var(--danger)] sm:col-span-2 lg:col-span-3">{dateError}</p>}
      </form>

      <div className="flex flex-col gap-3 border-t border-[var(--line)] px-5 py-3 sm:flex-row sm:items-center sm:justify-between lg:px-6">
        <p className="text-[13px] text-[var(--muted)]">{loading ? "กำลังโหลด…" : `ทั้งหมด ${total.toLocaleString("th-TH")} รายการ`}</p>
        <a href={`${getApiUrl()}${auditExportPath(applied)}`} download className="focus-ring inline-flex h-10 items-center gap-2 self-start rounded-md border border-[var(--line-strong)] bg-white px-3.5 text-sm font-medium text-[var(--accent)] hover:bg-[var(--surface-muted)] sm:self-auto"><DownloadSimple size={16} />ส่งออก CSV (สูงสุด 10,000 รายการ)</a>
      </div>

      {error && <div role="alert" className="mx-5 mb-4 rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)] lg:mx-6">{error}</div>}

      <div className="overflow-x-auto border-t border-[var(--line)]">
        <table className="data-grid w-full min-w-[760px] text-left text-sm">
          <thead><tr><th className="px-4 py-2.5">เวลา</th><th className="px-3 py-2.5">ผู้ใช้</th><th className="px-3 py-2.5">การกระทำ</th><th className="px-3 py-2.5">เอกสาร</th><th className="px-4 py-2.5">รายละเอียด</th></tr></thead>
          <tbody>
            {rows.map((item) => <tr key={item.id} className="border-t border-[var(--line-soft)] align-top">
              <td className="whitespace-nowrap px-4 py-3 text-[13px]">{formatAuditTime(item.createdAt)}</td>
              <td className="px-3 py-3"><p className="font-medium">{item.userName || item.userId}</p>{item.userName && <p className="text-[11px] text-[var(--subtle)]">{item.userId}</p>}</td>
              <td className="px-3 py-3"><span className="inline-flex whitespace-nowrap rounded-full bg-[var(--accent-soft)] px-2.5 py-0.5 text-xs font-medium text-[#155e97]">{item.actionLabel}</span></td>
              <td className="max-w-[220px] px-3 py-3 break-words"><DocumentCell item={item} /></td>
              <td className="max-w-[300px] px-4 py-3 text-[13px]"><DetailsCell text={item.details} /></td>
            </tr>)}
            {!loading && rows.length === 0 && <tr><td colSpan={5} className="px-6 py-10 text-center text-sm text-[var(--muted)]">ไม่พบประวัติการใช้งานตามตัวกรอง</td></tr>}
          </tbody>
        </table>
      </div>

      <nav aria-label="เปลี่ยนหน้า" className="flex items-center justify-between gap-3 border-t border-[var(--line)] px-5 py-3 text-sm lg:px-6">
        <button type="button" disabled={page <= 1 || loading} onClick={() => changePage(page - 1)} className="focus-ring inline-flex h-9 items-center gap-1 rounded-md border border-[var(--line-strong)] bg-white px-3 disabled:opacity-50"><CaretLeft size={14} />ก่อนหน้า</button>
        <span className="text-[13px] text-[var(--muted)]">หน้า {page} จาก {pageCount}</span>
        <button type="button" disabled={page >= pageCount || loading} onClick={() => changePage(page + 1)} className="focus-ring inline-flex h-9 items-center gap-1 rounded-md border border-[var(--line-strong)] bg-white px-3 disabled:opacity-50">ถัดไป<CaretRight size={14} /></button>
      </nav>
    </SettingsCard>
  </SettingsFrame>;
}
