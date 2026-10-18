"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { KeyboardEvent, Suspense, useEffect, useId, useState, useSyncExternalStore } from "react";
import { ArrowLeft, ClockCounterClockwise, FilePdf, MagnifyingGlass, SpinnerGap, User } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";
import { ExportReportButton } from "@/components/export-report-button";
import { OrderSuggestion, PersonHistory, PersonSummary, formatThaiDate, getPersonHistory, getSearchSuggestions, getTopPeople, statusLabel } from "@/lib/api";
import { initials } from "@/lib/person";
import { AppointmentState, Period, RecentSearch, appointmentStateLabel, fiscalYearRange, moveSelection, periodRange, rememberSearch } from "@/lib/search";

const recentKey = "docflow.recentSearches";
const chipClass = (active: boolean) => `focus-ring min-h-10 rounded-full border px-4 text-xs ${active ? "border-[var(--accent)] bg-[var(--accent)] font-medium text-white" : "border-[var(--line-strong)] bg-white hover:border-[var(--accent)]"}`;
const stateTone: Record<AppointmentState, string> = {
  ACTIVE: "bg-[var(--success-soft)] text-[var(--success)]",
  EXPIRING: "bg-[#ffead6] text-[#9a4a0c]",
  EXPIRED: "bg-[var(--line-soft)] text-[var(--muted)]",
  CANCELLED: "bg-[var(--danger-soft)] text-[var(--danger)]",
};

