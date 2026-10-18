export type TeacherDraft = {
  prefix: string;
  firstName: string;
  lastName: string;
  positionTitle: string;
  department: string;
  email: string;
  phone: string;
  managePosition: string;
  managePositionSince: string;
};

// teacherPayload ตัดช่องว่างทุกช่อง และไม่ส่งตำแหน่งบริหารถ้าไม่ได้กรอก (วันที่เริ่มดำรงตำแหน่งไม่มีความหมายถ้าไม่มีตำแหน่ง)
export function teacherPayload(draft: TeacherDraft): Omit<TeacherDraft, "managePosition" | "managePositionSince"> & Partial<Pick<TeacherDraft, "managePosition" | "managePositionSince">> {
  const { managePosition, managePositionSince, ...rest } = Object.fromEntries(Object.entries(draft).map(([key, value]) => [key, value.trim()])) as TeacherDraft;
  return managePosition ? { ...rest, managePosition, managePositionSince } : rest;
}

// linkedRowsNote ต่อท้ายข้อความบันทึกสำเร็จ บอกว่าระบบผูกแถวโดยตำแหน่งในเอกสารที่รอยืนยันให้กี่แถว
export function linkedRowsNote(linked: number) {
  return linked > 0 ? ` และผูกให้ ${linked} แถวในเอกสารที่รอยืนยันแล้ว` : "";
}
