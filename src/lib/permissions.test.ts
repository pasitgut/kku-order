import { test } from "node:test";
import assert from "node:assert/strict";
import { PERMISSIONS, ROLES, can, roleLabel } from "./permissions.ts";

test("permissions mirror requireRole in backend/main.go", () => {
  assert.deepEqual(ROLES, ["ADMIN", "STAFF", "DEVELOPER", "VIEWER"]);
  assert.equal(can("VIEWER", "view"), true);
  assert.equal(can("VIEWER", "import"), false);
  assert.equal(can("STAFF", "confirm"), true);
  assert.equal(can("STAFF", "delete"), false);
  assert.equal(can("DEVELOPER", "delete"), true);
  assert.equal(can("ADMIN", "settings"), true);
  assert.equal(can("STAFF", "settings"), false);
});

test("roles are matched case-insensitively and unknown roles get nothing", () => {
  assert.equal(can("admin", "delete"), true);
  assert.equal(can("", "view"), false);
  assert.equal(can("GUEST", "view"), false);
});

test("every permission has a Thai label and role labels are readable", () => {
  assert.ok(PERMISSIONS.every((permission) => permission.label.length > 0));
  assert.equal(roleLabel("ADMIN"), "ผู้ดูแลระบบ");
  assert.equal(roleLabel("unknown"), "unknown");
});