// ค้นหาล่าสุดเก็บในเบราว์เซอร์ของแต่ละคน อ่านหรือเขียนไม่ได้ (โหมดส่วนตัว) ก็ใช้หน้าได้ตามปกติ
function readRecent(): RecentSearch[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(recentKey) ?? "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

// ค้นหาล่าสุดเป็น store เล็กๆ ฝั่งเบราว์เซอร์ ตอน render บนเซิร์ฟเวอร์และตอน hydrate ใช้รายการว่าง หน้าเว็บจึงไม่ขัดกัน
const noRecent: RecentSearch[] = [];
const recentListeners = new Set<() => void>();
let recentCache: RecentSearch[] | null = null;

function recentSnapshot() {
  recentCache ??= readRecent();
  return recentCache;
}

function subscribeRecent(listener: () => void) {
  recentListeners.add(listener);
  return () => { recentListeners.delete(listener); };
}

function addRecent(entry: RecentSearch) {
  recentCache = rememberSearch(recentSnapshot(), entry);
  try { localStorage.setItem(recentKey, JSON.stringify(recentCache)); } catch { /* ไม่มีที่เก็บ ไม่เป็นไร */ }
  recentListeners.forEach((listener) => listener());
}

export default function SearchPage() {
  return <AppShell breadcrumb="หน้าหลัก / ค้นหา" title="ค้นหาคำสั่งแต่งตั้ง" description="พิมพ์ชื่อคน เลขที่คำสั่ง หรือชื่อเรื่อง แล้วเลือกจากรายการ">
    <Suspense fallback={<p role="status" className="flex items-center gap-2 text-sm text-[var(--muted)]"><SpinnerGap size={18} className="animate-spin" />กำลังโหลด...</p>}>
      <SearchView />
    </Suspense>
  </AppShell>;
}

function SearchView() {
  const router = useRouter();
  const personId = Number(useSearchParams().get("person")) || 0;
  const recent = useSyncExternalStore(subscribeRecent, recentSnapshot, () => noRecent);

  const pick = (entry: RecentSearch) => {
    addRecent(entry);
    router.push(entry.kind === "person" ? `/search?person=${entry.id}` : `/documents/${entry.id}`);
  };

  return <div className="space-y-6">
    <SearchBox key={`search-${personId}`} onPick={pick} />
    {personId ? <PersonView key={`person-${personId}`} id={personId} /> : <StartView recent={recent} />}
  </div>;
}

type Option = { kind: "person"; item: PersonSummary } | { kind: "order"; item: OrderSuggestion };

function SearchBox({ onPick }: { onPick: (entry: RecentSearch) => void }) {
  const listId = useId();
  const [query, setQuery] = useState("");
  // ผลค้นหาจำไว้คู่กับคำค้นที่ใช้ ผลของคำเก่าจึงไม่ถูกเปิดตอนกด Enter ระหว่างรอผลใหม่
  const [results, setResults] = useState<{ query: string; people: PersonSummary[]; orders: OrderSuggestion[] } | null>(null);
  const [selected, setSelected] = useState(-1);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const text = query.trim();
    if (!text) return;
    let active = true;
    const timer = setTimeout(() => {
      getSearchSuggestions(text).then((response) => { if (active) { setResults({ query: text, ...response.data }); setSelected(-1); } }).catch(() => { if (active) setResults({ query: text, people: [], orders: [] }); });
    }, 200);
    return () => { active = false; clearTimeout(timer); };
  }, [query]);

  const shown = results && results.query === query.trim() ? results : null;
  const options: Option[] = shown ? [...shown.people.map((item) => ({ kind: "person" as const, item })), ...shown.orders.map((item) => ({ kind: "order" as const, item }))] : [];
  const choose = (option: Option) => {
    setOpen(false);
    onPick(option.kind === "person" ? { kind: "person", id: option.item.id, label: option.item.name } : { kind: "order", id: option.item.id, label: option.item.orderNo || option.item.title });
  };
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault(); setOpen(true);
      setSelected((current) => moveSelection(current, event.key === "ArrowDown" ? 1 : -1, options.length));
    } else if (event.key === "Enter" && options.length) {
      event.preventDefault(); choose(options[Math.max(selected, 0)]);
    } else if (event.key === "Escape") {
      setOpen(false);
    }
  };
  const optionId = (index: number) => `${listId}-option-${index}`;
  const renderOption = (option: Option, index: number) => <li key={`${option.kind}-${option.item.id}`} id={optionId(index)} role="option" aria-selected={selected === index} onMouseDown={(event) => event.preventDefault()} onClick={() => choose(option)} onMouseEnter={() => setSelected(index)} className={`flex cursor-pointer items-center gap-3 px-4 py-2.5 ${selected === index ? "bg-[var(--brand-blue-pale)]" : ""}`}>
    {option.kind === "person" ? <>
      <span aria-hidden="true" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--accent-soft)] text-xs font-medium text-[var(--navy)]">{initials(option.item.name)}</span>
      <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium text-[var(--ink)]">{option.item.name}</span><span className="block truncate text-xs text-[var(--muted)]">{option.item.position || "ไม่ระบุตำแหน่ง"}</span></span>
      <span className="shrink-0 text-xs text-[var(--muted)]">{option.item.orders.toLocaleString("th-TH")} คำสั่ง</span>
    </> : <>
      <span aria-hidden="true" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[var(--accent)]"><FilePdf size={17} weight="duotone" /></span>
      <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium text-[var(--ink)]">{option.item.orderNo || "ไม่มีเลขที่"} · {option.item.title}</span><span className="block truncate text-xs text-[var(--muted)]">{formatThaiDate(option.item.issuedDate)} · {statusLabel(option.item.status)}</span></span>
    </>}
  </li>;

  return <section className="card relative p-4 sm:p-5">
    <div className="relative">
      <MagnifyingGlass size={19} aria-hidden="true" className="absolute left-3.5 top-1/2 -translate-y-1/2 text-[#8a9aaa]" />
      <input value={query} onChange={(event) => { setQuery(event.target.value); setOpen(true); }} onFocus={() => setOpen(true)} onBlur={() => setOpen(false)} onKeyDown={onKeyDown}
        role="combobox" aria-expanded={open && Boolean(shown)} aria-controls={listId} aria-autocomplete="list" aria-activedescendant={selected >= 0 ? optionId(selected) : undefined} aria-label="ค้นหาชื่อคน เลขที่คำสั่ง หรือชื่อเรื่อง"
        placeholder="ค้นหาชื่อคน เลขที่คำสั่ง หรือชื่อเรื่อง" autoComplete="off" className="focus-ring h-12 w-full rounded-full border-0 bg-[var(--pill)] pl-11 pr-4 text-sm text-[var(--navy)] outline-none placeholder:text-[var(--subtle)]" />
    </div>
    {open && shown && <div id={listId} role="listbox" aria-label="ผลการค้นหา" className="absolute inset-x-4 top-full z-20 -mt-1 max-h-[60vh] overflow-y-auto rounded-xl border border-[var(--line)] bg-white py-2 shadow-lg sm:inset-x-5">
      {options.length === 0 && <p className="px-4 py-3 text-sm text-[var(--muted)]">ไม่พบคนหรือคำสั่งที่ตรงกับ &quot;{query.trim()}&quot;</p>}
      {shown.people.length > 0 && <ul role="group" aria-label="คน"><li role="presentation" className="px-4 pb-1 pt-2 text-xs font-medium text-[var(--muted)]">คน</li>{shown.people.map((item, index) => renderOption({ kind: "person", item }, index))}</ul>}
      {shown.orders.length > 0 && <ul role="group" aria-label="คำสั่ง"><li role="presentation" className="px-4 pb-1 pt-3 text-xs font-medium text-[var(--muted)]">คำสั่ง</li>{shown.orders.map((item, index) => renderOption({ kind: "order", item }, shown.people.length + index))}</ul>}
      <p className="hidden border-t border-[var(--line-soft)] px-4 pt-2 text-[11px] text-[var(--subtle)] sm:block">↑↓ เลือก · Enter เปิด · Esc ปิด</p>
    </div>}
  </section>;
}

