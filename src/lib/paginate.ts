export type Page<T> = { items: T[]; page: number; pageCount: number; from: number; to: number; total: number };

// เลขหน้าที่เกินจำนวนจริง (เช่นหลังกรองรายการ) จะถูกดึงกลับมาอยู่ในช่วงที่มี
export function paginate<T>(items: T[], page: number, size: number): Page<T> {
  const pageCount = Math.max(1, Math.ceil(items.length / size));
  const current = Math.min(pageCount, Math.max(1, page));
  const start = (current - 1) * size;
  const slice = items.slice(start, start + size);
  return { items: slice, page: current, pageCount, from: slice.length ? start + 1 : 0, to: start + slice.length, total: items.length };
}
