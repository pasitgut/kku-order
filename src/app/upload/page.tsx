"use client";

import Link from "next/link";
import { ChangeEvent, DragEvent, useState } from "react";
import { ArrowRight, CheckCircle, CloudArrowUp, FileCsv, FileDoc, FilePdf, FileXls, Info, Trash, WarningCircle } from "@phosphor-icons/react";
import { AppShell, SectionTitle } from "@/components/app-shell";
import { ApiError, uploadDocument } from "@/lib/api";

type UploadState = "ready" | "uploading" | "success" | "duplicate" | "invalid" | "error";
type UploadFile = { file: File; state: UploadState; message?: string };

function FileIcon({ name }: { name: string }) {
  if (name.endsWith(".pdf")) return <FilePdf size={25} weight="duotone" className="text-[#c34b4b]" />;
  if (name.endsWith(".docx")) return <FileDoc size={25} weight="duotone" className="text-[#3979a7]" />;
  if (name.endsWith(".xlsx")) return <FileXls size={25} weight="duotone" className="text-[#21855b]" />;
  return <FileCsv size={25} weight="duotone" className="text-[#7c65a4]" />;
}

function validateFile(file: File): UploadState {
  return file.size > 50 * 1024 * 1024 || ![".pdf", ".docx", ".xlsx", ".csv"].some((suffix) => file.name.toLowerCase().endsWith(suffix)) ? "invalid" : "ready";
}