function StartView({ recent }: { recent: RecentSearch[] }) {
  const fiscal = fiscalYearRange(new Date());
  const [top, setTop] = useState<PersonSummary[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    const { from, to } = fiscalYearRange(new Date());
    getTopPeople({ from, to }).then((response) => { if (active) setTop(response.data); }).catch((reason: Error) => { if (active) { setTop([]); setError(reason.message); } });
    return () => { active = false; };
  }, []);

  return <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
    <section className="card overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div><h2 className="text-lg font-semibold text-[var(--navy)]">ได้รับแต่งตั้งมากที่สุด · {fiscal.label}</h2><p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">นับจากคำสั่งที่ยืนยันแล้ว ตั้งแต่ {formatThaiDate(fiscal.from)}</p></div>
        <ExportReportButton filters={{ from: fiscal.from, to: fiscal.to }} label={`ส่งออกรายงาน${fiscal.label}`} />
      </div>
      {error && <p role="alert" className="px-5 py-3 text-sm text-[var(--danger)]">{error}</p>}
      {top === null ? <p role="status" className="flex items-center gap-2 px-5 py-8 text-sm text-[var(--muted)]"><SpinnerGap size={18} className="animate-spin" />กำลังโหลด...</p>
        : <div className="overflow-x-auto"><table className="data-grid w-full text-left text-sm"><caption className="sr-only">คนที่ได้รับแต่งตั้งมากที่สุดใน{fiscal.label}</caption>
          <thead><tr><th scope="col" className="w-12 px-4 py-3 sm:px-5">#</th><th scope="col" className="px-4 py-3">ชื่อ</th><th scope="col" className="hidden px-4 py-3 sm:table-cell">ตำแหน่ง</th><th scope="col" className="px-5 py-3 text-right">คำสั่ง</th></tr></thead>
          <tbody>{top.map((person, index) => <tr key={person.id} className="hover:bg-[var(--brand-blue-pale)]">
            <td className="px-5 py-3 text-[var(--muted)]">{index + 1}</td>
            <td className="px-4 py-3"><Link href={`/search?person=${person.id}`} className="focus-ring rounded font-medium text-[var(--accent)] hover:underline">{person.name}</Link></td>
            <td className="hidden px-4 py-3 text-[var(--muted)] sm:table-cell">{person.position || "-"}</td>
            <td className="px-5 py-3 text-right font-medium">{person.orders.toLocaleString("th-TH")}</td>
          </tr>)}{top.length === 0 && !error && <tr><td colSpan={4} className="px-5 py-10 text-center text-sm text-[var(--muted)]">ยังไม่มีคำสั่งที่ยืนยันแล้วใน{fiscal.label}</td></tr>}</tbody>
        </table></div>}
    </section>
    <section className="card overflow-hidden">
      <div className="border-b border-[var(--line)] px-5 py-4"><h2 className="text-lg font-semibold text-[var(--navy)]">ค้นหาล่าสุด</h2><p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">เก็บไว้ในเบราว์เซอร์นี้เท่านั้น</p></div>
      {recent.length === 0 ? <p className="px-5 py-6 text-sm text-[var(--muted)]">ยังไม่มีรายการ ลองพิมพ์ชื่อคนหรือเลขที่คำสั่งด้านบน</p>
        : <ul className="divide-y divide-[var(--line-soft)]">{recent.map((entry) => <li key={`${entry.kind}-${entry.id}`}><Link href={entry.kind === "person" ? `/search?person=${entry.id}` : `/documents/${entry.id}`} className="focus-ring flex min-h-11 items-center gap-3 px-5 py-2 text-sm hover:bg-[var(--brand-blue-pale)]">
          {entry.kind === "person" ? <User size={17} className="shrink-0 text-[var(--muted)]" /> : <ClockCounterClockwise size={17} className="shrink-0 text-[var(--muted)]" />}
          <span className="min-w-0 truncate">{entry.label}</span>
        </Link></li>)}</ul>}
    </section>
  </div>;
}

