"use client";

import Link from "next/link";
import { ChangeEvent, DragEvent, useState } from "react";
import { ArrowRight, FileArrowUp, Info, Trash } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";
import { UploadState, useUploadQueue } from "@/components/upload-queue-context";

const stateLabels: Record<UploadState, { label: string; dot: string }> = {
  ready: { label: "พร้อมนำเข้า", dot: "bg-[#c9d1d9]" },
  queued: { label: "รอคิว", dot: "bg-[var(--brand-blue-light)]" },
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

export default function UploadPage() {
  const { files, uploading, addFiles, removeFile, clearFiles, startUpload, retryFile, skipFile } = useUploadQueue();
  const [isDragging, setIsDragging] = useState(false);

  const onFileChange = (event: ChangeEvent<HTMLInputElement>) => { if (event.target.files) addFiles(event.target.files); event.target.value = ""; };
  const onDrop = (event: DragEvent<HTMLLabelElement>) => { event.preventDefault(); setIsDragging(false); if (event.dataTransfer.files) addFiles(event.dataTransfer.files); };

  const counts = files.reduce<Record<string, number>>((result, item) => ({ ...result, [item.state]: (result[item.state] ?? 0) + 1 }), {});
  const summary = [["กำลังส่ง", (counts.queued ?? 0) + (counts.uploading ?? 0) || undefined], ["บันทึกแล้ว", counts.success], ["ไฟล์ซ้ำ", counts.duplicate], ["ไฟล์ผิด", (counts.invalid ?? 0) + (counts.error ?? 0) || undefined], ["รอนำเข้า", counts.ready]].filter(([, count]) => count).map(([label, count]) => `${label} ${count}`).join(" · ");
  const readyCount = counts.ready ?? 0;

  const dropzone = (large: boolean) => <label onDragOver={(event) => { event.preventDefault(); setIsDragging(true); }} onDragLeave={() => setIsDragging(false)} onDrop={onDrop} className={`focus-ring flex cursor-pointer flex-col items-center justify-center gap-2.5 rounded-[10px] border-2 border-dashed bg-white text-center transition ${large ? "min-h-[320px] p-10" : "p-7"} ${isDragging ? "border-[var(--accent)] bg-[var(--brand-blue-pale)]" : "border-[var(--brand-blue-light)] hover:border-[var(--accent)] hover:bg-[var(--brand-blue-pale)]"}`}>
    <input type="file" multiple accept=".pdf,.docx,.xlsx,.csv" className="sr-only" onChange={onFileChange} />
    <span className="flex h-14 w-14 items-center justify-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]"><FileArrowUp size={26} /></span>
    <span className="text-[15px] font-medium text-[var(--navy)]">{large ? "ลากไฟล์มาวางที่นี่ หรือคลิกเพื่อเลือกไฟล์" : "ลากไฟล์มาวาง หรือคลิกเพื่อเลือก"}</span>
    <span className="text-xs font-light leading-5 text-[var(--muted)]">PDF สำหรับ OCR · DOCX, XLSX, CSV นำเข้าโดยตรง<br />ไม่เกิน 50 MB ต่อไฟล์</span>
  </label>;

  return <AppShell breadcrumb="หน้าหลัก / นำเข้าเอกสาร" title="นำเข้าชุดเอกสารคำสั่งแต่งตั้ง" description="ไฟล์จะถูกบันทึกลง MySQL ก่อนเข้าสู่คิวปรับภาพและสกัดด้วยบริการ OCR">
    <div className="grid min-w-0 grid-cols-1 items-start gap-6 xl:grid-cols-3">
      <div className="min-w-0 xl:col-span-2">
        {files.length === 0 ? dropzone(true) : <section className="card flex flex-col overflow-hidden">
          <div className="flex flex-col gap-3 border-b border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
            <div><h2 className="text-[17px] font-semibold text-[var(--navy)]">ไฟล์ที่เลือก ({files.length})</h2><p className="mt-0.5 text-xs text-[var(--muted)]">{summary}</p></div>
            <div className="flex flex-wrap gap-2"><button type="button" onClick={clearFiles} disabled={uploading} className="focus-ring h-10 rounded-md border border-[var(--line-strong)] bg-white px-3.5 text-[13px] text-[var(--muted)] hover:bg-[var(--line-soft)] disabled:opacity-50">ล้างทั้งหมด</button><button type="button" onClick={startUpload} disabled={readyCount === 0} className="focus-ring inline-flex h-10 items-center gap-2 rounded-md bg-[var(--accent)] px-4 text-[13px] font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[#a6b3bf]">{readyCount ? `นำเข้า ${readyCount} ไฟล์ที่พร้อม` : uploading ? "กำลังบันทึกลงระบบ..." : "นำเข้าเอกสาร"}<ArrowRight size={16} weight="bold" /></button></div>
          </div>
          <ul className="flex-1">{files.map((item) => {
            const state = stateLabels[item.state];
            return <li key={item.id} className="border-b border-[var(--line-soft)] px-5 py-3.5 last:border-0 lg:px-6">
              <div className="flex items-center gap-3.5">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[#f1f3f6] text-[10px] font-semibold text-[var(--muted)]">{fileExtension(item.file.name)}</span>
                <div className="min-w-0 flex-1">
                  <p title={item.file.name} className="truncate text-sm font-medium">{item.file.name}</p>
                  <p className="mt-0.5 text-xs font-light text-[var(--muted)]">{(item.file.size / 1024 / 1024).toFixed(2)} MB{item.duplicateDocumentId ? <> · ตรงกับ <Link href={`/documents/${item.duplicateDocumentId}`} className="text-[var(--accent)] hover:underline">เอกสาร #{item.duplicateDocumentId}</Link></> : null}{item.message ? ` · ${item.message}` : ""}</p>
                  {item.state === "uploading" && <span className="mt-2 block h-1 overflow-hidden rounded-full bg-[var(--line)]"><span className="block h-full w-1/2 animate-pulse rounded-full bg-[var(--accent)]" /></span>}
                </div>
                {(item.state === "error" || (item.state === "duplicate" && !item.canOverwrite)) && <button type="button" onClick={() => retryFile(item.id, false)} className="focus-ring h-9 rounded-md border border-[var(--line-strong)] bg-white px-3 text-xs hover:bg-[var(--line-soft)] disabled:opacity-50">ลองอีกครั้ง</button>}
                <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-[var(--muted)]"><span className={`h-2 w-2 rounded-full ${state.dot}`} />{state.label}</span>
                {item.state !== "uploading" && item.state !== "queued" && <button type="button" aria-label={`นำ ${item.file.name} ออก`} onClick={() => removeFile(item.id)} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[var(--line-soft)] hover:text-[var(--danger)] disabled:opacity-40"><Trash size={16} /></button>}
              </div>
              {item.state === "duplicate" && item.canOverwrite && <div role="alert" className="mt-3 flex flex-col gap-2.5 rounded-lg bg-[var(--surface-muted)] px-3 py-2.5 sm:ml-[54px] sm:flex-row sm:items-center">
                <span className="flex-1 text-[13px] font-medium">พบไฟล์นี้ในระบบแล้ว ต้องการบันทึกไฟล์ใหม่แทนไฟล์เดิมหรือไม่?</span>
                <div className="flex gap-2"><button type="button" onClick={() => skipFile(item.id)} className="focus-ring h-9 rounded-md border border-[var(--line-strong)] bg-white px-3.5 text-xs hover:bg-[var(--line-soft)] disabled:opacity-50">ไม่บันทึก</button><button type="button" onClick={() => retryFile(item.id, true)} className="focus-ring h-9 rounded-md bg-[var(--accent)] px-3.5 text-xs font-medium text-white hover:bg-[var(--accent-strong)] disabled:opacity-50">บันทึกแทนไฟล์เดิม</button></div>
              </div>}
            </li>;
          })}</ul>
        </section>}
      </div>

      <aside className="flex min-w-0 flex-col gap-5">
        {files.length > 0 && dropzone(false)}
        <section className="card p-5 lg:p-6">
          <h2 className="font-semibold text-[var(--navy)]">กระบวนการหลังนำเข้า</h2>
          <ol className="mt-4 space-y-4">{[["01", "ตรวจทิศทางและปรับภาพ", "Deskew และปรับความสว่างก่อน OCR"], ["02", "สกัดข้อความด้วยบริการ OCR", "ส่งไฟล์ไปอ่านข้อความที่บริการ OCR ที่ตั้งค่าไว้ในหน้าตั้งค่า"], ["03", "จำแนกฟิลด์ข้อมูล", "แยกชื่อ ตำแหน่ง บทบาท เลขที่คำสั่ง และวันที่"], ["04", "ตรวจสอบก่อนบันทึกสมบูรณ์", "เจ้าหน้าที่แก้ไขและยืนยันทุกครั้งก่อนเป็น CONFIRMED"]].map(([step, title, description]) => <li key={step} className="flex gap-3"><span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[#f1f3f6] text-[11px] font-semibold text-[var(--muted)]">{step}</span><div><p className="text-sm font-medium">{title}</p><p className="mt-0.5 text-xs font-light leading-5 text-[var(--muted)]">{description}</p></div></li>)}</ol>
        </section>
        <section className="card flex gap-3 p-5 lg:p-6">
          <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[#f1f3f6] text-[var(--muted)]"><Info size={15} /></span>
          <div><h2 className="text-sm font-semibold text-[var(--navy)]">ก่อนนำเข้า</h2><p className="mt-1 text-[13px] font-light leading-6 text-[var(--muted)]">ระบบเทียบ hash ของไฟล์ ไฟล์เนื้อเดียวกันถือว่าซ้ำแม้ชื่อต่างกัน · เอกสารควรละเอียดไม่น้อยกว่า 300 DPI ไฟล์คุณภาพต่ำจะถูกทำเครื่องหมายให้ตรวจสอบ</p></div>
        </section>
      </aside>
    </div>

  </AppShell>;
}
