"use client";

import { useEffect, useId, useRef, useState } from "react";
import { CaretDown, DownloadSimple, SpinnerGap } from "@phosphor-icons/react";
import { useToast } from "@/components/toast";
import { downloadReport } from "@/lib/api";
import { ReportFilters, ReportFormat, reportFormats } from "@/lib/report";

// ปุ่ม "ส่งออกรายงาน" เลือก PDF / Word / Excel ไฟล์ใช้ตัวกรองชุดเดียวกับที่เห็นบนหน้า
export function ExportReportButton({ filters, label = "ส่งออกรายงาน" }: { filters: ReportFilters; label?: string }) {
  const toast = useToast();
  const menuId = useId();
  const wrapper = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState<ReportFormat | null>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent ? event.key === "Escape" : !wrapper.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => { document.removeEventListener("mousedown", close); document.removeEventListener("keydown", close); };
  }, [open]);

  const download = async (format: ReportFormat) => {
    setOpen(false); setBusy(format);
    try {
      await downloadReport(format, filters);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "สร้างไฟล์รายงานไม่สำเร็จ");
    } finally {
      setBusy(null);
    }
  };

  return <div ref={wrapper} className="relative shrink-0">
    <button type="button" aria-haspopup="menu" aria-expanded={open} aria-controls={menuId} disabled={busy !== null} onClick={() => setOpen((current) => !current)}
      className="focus-ring inline-flex min-h-11 items-center gap-2 rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:opacity-60">
      {busy ? <SpinnerGap size={18} className="animate-spin" /> : <DownloadSimple size={18} weight="bold" />}
      {busy ? `กำลังสร้างไฟล์ ${reportFormats.find((item) => item.format === busy)?.label}...` : label}
      {!busy && <CaretDown size={14} />}
    </button>
    {open && <ul id={menuId} role="menu" aria-label="เลือกรูปแบบไฟล์" className="absolute right-0 z-20 mt-1 w-44 overflow-hidden rounded-lg border border-[var(--line)] bg-white py-1 shadow-lg">
      {reportFormats.map((item) => <li key={item.format} role="none">
        <button type="button" role="menuitem" onClick={() => download(item.format)} className="focus-ring flex min-h-10 w-full items-center px-4 text-left text-sm hover:bg-[var(--brand-blue-pale)]">{item.label} <span className="ml-auto text-xs text-[var(--muted)]">.{item.format}</span></button>
      </li>)}
    </ul>}
  </div>;
}
