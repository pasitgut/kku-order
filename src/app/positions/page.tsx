"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { PencilSimple, SpinnerGap } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";
import { useMe } from "@/components/me-context";
import { CurrentPositionHolder, formatThaiDate, getCurrentPositionHolders } from "@/lib/api";
import { sourceLabel } from "@/lib/current-positions";
import { can } from "@/lib/permissions";

const dateOrDash = (value: string | null) => (value ? formatThaiDate(value) : "—");

// หน้านี้แสดงผู้ดำรงตำแหน่งบริหาร ณ วันนี้ คนที่เห็นคือคนเดียวกับที่ระบบผูกให้แถวโดยตำแหน่งในเอกสารที่ออกวันนี้
export default function PositionsPage() {
  const { me } = useMe();
  const [holders, setHolders] = useState<CurrentPositionHolder[] | null>(null);
  const [error, setError] = useState("");
  const today = new Date().toISOString();

  useEffect(() => {
    let active = true;
    getCurrentPositionHolders()
      .then((response) => { if (active) setHolders(response.data); })
      .catch((reason: Error) => { if (active) { setHolders([]); setError(reason.message); } });
    return () => { active = false; };
  }, []);

  const editLink = can(me?.role ?? "", "createPerson")
    ? <Link href="/settings/teachers" className="focus-ring inline-flex h-10 items-center gap-2 rounded-lg border border-[var(--accent)] px-3.5 text-[13px] font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]"><PencilSimple size={16} />แก้ไขในข้อมูลอาจารย์</Link>
    : null;

  return <AppShell breadcrumb="หน้าหลัก / บุคลากรโดยตำแหน่ง" title="บุคลากรโดยตำแหน่ง" description="ผู้ดำรงตำแหน่งบริหารในปัจจุบัน ใช้ตรวจว่าแถวโดยตำแหน่งในเอกสารผูกกับคนที่ถูกต้อง">
    <section className="card overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div><h2 className="text-lg font-semibold text-[var(--navy)]">ผู้ดำรงตำแหน่ง</h2><p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">ข้อมูล ณ วันที่ {formatThaiDate(today)}</p></div>
        {editLink}
      </div>
      {error && <p role="alert" className="px-5 py-3 text-sm text-[var(--danger)]">{error}</p>}
      {holders === null ? <p role="status" className="flex items-center gap-2 px-5 py-8 text-sm text-[var(--muted)]"><SpinnerGap size={18} className="animate-spin" />กำลังโหลด...</p>
        : <div className="overflow-x-auto"><table className="data-grid w-full min-w-[680px] text-left text-sm"><caption className="sr-only">ผู้ดำรงตำแหน่งบริหาร ณ วันที่ {formatThaiDate(today)}</caption>
          <thead><tr><th scope="col" className="px-5 py-3">ตำแหน่ง</th><th scope="col" className="px-4 py-3">ชื่อ-สกุล</th><th scope="col" className="px-4 py-3">วันเริ่ม</th><th scope="col" className="px-4 py-3">วันสิ้นสุด</th><th scope="col" className="px-5 py-3">ที่มา</th></tr></thead>
          <tbody>{holders.map((holder) => <tr key={`${holder.positionTitle}-${holder.directoryUserId}`}>
            <td className="px-5 py-3 font-medium text-[var(--ink)]">{holder.positionTitle}</td>
            <td className="px-4 py-3">{holder.fullName || "—"}</td>
            <td className="whitespace-nowrap px-4 py-3">{dateOrDash(holder.startsOn)}</td>
            <td className="whitespace-nowrap px-4 py-3">{dateOrDash(holder.endsOn)}</td>
            <td className="px-5 py-3 text-[var(--muted)]">{sourceLabel(holder.source)}</td>
          </tr>)}{holders.length === 0 && !error && <tr><td colSpan={5} className="px-5 py-10 text-center text-sm text-[var(--muted)]">ยังไม่มีข้อมูลผู้ดำรงตำแหน่ง ข้อมูลมาจากการ sync รายชื่อของวิทยาลัย หรือการกรอกในหน้าข้อมูลอาจารย์</td></tr>}</tbody>
        </table></div>}
    </section>
  </AppShell>;
}
