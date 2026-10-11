import { test } from "node:test";
import assert from "node:assert/strict";
import { ocrAPIKeyPlaceholder } from "./ocr-api-key.ts";

test("placeholder shows the saved key's last four characters from the web", () => {
  assert.equal(ocrAPIKeyPlaceholder("web", "abcd"), "ตั้งไว้แล้ว ลงท้าย ••••abcd (จากหน้าเว็บ)");
});

test("placeholder says the env file supplies the key", () => {
  assert.equal(ocrAPIKeyPlaceholder("env", "wxyz"), "ตั้งไว้แล้ว ลงท้าย ••••wxyz (จากไฟล์ .env ของเซิร์ฟเวอร์)");
  assert.equal(ocrAPIKeyPlaceholder("env", ""), "ตั้งไว้แล้ว (จากไฟล์ .env ของเซิร์ฟเวอร์)");
});

test("placeholder says nothing is set when there is no key", () => {
  assert.equal(ocrAPIKeyPlaceholder("none", ""), "ยังไม่ได้ตั้ง");
});