export default function UploadPage() {
  const [files, setFiles] = useState<UploadFile[]>([]);
  const [isDragging, setIsDragging] = useState(false);
  const [processing, setProcessing] = useState(false);

  const addFiles = (incoming: FileList | File[]) => {
    const next = Array.from(incoming).map((file) => ({ file, state: validateFile(file), message: validateFile(file) === "invalid" ? "รองรับ PDF, DOCX, XLSX, CSV และขนาดไม่เกิน 50 MB" : undefined }));
    setFiles((current) => [...current, ...next]);
  };

  const onFileChange = (event: ChangeEvent<HTMLInputElement>) => { if (event.target.files) addFiles(event.target.files); event.target.value = ""; };
  const onDrop = (event: DragEvent<HTMLLabelElement>) => { event.preventDefault(); setIsDragging(false); if (event.dataTransfer.files) addFiles(event.dataTransfer.files); };

  const startProcessing = async () => {
    const readyFiles = files.map((item, index) => ({ item, index })).filter(({ item }) => item.state === "ready");
    if (!readyFiles.length) return;
    setProcessing(true);
    for (const { item, index } of readyFiles) {
      setFiles((current) => current.map((entry, currentIndex) => currentIndex === index ? { ...entry, state: "uploading" } : entry));
      try {
        await uploadDocument(item.file);
        setFiles((current) => current.map((entry, currentIndex) => currentIndex === index ? { ...entry, state: "success", message: "บันทึกในฐานข้อมูลแล้ว และรอ one-ocr ประมวลผล" } : entry));
      } catch (reason) {
        if (reason instanceof ApiError && reason.status === 409 && reason.details.canOverwrite === true) {
          const overwrite = window.confirm("พบไฟล์นี้ในระบบแล้ว ต้องการบันทึกไฟล์ใหม่ทับรายการเดิมหรือไม่?");
          if (overwrite) {
            try {
              await uploadDocument(item.file, true);
              setFiles((current) => current.map((entry, currentIndex) => currentIndex === index ? { ...entry, state: "success", message: "บันทึกทับรายการเดิมแล้ว และรอ one-ocr ประมวลผล" } : entry));
              continue;
            } catch (overwriteReason) {
              reason = overwriteReason;
            }
          }
        }
        const message = reason instanceof Error ? reason.message : "นำเข้าไฟล์ไม่สำเร็จ";
        setFiles((current) => current.map((entry, currentIndex) => currentIndex === index ? { ...entry, state: message.includes("ซ้ำ") ? "duplicate" : "error", message } : entry));
      }
    }
    setProcessing(false);
  };

  return <AppShell><div className="mx-auto max-w-[1200px] space-y-8">
    <SectionTitle eyebrow="นำเข้าเอกสาร" title="นำเข้าชุดเอกสารคำสั่งแต่งตั้ง" description="ไฟล์จะถูกบันทึกลง MySQL จริงก่อนเข้าสู่คิวปรับภาพและสกัดด้วย one-ocr ภายในระบบ" />
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.65fr)]">
      <section className="space-y-5"><label onDragOver={(event) => { event.preventDefault(); setIsDragging(true); }} onDragLeave={() => setIsDragging(false)} onDrop={onDrop} className={`focus-ring block cursor-pointer rounded-xl border-2 border-dashed bg-white p-8 text-center transition sm:p-12 ${isDragging ? "border-[var(--accent)] bg-[var(--accent-soft)]" : "border-[#c6d4df] hover:border-[var(--accent)] hover:bg-[#fbfefe]"}`}><input type="file" multiple accept=".pdf,.docx,.xlsx,.csv" className="sr-only" onChange={onFileChange} /><span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--accent-soft)] text-[var(--accent)]"><CloudArrowUp size={30} weight="duotone" /></span><span className="mt-5 block text-base font-bold text-[var(--ink)]">ลากไฟล์มาวางที่นี่ หรือคลิกเพื่อเลือกไฟล์</span><span className="mt-2 block text-sm text-[var(--muted)]">PDF สำหรับ OCR และ DOCX, XLSX, CSV สำหรับนำเข้าข้อมูลโดยตรง</span><span className="mt-5 inline-flex items-center gap-2 text-xs font-semibold text-[var(--accent)]"><Info size={15} /> ขนาดไม่เกิน 50 MB ต่อไฟล์ · ตรวจ hash ไฟล์ซ้ำใน backend</span></label>
        {files.length > 0 && <div className="overflow-hidden rounded-xl border border-[var(--line)] bg-white"><div className="flex items-center justify-between border-b border-[var(--line)] px-5 py-4"><div><h2 className="text-sm font-bold text-[var(--ink)]">ไฟล์ที่เลือก ({files.length})</h2><p className="mt-1 text-xs text-[var(--muted)]">รายการนี้จะถูกส่งเข้า API และบันทึกลง MySQL</p></div><button type="button" onClick={() => setFiles([])} className="focus-ring text-xs font-semibold text-[var(--muted)] hover:text-[var(--danger)]">ล้างทั้งหมด</button></div><div className="divide-y divide-[#edf0f3]">{files.map((item, index) => <div key={`${item.file.name}-${index}`} className="flex items-center gap-3 px-5 py-3.5"><FileIcon name={item.file.name.toLowerCase()} /><div className="min-w-0 flex-1"><p className="truncate text-sm font-semibold text-[var(--ink)]">{item.file.name}</p><p className="mt-1 text-xs text-[var(--muted)]">{(item.file.size / 1024 / 1024).toFixed(2)} MB</p>{item.message && <p className={`mt-1 text-xs ${item.state === "error" || item.state === "invalid" ? "text-[var(--danger)]" : "text-[var(--muted)]"}`}>{item.message}</p>}</div>{item.state === "ready" && <span className="text-xs font-semibold text-[var(--accent)]">พร้อมนำเข้า</span>}{item.state === "uploading" && <span className="text-xs font-semibold text-[var(--warning)]">กำลังส่งไฟล์</span>}{item.state === "success" && <span className="inline-flex items-center gap-1 text-xs font-semibold text-[var(--success)]"><CheckCircle size={16} weight="fill" /> บันทึกแล้ว</span>}{item.state === "duplicate" && <span className="inline-flex items-center gap-1 text-xs font-semibold text-[var(--warning)]"><WarningCircle size={16} weight="fill" /> ไฟล์ซ้ำ</span>}{(item.state === "invalid" || item.state === "error") && <span className="inline-flex items-center gap-1 text-xs font-semibold text-[var(--danger)]"><WarningCircle size={16} weight="fill" /> ตรวจสอบไฟล์</span>}<button type="button" aria-label={`ลบ ${item.file.name}`} onClick={() => setFiles((current) => current.filter((_, fileIndex) => fileIndex !== index))} className="focus-ring ml-2 rounded-md p-1.5 text-[var(--muted)] hover:bg-[var(--danger-soft)] hover:text-[var(--danger)]"><Trash size={16} /></button></div>)}</div></div>}
        {files.some((item) => item.state === "success") && <div className="flex items-center gap-3 rounded-xl border border-[#bfe6d2] bg-[var(--success-soft)] p-4 text-sm text-[var(--success)]"><CheckCircle size={21} weight="fill" /><span>ไฟล์ถูกบันทึกในระบบแล้ว สามารถเปิดรายการเพื่อตรวจสอบผล one-ocr ได้</span><Link href="/documents" className="ml-auto shrink-0 font-bold underline">ดูเอกสาร</Link></div>}
        <div className="flex flex-col justify-end gap-3 sm:flex-row"><Link href="/" className="focus-ring inline-flex h-11 items-center justify-center rounded-lg border border-[var(--line)] px-5 text-sm font-semibold text-[var(--muted)] hover:border-[var(--navy)] hover:text-[var(--navy)]">ยกเลิก</Link><button type="button" onClick={startProcessing} disabled={processing || !files.some((item) => item.state === "ready")} className="focus-ring inline-flex h-11 items-center justify-center gap-2 rounded-lg bg-[var(--accent)] px-5 text-sm font-semibold text-white transition hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[#a6b3bf]">{processing ? "กำลังบันทึกลงระบบ..." : "นำเข้าเอกสาร"}<ArrowRight size={17} weight="bold" /></button></div>
      </section>
      <aside className="space-y-5"><div className="rounded-xl border border-[var(--line)] bg-white p-5"><h2 className="text-base font-bold text-[var(--ink)]">กระบวนการหลังนำเข้า</h2><div className="mt-5 space-y-5">{[["01", "ตรวจทิศทางและปรับภาพ", "Deskew และปรับความสว่างก่อน OCR"], ["02", "สกัดข้อความด้วย one-ocr", "ประมวลผลภาษาไทย อังกฤษ เลขไทย และเลขอารบิกในเครื่อง"], ["03", "จำแนกฟิลด์ข้อมูล", "แยกชื่อ ตำแหน่ง บทบาท เลขที่คำสั่ง และวันที่"], ["04", "ตรวจสอบก่อนบันทึกสมบูรณ์", "เจ้าหน้าที่แก้ไขและยืนยันทุกครั้งก่อนเป็น CONFIRMED"]].map(([step, title, description]) => <div key={step} className="flex gap-3"><span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[#edf3f7] text-[11px] font-bold text-[var(--accent)]">{step}</span><div><p className="text-sm font-semibold text-[var(--ink)]">{title}</p><p className="mt-1 text-xs leading-5 text-[var(--muted)]">{description}</p></div></div>)}</div></div><div className="rounded-xl border border-[#d4e9ec] bg-[#edf8f7] p-5"><div className="flex gap-3"><Info size={20} weight="duotone" className="mt-0.5 shrink-0 text-[var(--accent)]" /><div><h2 className="text-sm font-bold text-[var(--ink)]">ข้อควรรู้ตาม TOR</h2><p className="mt-2 text-xs leading-5 text-[#55746f]">เอกสารมาตรฐานควรมีความละเอียดไม่น้อยกว่า 300 DPI ส่วนไฟล์คุณภาพต่ำจะถูกทำเครื่องหมายให้ตรวจสอบ</p></div></div></div></aside>
    </div>
  </div></AppShell>;
}
