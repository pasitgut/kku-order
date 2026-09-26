"use client";

import Link from "next/link";
import { ChangeEvent, DragEvent, useEffect, useRef, useState } from "react";
import { ArrowRight, Copy, FileArrowUp, Info, Trash } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";
import { useToast } from "@/components/toast";
import { ApiError, uploadDocument } from "@/lib/api";

type UploadState = "ready" | "uploading" | "success" | "duplicate" | "skipped" | "invalid" | "error";
type UploadFile = { file: File; state: UploadState; message?: string; duplicateDocumentId?: number; canOverwrite?: boolean };
type OverwriteQuestion = { file: File; duplicateDocumentId?: number; resolve: (overwrite: boolean) => void };

const stateLabels: Record<UploadState, { label: string; dot: string }> = {
  ready: { label: "พร้อมนำเข้า", dot: "bg-[#c9d1d9]" },
  uploading: { label: "กำลังส่งไฟล์", dot: "bg-[var(--accent)]" },
  success: { label: "บันทึกแล้ว", dot: "bg-[var(--success)]" },
  duplicate: { label: "ไฟล์ซ้ำ", dot: "bg-[var(--warning-dot)]" },
  skipped: { label: "ไม่ได้บันทึก", dot: "bg-[#c9d1d9]" },
  invalid: { label: "ไฟล์ผิด", dot: "bg-[var(--highlight)]" },
  error: { label: "นำเข้าไม่สำเร็จ", dot: "bg-[var(--highlight)]" },
};

function fileExtension(name: string) {
  return name.split(".").pop()?.toUpperCase().slice(0, 4) || "FILE";
}

function validateFile(file: File): UploadState {
  return file.size > 50 * 1024 * 1024 || ![".pdf", ".docx", ".xlsx", ".csv"].some((suffix) => file.name.toLowerCase().endsWith(suffix)) ? "invalid" : "ready";
}

