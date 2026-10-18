"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { FileArrowUp } from "@phosphor-icons/react";
import { useToast } from "@/components/toast";
import { ApiError, uploadDocument } from "@/lib/api";
import { UploadOutcome, createUploadQueue, sortByFileName } from "@/lib/upload-queue";

export type UploadState = "ready" | "queued" | "uploading" | "success" | "duplicate" | "skipped" | "invalid" | "error";
export type UploadFile = { id: number; file: File; state: UploadState; message?: string; duplicateDocumentId?: number; canOverwrite?: boolean; batch?: number };

type UploadQueue = {
  files: UploadFile[];
  uploading: boolean;
  addFiles: (incoming: FileList | File[]) => void;
  removeFile: (id: number) => void;
  clearFiles: () => void;
  startUpload: () => void;
  retryFile: (id: number, overwrite: boolean) => void;
  skipFile: (id: number) => void;
};

type UploadJob = { id: number; file: File; overwrite: boolean };

const UploadQueueContext = createContext<UploadQueue | null>(null);

// ไฟล์ใหญ่บนเน็ตช้าต้องได้เวลามากขึ้น: 2 นาที บวกเวลาส่งที่ราว 1 Mbit/s
const BASE_TIMEOUT_MS = 2 * 60 * 1000;
const SLOW_LINK_BYTES_PER_MS = (1024 * 1024) / 8 / 1000;

function validateFile(file: File): UploadState {
  return file.size > 50 * 1024 * 1024 || ![".pdf", ".docx", ".xlsx", ".csv"].some((suffix) => file.name.toLowerCase().endsWith(suffix)) ? "invalid" : "ready";
}

function isBusy(state: UploadState) {
  return state === "queued" || state === "uploading";
}

async function sendFile({ file, overwrite }: UploadJob, signal: AbortSignal): Promise<UploadOutcome> {
  try {
    await uploadDocument(file, overwrite, signal);
    return { kind: "saved" };
  } catch (reason) {
    if (reason instanceof ApiError && reason.status === 409) {
      const duplicateDocumentId = typeof reason.details.duplicateDocumentId === "number" ? reason.details.duplicateDocumentId : undefined;
      return { kind: "duplicate", duplicateDocumentId, canOverwrite: reason.details.canOverwrite === true, message: reason.message };
    }
    return { kind: "failed", message: reason instanceof Error ? reason.message : "นำเข้าไฟล์ไม่สำเร็จ" };
  }
}

// แถวที่เริ่มส่งแล้วอยู่ที่เดิม เรียงชื่อเฉพาะไฟล์ที่ยังไม่เริ่ม จะได้ไม่มีแถวกระโดดระหว่างส่ง
function arrangeFiles(files: UploadFile[]) {
  const notStarted = (item: UploadFile) => item.state === "ready" || item.state === "invalid";
  const waiting = sortByFileName(files.filter(notStarted).map((item) => ({ item, name: item.file.name }))).map(({ item }) => item);
  return [...files.filter((item) => !notStarted(item)), ...waiting];
}

