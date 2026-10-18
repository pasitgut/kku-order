"use client";

import { FormEvent, useEffect, useRef, useState } from "react";
import { WarningCircle } from "@phosphor-icons/react";
import { useToast } from "@/components/toast";
import { ApiDocument, cancelDocument } from "@/lib/api";

type Props = {
  document: ApiDocument;
  onClose: () => void;
  onCancelled: (document: ApiDocument) => void;
};

export function DocumentCancellationForm({ document: target, onClose, onCancelled }: Props) {
  const toast = useToast();
  const [reason, setReason] = useState("");
  const [replacedBy, setReplacedBy] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const inFlight = useRef(false);
  const reasonRef = useRef<HTMLTextAreaElement>(null);
  const count = Array.from(reason.trim()).length;
  // มีคำสั่งใหม่มาแทนก็ยกเลิกได้โดยไม่ต้องเขียนเหตุผล ระบบเขียนว่า "ใช้คำสั่งเลขที่ … แทน" ให้
  const hasReplacement = replacedBy.trim() !== "";
  const ready = (count > 0 || hasReplacement) && count <= 2000;
  const prefix = `cancel-document-${target.id}`;

  useEffect(() => { reasonRef.current?.focus(); }, []);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (inFlight.current) return;
    if (!ready) {
      setError("กรุณาระบุเหตุผลที่ยกเลิก (ไม่เกิน 2,000 ตัวอักษร) หรือเลขที่คำสั่งที่ใช้แทน");
      reasonRef.current?.focus();
      return;
    }
    inFlight.current = true;
    setSubmitting(true);
    setError("");
    try {
      const { data } = await cancelDocument(target.id, reason.trim(), replacedBy.trim());
      toast("ยกเลิกเอกสารเรียบร้อยแล้ว");
      onCancelled(data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "ไม่สามารถยกเลิกเอกสารได้");
    } finally {
      inFlight.current = false;
      setSubmitting(false);
    }
  };

  return <form id={prefix} onSubmit={submit} aria-labelledby={`${prefix}-title`} className="max-w-3xl space-y-3 rounded-lg border border-[var(--danger)]/25 bg-[var(--danger-soft)] p-4 sm:p-5">
    <h3 id={`${prefix}-title`} className="flex items-start gap-2 text-sm font-semibold text-[var(--navy)]"><WarningCircle size={18} className="mt-0.5 shrink-0 text-[var(--danger)]" /><span className="break-words">ยกเลิกเอกสาร: {target.title || target.originalFilename}</span></h3>
    <p className="text-xs leading-5 text-[var(--muted)]">หลังยกเลิกจะเปิดดูย้อนหลังได้ แต่ไม่สามารถแก้ไขหรือยืนยันได้ และไม่มีแจ้งเตือนหมดวาระ ถ้าเคยยืนยันแล้ว รายงานจะยังแสดงคำสั่งนี้พร้อมหมายเหตุว่ายกเลิก</p>
    <label htmlFor={`${prefix}-replaced-by`} className="block text-sm font-medium">ใช้คำสั่งเลขที่ … แทน <span className="font-normal text-[var(--muted)]">(ถ้ามี)</span></label>
    <input id={`${prefix}-replaced-by`} value={replacedBy} maxLength={100} disabled={submitting} onChange={(event) => { setReplacedBy(event.target.value); setError(""); }} placeholder="เช่น 123/2569" className="focus-ring block h-11 w-full max-w-xs rounded-md border border-[var(--line-strong)] bg-[var(--surface)] px-3 text-sm disabled:opacity-60" />
    <label htmlFor={`${prefix}-reason`} className="block text-sm font-medium">เหตุผลที่ยกเลิก {hasReplacement ? <span className="font-normal text-[var(--muted)]">(ไม่บังคับเมื่อมีคำสั่งที่ใช้แทน)</span> : <span className="text-[var(--danger)]">*</span>}</label>
    <textarea ref={reasonRef} id={`${prefix}-reason`} required={!hasReplacement} maxLength={4000} rows={3} disabled={submitting} value={reason} onChange={(event) => { setReason(event.target.value); setError(""); }} aria-invalid={Boolean(error) || count > 2000} aria-describedby={`${prefix}-help ${prefix}-error`} className="focus-ring block w-full rounded-md border border-[var(--line-strong)] bg-[var(--surface)] px-3 py-2 text-sm leading-6 disabled:opacity-60" />
    <p id={`${prefix}-help`} className={`text-xs ${count > 2000 ? "text-[var(--danger)]" : "text-[var(--muted)]"}`}>{count.toLocaleString("th-TH")} / 2,000 ตัวอักษร</p>
    {error && <p id={`${prefix}-error`} role="alert" className="text-sm text-[var(--danger)]">{error}</p>}
    <div className="flex flex-wrap justify-end gap-2"><button type="button" disabled={submitting} onClick={() => { onClose(); window.requestAnimationFrame(() => window.document.getElementById(`${prefix}-button`)?.focus()); }} className="focus-ring h-11 rounded-md border border-[var(--line-strong)] bg-[var(--surface)] px-4 text-sm disabled:opacity-50">กลับไปที่รายการ</button><button type="submit" disabled={submitting || !ready} className="focus-ring h-11 rounded-md bg-[var(--danger)] px-4 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50">{submitting ? "กำลังยกเลิก..." : "ยืนยันยกเลิกเอกสาร"}</button></div>
  </form>;
}