function PersonView({ id }: { id: number }) {
  const [period, setPeriod] = useState<Period>("this");
  const [custom, setCustom] = useState({ from: "", to: "" });
  const [activeOnly, setActiveOnly] = useState(false);
  const [history, setHistory] = useState<PersonHistory | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const fiscal = fiscalYearRange(new Date());
  const lastFiscal = fiscalYearRange(new Date(), -1);

  useEffect(() => {
    let active = true;
    getPersonHistory(id, periodRange(period, new Date(), custom), activeOnly).then((response) => { if (active) { setHistory(response.data); setError(""); } })
      .catch((reason: Error) => { if (active) setError(reason.message); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [id, period, custom, activeOnly]);

  const choosePeriod = (value: Period) => { setLoading(true); setPeriod(value); };
  const person = history?.person;
  const periods: [Period, string][] = [["all", "ทั้งหมด"], ["this", `ปีงบนี้ (${fiscal.label.replace("ปีงบ ", "")})`], ["last", `ปีงบก่อน (${lastFiscal.label.replace("ปีงบ ", "")})`], ["custom", "กำหนดเอง"]];

  return <div className="space-y-6">
    <Link href="/search" className="focus-ring inline-flex min-h-10 items-center gap-1.5 rounded-md text-sm text-[var(--accent)] hover:underline"><ArrowLeft size={16} />กลับไปหน้าค้นหา</Link>
    {error && <p role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm text-[var(--danger)]">{error}</p>}
    {person && <section className="card flex flex-col gap-4 p-5 sm:flex-row sm:items-center">
      <span aria-hidden="true" className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-[var(--accent-soft)] text-lg font-medium text-[var(--navy)]">{initials(person.name)}</span>
      <div className="min-w-0 flex-1">
        <h2 className="break-words text-xl font-semibold text-[var(--navy)]">{person.name}</h2>
        <p className="mt-0.5 text-sm text-[var(--muted)]">{person.position || "ไม่ระบุตำแหน่ง"}</p>
        <p className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm"><span><b className="font-semibold">{person.orders.toLocaleString("th-TH")}</b> คำสั่ง</span><span className="text-[var(--success)]">ยังมีผล {person.active.toLocaleString("th-TH")}</span><span className="text-[#9a4a0c]">ใกล้หมดวาระ {person.expiring.toLocaleString("th-TH")}</span></p>
      </div>
      <ExportReportButton filters={{ person: id, ...periodRange(period, new Date(), custom), activeOnly }} label="ส่งออกรายงานของคนนี้" />
    </section>}

    <section className="card overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-[var(--line)] bg-[var(--surface-muted)] px-5 py-4">
        <div role="group" aria-label="ช่วงวันที่ออกคำสั่ง" className="flex flex-wrap gap-2">{periods.map(([value, label]) => <button key={value} type="button" aria-pressed={period === value} onClick={() => choosePeriod(value)} className={chipClass(period === value)}>{label}</button>)}</div>
        {period === "custom" && <div className="flex flex-wrap items-center gap-3 text-xs text-[var(--muted)]">
          <label className="flex items-center gap-2">ตั้งแต่<input type="date" value={custom.from} onChange={(event) => { setLoading(true); setCustom({ ...custom, from: event.target.value }); }} className="focus-ring h-10 rounded-lg border border-[var(--line)] bg-white px-2 text-sm" /></label>
          <label className="flex items-center gap-2">ถึง<input type="date" value={custom.to} onChange={(event) => { setLoading(true); setCustom({ ...custom, to: event.target.value }); }} className="focus-ring h-10 rounded-lg border border-[var(--line)] bg-white px-2 text-sm" /></label>
        </div>}
        <label className="flex min-h-10 w-fit cursor-pointer items-center gap-2 text-sm"><input type="checkbox" checked={activeOnly} onChange={(event) => { setLoading(true); setActiveOnly(event.target.checked); }} className="h-4 w-4" />เฉพาะที่ยังมีผล</label>
      </div>
      <div aria-busy={loading} className="overflow-x-auto"><table className="data-grid w-full min-w-[760px] text-left text-sm"><caption className="sr-only">คำสั่งแต่งตั้งของ {person?.name}</caption>
        <thead><tr><th scope="col" className="px-5 py-3">คำสั่ง</th><th scope="col" className="px-4 py-3">บทบาท</th><th scope="col" className="px-4 py-3">วันที่ออก</th><th scope="col" className="px-4 py-3">ครบวาระ</th><th scope="col" className="px-5 py-3">สถานะ</th></tr></thead>
        <tbody>{history?.appointments.map((row) => <tr key={row.documentId} className="hover:bg-[var(--brand-blue-pale)]">
          <td className="px-5 py-3"><Link href={`/documents/${row.documentId}`} className="focus-ring block rounded"><span className="block font-medium text-[var(--ink)]">{row.orderNo || "ไม่มีเลขที่"}</span><span className="block max-w-[360px] truncate text-xs text-[var(--muted)]">{row.title}</span></Link></td>
          <td className="px-4 py-3">{row.role || "-"}</td>
          <td className="whitespace-nowrap px-4 py-3">{formatThaiDate(row.issuedDate)}</td>
          <td className="whitespace-nowrap px-4 py-3">{row.expiryDate ? formatThaiDate(row.expiryDate) : "-"}</td>
          <td className="px-5 py-3"><span className={`inline-flex whitespace-nowrap rounded-full px-2.5 py-1 text-xs font-medium ${stateTone[row.state]}`}>{appointmentStateLabel(row.state)}</span></td>
        </tr>)}{!loading && history?.appointments.length === 0 && <tr><td colSpan={5} className="px-5 py-12 text-center text-sm text-[var(--muted)]">ไม่มีคำสั่งในช่วงนี้{period !== "all" && <><br /><button type="button" onClick={() => choosePeriod("all")} className="focus-ring mt-2 min-h-10 text-[var(--accent)] underline underline-offset-4">ดูทุกช่วงเวลา</button></>}</td></tr>}</tbody>
      </table></div>
      {loading && <p role="status" className="flex items-center gap-2 border-t border-[var(--line)] px-5 py-3 text-xs text-[var(--muted)]"><SpinnerGap size={16} className="animate-spin" />กำลังโหลด...</p>}
    </section>
  </div>;
}
