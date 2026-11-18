// มีหน้าถัดไปเมื่อยังมีแถวเหลือหลังหน้านี้ หน้าสุดท้ายที่เต็มพอดีจึงไม่มีหน้าถัดไป
export function hasNextPage(page: number, limit: number, total: number): boolean {
  return page * limit < total;
}
