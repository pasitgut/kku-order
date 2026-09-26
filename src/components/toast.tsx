"use client";

import Link from "next/link";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { Check, ExclamationMark, X } from "@phosphor-icons/react";

type ToastKind = "success" | "error";
type ToastAction = { label: string; href: string };
type Toast = { id: number; message: string; kind: ToastKind; action?: ToastAction };
type ToastOptions = { kind?: ToastKind; action?: ToastAction };

const ToastContext = createContext<(message: string, options?: ToastOptions) => void>(() => {});

const TOAST_DURATION_MS = 4000;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const dismiss = useCallback((id: number) => setToasts((current) => current.filter((toast) => toast.id !== id)), []);

  const show = useCallback((message: string, options: ToastOptions = {}) => {
    const id = nextId.current++;
    // เก็บไว้ไม่เกิน 3 อัน ของเก่าที่ยังไม่หายจะถูกดันออก
    setToasts((current) => [...current.slice(-2), { id, message, kind: options.kind ?? "success", action: options.action }]);
  }, []);

  return <ToastContext.Provider value={show}>
    {children}
    <div aria-live="polite" className="pointer-events-none fixed inset-x-4 bottom-24 z-50 flex flex-col items-center gap-2 sm:inset-x-auto sm:right-8 sm:bottom-8 sm:items-end lg:bottom-10">
      {toasts.map((toast) => <ToastItem key={toast.id} toast={toast} onDismiss={dismiss} />)}
    </div>
  </ToastContext.Provider>;
}

function ToastItem({ toast, onDismiss }: { toast: Toast; onDismiss: (id: number) => void }) {
  useEffect(() => {
    const timer = window.setTimeout(() => onDismiss(toast.id), TOAST_DURATION_MS);
    return () => window.clearTimeout(timer);
  }, [toast.id, onDismiss]);

  const Icon = toast.kind === "error" ? ExclamationMark : Check;
  return <div role={toast.kind === "error" ? "alert" : "status"} className="toast-enter pointer-events-auto flex w-full max-w-[460px] items-center gap-3 rounded-lg bg-[var(--bar)] py-2.5 pl-4 pr-2 text-sm text-white shadow-[0_8px_24px_rgba(0,0,0,0.25)] sm:w-auto sm:min-w-[360px]">
    <span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full ${toast.kind === "error" ? "bg-[var(--danger)]" : "bg-[var(--success)]"}`}><Icon size={14} weight="bold" /></span>
    <span className="min-w-0 flex-1">{toast.message}</span>
    {toast.action && <Link href={toast.action.href} onClick={() => onDismiss(toast.id)} className="focus-ring whitespace-nowrap rounded px-2 py-2 text-[13px] font-medium text-[var(--brand-blue-light)] hover:text-white">{toast.action.label}</Link>}
    <button type="button" aria-label="ปิด" onClick={() => onDismiss(toast.id)} className="focus-ring flex h-9 w-9 shrink-0 items-center justify-center rounded text-[var(--bar-muted)] hover:text-white"><X size={16} /></button>
  </div>;
}

export function useToast() {
  return useContext(ToastContext);
}