// คิวอยู่ระดับ layout จึงส่งไฟล์ต่อได้แม้ผู้ใช้เปลี่ยนไปหน้าอื่นในระบบ
export function UploadQueueProvider({ children }: { children: React.ReactNode }) {
  const toast = useToast();
  const [files, setFiles] = useState<UploadFile[]>([]);
  const filesRef = useRef(files);
  const busyIds = useRef(new Set<number>());
  const nextId = useRef(1);
  const batch = useRef(0);
  const savedInBatch = useRef(0);
  const uploading = files.some((item) => isBusy(item.state));

  useEffect(() => { filesRef.current = files; }, [files]);

  const patchFile = useCallback((id: number, patch: Partial<UploadFile>) => setFiles((current) => current.map((item) => item.id === id ? { ...item, ...patch } : item)), []);

  const toastRef = useRef(toast);
  useEffect(() => { toastRef.current = toast; }, [toast]);

  const queueRef = useRef<ReturnType<typeof createUploadQueue<UploadJob>> | null>(null);

  // สร้างคิวครั้งแรกที่มีไฟล์ให้ส่ง แล้วใช้คิวเดียวนี้ตลอด
  const getQueue = useCallback(() => queueRef.current ??= createUploadQueue<UploadJob>({
    timeoutMs: (job) => BASE_TIMEOUT_MS + job.file.size / SLOW_LINK_BYTES_PER_MS,
    onStart: (job) => patchFile(job.id, { state: "uploading" }),
    send: sendFile,
    onDone: (job, outcome) => {
      busyIds.current.delete(job.id);
      if (outcome.kind === "saved") {
        savedInBatch.current += 1;
        patchFile(job.id, { state: "success", message: job.overwrite ? "บันทึกแทนไฟล์เดิมแล้ว รอระบบ OCR ประมวลผล" : "รอระบบ OCR ประมวลผล" });
      } else if (outcome.kind === "duplicate") {
        patchFile(job.id, { state: "duplicate", duplicateDocumentId: outcome.duplicateDocumentId, canOverwrite: outcome.canOverwrite, message: outcome.canOverwrite ? undefined : outcome.message });
      } else {
        patchFile(job.id, { state: "error", message: outcome.message });
      }
    },
    onIdle: () => {
      const saved = savedInBatch.current;
      savedInBatch.current = 0;
      if (saved > 0) toastRef.current(`นำเข้า ${saved} ไฟล์แล้ว · รอระบบ OCR ประมวลผล`, { action: { label: "ดูเอกสาร", href: "/documents" } });
    },
  }), [patchFile]);

  // ไฟล์ที่อยู่ในคิวหรือกำลังส่งจะไม่ถูกเพิ่มซ้ำ กดปุ่มรัวๆ ก็ส่งไฟล์ละครั้งเดียว
  const enqueue = useCallback((ids: number[], overwrite: boolean) => {
    const jobs = ids.filter((id) => !busyIds.current.has(id)).flatMap((id) => {
      const item = filesRef.current.find((entry) => entry.id === id);
      return item ? [{ id, file: item.file, overwrite }] : [];
    });
    if (jobs.length === 0) return;
    if (busyIds.current.size === 0) batch.current += 1;
    for (const job of jobs) {
      busyIds.current.add(job.id);
      patchFile(job.id, { state: "queued", message: undefined, batch: batch.current });
    }
    getQueue().add(jobs);
  }, [patchFile, getQueue]);

  const addFiles = useCallback((incoming: FileList | File[]) => {
    const added = Array.from(incoming).map((file) => {
      const state = validateFile(file);
      return { id: nextId.current++, file, state, message: state === "invalid" ? "รองรับ PDF, DOCX, XLSX, CSV และขนาดไม่เกิน 50 MB" : undefined };
    });
    setFiles((current) => arrangeFiles([...current, ...added]));
  }, []);

  const startUpload = useCallback(() => enqueue(filesRef.current.filter((item) => item.state === "ready").map((item) => item.id), false), [enqueue]);
  const retryFile = useCallback((id: number, overwrite: boolean) => enqueue([id], overwrite), [enqueue]);
  const skipFile = useCallback((id: number) => patchFile(id, { state: "skipped", canOverwrite: false }), [patchFile]);
  const removeFile = useCallback((id: number) => setFiles((current) => current.filter((item) => item.id !== id || isBusy(item.state))), []);
  const clearFiles = useCallback(() => setFiles((current) => current.filter((item) => isBusy(item.state))), []);

  // ไฟล์อยู่ในเบราว์เซอร์ ปิดแท็บหรือรีเฟรชระหว่างส่งจะทำให้ไฟล์ที่เหลือไม่เข้าระบบ
  useEffect(() => {
    if (!uploading) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [uploading]);

  const value = useMemo(() => ({ files, uploading, addFiles, removeFile, clearFiles, startUpload, retryFile, skipFile }), [files, uploading, addFiles, removeFile, clearFiles, startUpload, retryFile, skipFile]);

  return <UploadQueueContext.Provider value={value}>
    {children}
    <UploadProgress files={files} />
  </UploadQueueContext.Provider>;
}

export function useUploadQueue() {
  const queue = useContext(UploadQueueContext);
  if (!queue) throw new Error("useUploadQueue must be used inside UploadQueueProvider");
  return queue;
}

// นับเฉพาะชุดที่กำลังส่งอยู่ ไม่รวมไฟล์จากชุดก่อนหน้า
function UploadProgress({ files }: { files: UploadFile[] }) {
  const pathname = usePathname();
  const busy = files.filter((item) => isBusy(item.state));
  if (pathname === "/upload" || busy.length === 0) return null;
  const currentBatch = Math.max(...busy.map((item) => item.batch ?? 0));
  const inBatch = files.filter((item) => item.batch === currentBatch);
  const finished = inBatch.filter((item) => !isBusy(item.state)).length;
  return <Link href="/upload" role="status" className="focus-ring fixed bottom-24 left-4 z-40 flex items-center gap-3 rounded-[10px] bg-[var(--navy)] px-4 py-3 text-sm text-white shadow-[0_12px_32px_rgba(0,0,0,0.25)] lg:bottom-8 lg:left-8">
    <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--accent)]"><FileArrowUp size={18} /></span>
    <span><span className="block font-medium">กำลังอัปโหลด {finished}/{inBatch.length}</span><span className="block text-xs font-light text-[var(--brand-blue-text)]">อย่าปิดแท็บนี้จนกว่าจะเสร็จ</span></span>
  </Link>;
}
