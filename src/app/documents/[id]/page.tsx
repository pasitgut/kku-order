"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowLeft } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";
import { DocumentWorkspace } from "@/components/document-workspace";
import { ApiDocument, getDocument } from "@/lib/api";

export default function DocumentPage({ params }: { params: Promise<{ id: string }> }) {
  const [document, setDocument] = useState<ApiDocument | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    let timer: number | undefined;
    params.then(({ id }) => {
      const load = () => getDocument(id).then((response) => {
        if (!active) return;
        setDocument(response.data);
        if (response.data.status === "PROCESSING") timer = window.setTimeout(load, 3000);
      }).catch((reason: Error) => { if (active) setError(reason.message); });
      load();
    });
    return () => { active = false; if (timer) window.clearTimeout(timer); };
  }, [params]);

  if (error) return <AppShell><div className="mx-auto max-w-xl rounded-xl border border-[var(--danger-soft)] bg-[var(--danger-soft)] p-8 text-center"><p className="font-semibold text-[var(--danger)]">{error}</p><Link href="/documents" className="mt-5 inline-flex items-center gap-2 font-semibold text-[var(--accent)]"><ArrowLeft size={17} /> กลับไปเอกสารทั้งหมด</Link></div></AppShell>;
  if (!document) return <AppShell><div className="mx-auto max-w-xl rounded-xl border border-[var(--line)] bg-white p-8 text-center"><div className="skeleton mx-auto h-6 w-48" /><div className="skeleton mx-auto mt-3 h-4 w-72" /><p className="mt-6 text-sm text-[var(--muted)]">กำลังโหลดข้อมูลจาก backend...</p></div></AppShell>;
  return <DocumentWorkspace key={`${document.id}-${document.updatedAt}`} document={document} />;
}