export default function UploadPage() {
  const toast = useToast();
  const [files, setFiles] = useState<UploadFile[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [question, setQuestion] = useState<OverwriteQuestion | null>(null);

  const addFiles = (incoming: FileList | File[]) => {
    const next = Array.from(incoming).map((file) => ({ file, state: validateFile(file), message: validateFile(file) === "invalid" ? "รองรับ PDF, DOCX, XLSX, CSV และขนาดไม่เกิน 50 MB" : undefined }));
    setFiles((current) => [...current, ...next]);
  };

  const onFileChange = (event: ChangeEvent<HTMLInputElement>) => { if (event.target.files) addFiles(event.target.files); event.target.value = ""; };
  const onDrop = (event: DragEvent<HTMLLabelElement>) => { event.preventDefault(); setIsDragging(false); if (event.dataTransfer.files) addFiles(event.dataTransfer.files); };
  const patchFile = (index: number, patch: Partial<UploadFile>) => setFiles((current) => current.map((entry, currentIndex) => currentIndex === index ? { ...entry, ...patch } : entry));

  const askOverwrite = (file: File, duplicateDocumentId?: number) => new Promise<boolean>((resolve) => setQuestion({ file, duplicateDocumentId, resolve }));
  const answer = (overwrite: boolean) => { question?.resolve(overwrite); setQuestion(null); };

  // คืน true เมื่อบันทึกสำเร็จ ส่วนไฟล์ซ้ำที่ไม่ได้บันทึกทับจะค้างสถานะไว้ให้ตัดสินใจภายหลัง
  const uploadOne = async (index: number, file: File, overwrite: boolean, askOnDuplicate: boolean): Promise<boolean> => {
    patchFile(index, { state: "uploading", message: undefined });
    try {
      await uploadDocument(file, overwrite);
      patchFile(index, { state: "success", message: overwrite ? "บันทึกแทนไฟล์เดิมแล้ว รอ one-ocr ประมวลผล" : "รอ one-ocr ประมวลผล" });
      return true;
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 409) {
        const duplicateDocumentId = typeof reason.details.duplicateDocumentId === "number" ? reason.details.duplicateDocumentId : undefined;
        const canOverwrite = reason.details.canOverwrite === true;
        if (canOverwrite && askOnDuplicate && await askOverwrite(file, duplicateDocumentId)) return uploadOne(index, file, true, false);
        patchFile(index, { state: "duplicate", duplicateDocumentId, canOverwrite, message: canOverwrite ? undefined : reason.message });
        return false;
      }
      patchFile(index, { state: "error", message: reason instanceof Error ? reason.message : "นำเข้าไฟล์ไม่สำเร็จ" });
      return false;
    }
  };

  const announce = (saved: number) => {
    if (saved > 0) toast(`นำเข้า ${saved} ไฟล์แล้ว · รอ one-ocr ประมวลผล`, { action: { label: "ดูเอกสาร", href: "/documents" } });
  };

  const startProcessing = async () => {
    const readyFiles = files.map((item, index) => ({ item, index })).filter(({ item }) => item.state === "ready");
    if (!readyFiles.length) return;
    setProcessing(true);
    let saved = 0;
    for (const { item, index } of readyFiles) {
      if (await uploadOne(index, item.file, false, true)) saved += 1;
    }
    setProcessing(false);
    announce(saved);
  };

  const retry = async (index: number, overwrite: boolean) => {
    setProcessing(true);
    const saved = await uploadOne(index, files[index].file, overwrite, false);
    setProcessing(false);
    announce(saved ? 1 : 0);
  };

  const counts = files.reduce<Record<string, number>>((result, item) => ({ ...result, [item.state]: (result[item.state] ?? 0) + 1 }), {});
  const summary = [["บันทึกแล้ว", counts.success], ["ไฟล์ซ้ำ", counts.duplicate], ["ไฟล์ผิด", (counts.invalid ?? 0) + (counts.error ?? 0) || undefined], ["รอนำเข้า", counts.ready]].filter(([, count]) => count).map(([label, count]) => `${label} ${count}`).join(" · ");
  const readyCount = counts.ready ?? 0;

  const dropzone = (large: boolean) => <label onDragOver={(event) => { event.preventDefault(); setIsDragging(true); }} onDragLeave={() => setIsDragging(false)} onDrop={onDrop} className={`focus-ring flex cursor-pointer flex-col items-center justify-center gap-2.5 rounded-[10px] border-2 border-dashed bg-white text-center transition ${large ? "min-h-[320px] p-10" : "p-7"} ${isDragging ? "border-[var(--accent)] bg-[var(--brand-blue-pale)]" : "border-[var(--brand-blue-light)] hover:border-[var(--accent)] hover:bg-[var(--brand-blue-pale)]"}`}>
    <input type="file" multiple accept=".pdf,.docx,.xlsx,.csv" className="sr-only" onChange={onFileChange} />
    <span className="flex h-14 w-14 items-center justify-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]"><FileArrowUp size={26} /></span>
    <span className="text-[15px] font-medium text-[var(--navy)]">{large ? "ลากไฟล์มาวางที่นี่ หรือคลิกเพื่อเลือกไฟล์" : "ลากไฟล์มาวาง หรือคลิกเพื่อเลือก"}</span>
    <span className="text-xs font-light leading-5 text-[var(--muted)]">PDF สำหรับ OCR · DOCX, XLSX, CSV นำเข้าโดยตรง<br />ไม่เกิน 50 MB ต่อไฟล์</span>
  </label>;

  return <AppShell breadcrumb="หน้าหลัก / นำเข้าเอกสาร" title="นำเข้าชุดเอกสารคำสั่งแต่งตั้ง" description="ไฟล์จะถูกบันทึกลง MySQL ก่อนเข้าสู่คิวปรับภาพและสกัดด้วย one-ocr">
    <div className="grid items-start gap-6 xl:grid-cols-3">
      <div className="xl:col-span-2">
        {files.length === 0 ? dropzone(true) : <section className="card flex flex-col overflow-hidden">
          <div className="flex flex-col gap-3 border-b border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
            <div><h2 className="text-[17px] font-semibold text-[var(--navy)]">ไฟล์ที่เลือก ({files.length})</h2><p className="mt-0.5 text-xs text-[var(--muted)]">{summary}</p></div>
            <div className="flex gap-2"><button type="button" onClick={() => setFiles([])} disabled={processing} className="focus-ring h-10 rounded-md border border-[var(--line-strong)] bg-white px-3.5 text-[13px] text-[var(--muted)] hover:bg-[var(--line-soft)] disabled:opacity-50">ล้างทั้งหมด</button><button type="button" onClick={startProcessing} disabled={processing || readyCount === 0} className="focus-ring inline-flex h-10 items-center gap-2 rounded-md bg-[var(--accent)] px-4 text-[13px] font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[#a6b3bf]">{processing ? "กำลังบันทึกลงระบบ..." : readyCount ? `นำเข้า ${readyCount} ไฟล์ที่พร้อม` : "นำเข้าเอกสาร"}<ArrowRight size={16} weight="bold" /></button></div>
          </div>
          <ul className="flex-1">{files.map((item, index) => {
            const state = stateLabels[item.state];
            return <li key={`${item.file.name}-${index}`} className="border-b border-[var(--line-soft)] px-5 py-3.5 last:border-0 lg:px-6">
              <div className="flex items-center gap-3.5">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[#f1f3f6] text-[10px] font-semibold text-[var(--muted)]">{fileExtension(item.file.name)}</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{item.file.name}</p>
                  <p className="mt-0.5 text-xs font-light text-[var(--muted)]">{(item.file.size / 1024 / 1024).toFixed(2)} MB{item.duplicateDocumentId ? <> · ตรงกับ <Link href={`/documents/${item.duplicateDocumentId}`} className="text-[var(--accent)] hover:underline">เอกสาร #{item.duplicateDocumentId}</Link></> : null}{item.message ? ` · ${item.message}` : ""}</p>
                  {item.state === "uploading" && <span className="mt-2 block h-1 overflow-hidden rounded-full bg-[var(--line)]"><span className="block h-full w-1/2 animate-pulse rounded-full bg-[var(--accent)]" /></span>}
                </div>
                {item.state === "duplicate" && !item.canOverwrite && <button type="button" disabled={processing} onClick={() => retry(index, false)} className="focus-ring h-9 rounded-md border border-[var(--line-strong)] bg-white px-3 text-xs hover:bg-[var(--line-soft)] disabled:opacity-50">ลองอีกครั้ง</button>}
                <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-[var(--muted)]"><span className={`h-2 w-2 rounded-full ${state.dot}`} />{state.label}</span>
                {item.state !== "uploading" && <button type="button" aria-label={`นำ ${item.file.name} ออก`} disabled={processing} onClick={() => setFiles((current) => current.filter((_, fileIndex) => fileIndex !== index))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[var(--line-soft)] hover:text-[var(--danger)] disabled:opacity-40"><Trash size={16} /></button>}
              </div>
              {item.state === "duplicate" && item.canOverwrite && <div role="alert" className="mt-3 flex flex-col gap-2.5 rounded-lg bg-[var(--surface-muted)] px-3 py-2.5 sm:ml-[54px] sm:flex-row sm:items-center">
                <span className="flex-1 text-[13px] font-medium">พบไฟล์นี้ในระบบแล้ว ต้องการบันทึกไฟล์ใหม่แทนไฟล์เดิมหรือไม่?</span>
                <div className="flex gap-2"><button type="button" disabled={processing} onClick={() => patchFile(index, { state: "skipped", canOverwrite: false })} className="focus-ring h-9 rounded-md border border-[var(--line-strong)] bg-white px-3.5 text-xs hover:bg-[var(--line-soft)] disabled:opacity-50">ยกเลิก</button><button type="button" disabled={processing} onClick={() => retry(index, true)} className="focus-ring h-9 rounded-md bg-[var(--accent)] px-3.5 text-xs font-medium text-white hover:bg-[var(--accent-strong)] disabled:opacity-50">บันทึกแทนไฟล์เดิม</button></div>
              </div>}
            </li>;
          })}</ul>
        </section>}
      </div>

      <aside className="flex flex-col gap-5">
        {files.length > 0 && dropzone(false)}
        <section className="card p-5 lg:p-6">
          <h2 className="font-semibold text-[var(--navy)]">กระบวนการหลังนำเข้า</h2>
          <ol className="mt-4 space-y-4">{[["01", "ตรวจทิศทางและปรับภาพ", "Deskew และปรับความสว่างก่อน OCR"], ["02", "สกัดข้อความด้วย one-ocr", "ประมวลผลภาษาไทย อังกฤษ เลขไทย และเลขอารบิกในเครื่อง"], ["03", "จำแนกฟิลด์ข้อมูล", "แยกชื่อ ตำแหน่ง บทบาท เลขที่คำสั่ง และวันที่"], ["04", "ตรวจสอบก่อนบันทึกสมบูรณ์", "เจ้าหน้าที่แก้ไขและยืนยันทุกครั้งก่อนเป็น CONFIRMED"]].map(([step, title, description]) => <li key={step} className="flex gap-3"><span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[#f1f3f6] text-[11px] font-semibold text-[var(--muted)]">{step}</span><div><p className="text-sm font-medium">{title}</p><p className="mt-0.5 text-xs font-light leading-5 text-[var(--muted)]">{description}</p></div></li>)}</ol>
        </section>
        <section className="card flex gap-3 p-5 lg:p-6">
          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[#f1f3f6] text-[var(--muted)]"><Info size={15} /></span>
          <div><h2 className="text-sm font-semibold text-[var(--navy)]">ก่อนนำเข้า</h2><p className="mt-1 text-[13px] font-light leading-6 text-[var(--muted)]">ระบบเทียบ hash ของไฟล์ ไฟล์เนื้อเดียวกันถือว่าซ้ำแม้ชื่อต่างกัน · เอกสารควรละเอียดไม่น้อยกว่า 300 DPI ไฟล์คุณภาพต่ำจะถูกทำเครื่องหมายให้ตรวจสอบ</p></div>
        </section>
      </aside>
    </div>

    {question && <DuplicateFileDialog question={question} onAnswer={answer} />}
  </AppShell>;
}

function DuplicateFileDialog({ question, onAnswer }: { question: OverwriteQuestion; onAnswer: (overwrite: boolean) => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog && !dialog.open) dialog.showModal();
  }, []);

  return <dialog ref={dialogRef} aria-labelledby="duplicate-file-title" aria-describedby="duplicate-file-question" onCancel={(event) => { event.preventDefault(); onAnswer(false); }} className="m-auto w-[min(92vw,520px)] rounded-xl bg-white p-0 text-[var(--ink)] shadow-[0_20px_48px_rgba(0,0,0,0.3)] backdrop:bg-[rgba(3,20,69,0.62)]">
    <div className="flex flex-col items-center gap-3.5 px-7 pb-5 pt-7 text-center">
      <span className="flex h-14 w-14 items-center justify-center rounded-full bg-[var(--warning-soft)] text-[var(--warning)]"><Copy size={26} /></span>
      <div><h2 id="duplicate-file-title" className="text-xl font-semibold text-[var(--navy)]">พบไฟล์นี้ในระบบแล้ว</h2><p id="duplicate-file-question" className="mt-1.5 text-[15px]">ต้องการบันทึกไฟล์ใหม่แทนไฟล์เดิมหรือไม่?</p></div>
    </div>
    <div className="mx-7 overflow-hidden rounded-lg border border-[var(--line)] text-left">
      <div className="flex items-center gap-3 px-3.5 py-3"><span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#f1f3f6] text-[10px] font-semibold text-[var(--muted)]">{fileExtension(question.file.name)}</span><span className="min-w-0 flex-1"><span className="block text-[11px] text-[var(--muted)]">ไฟล์ใหม่</span><span className="block truncate text-sm font-medium">{question.file.name}</span></span><span className="text-xs text-[var(--muted)]">{(question.file.size / 1024 / 1024).toFixed(2)} MB</span></div>
      {question.duplicateDocumentId && <div className="flex items-center gap-3 border-t border-[var(--line)] bg-[var(--surface-muted)] px-3.5 py-3"><span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[10px] font-semibold text-[var(--accent)]">#{question.duplicateDocumentId}</span><span className="min-w-0 flex-1"><span className="block text-[11px] text-[var(--muted)]">ไฟล์เดิม</span><span className="block text-sm font-medium">เอกสาร #{question.duplicateDocumentId}</span></span><Link href={`/documents/${question.duplicateDocumentId}`} target="_blank" className="focus-ring text-xs font-medium text-[var(--accent)] hover:underline">เปิดดู</Link></div>}
    </div>
    <p className="mx-7 mt-3.5 text-center text-xs font-light text-[var(--muted)]">ข้อมูลที่ตรวจไว้ในไฟล์เดิมจะถูกสกัดใหม่ทั้งหมด</p>
    <div className="grid grid-cols-2 gap-2.5 px-7 pb-6 pt-5">
      <button type="button" autoFocus onClick={() => onAnswer(false)} className="focus-ring h-[46px] rounded-md border border-[var(--line-strong)] bg-white text-sm hover:bg-[var(--line-soft)]">ยกเลิก</button>
      <button type="button" onClick={() => onAnswer(true)} className="focus-ring h-[46px] rounded-md bg-[var(--danger)] text-sm font-medium text-white hover:bg-[#8f2a0c]">บันทึกแทนไฟล์เดิม</button>
    </div>
  </dialog>;
}
