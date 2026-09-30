import { describe, expect, test } from "vitest";
import { buildJoinedRows, joinKey } from "./actionComparison";
import type { ActionForJoin } from "./actionComparison";

function action(
  label: string,
  opts: Partial<Omit<ActionForJoin, "label">> = {},
): ActionForJoin {
  return { id: label, label, type: opts.type ?? null, primaryOutput: opts.primaryOutput ?? null, cacheStatus: opts.cacheStatus ?? null };
}

describe("joinKey", () => {
  test("uses label, type, and primaryOutput separated by ||", () => {
    expect(joinKey(action("//foo:bar", { type: "GoCompile", primaryOutput: "foo.a" }))).toBe(
      "//foo:bar||GoCompile||foo.a",
    );
  });

  test("null type and primaryOutput become empty strings", () => {
    expect(joinKey(action("//foo:bar"))).toBe("//foo:bar||||");
  });

  test("same label + different type → different keys", () => {
    const k1 = joinKey(action("//foo:bar", { type: "GoCompile" }));
    const k2 = joinKey(action("//foo:bar", { type: "GoLink" }));
    expect(k1).not.toBe(k2);
  });
});

describe("buildJoinedRows", () => {
  test("empty inputs → empty output", () => {
    expect(buildJoinedRows([], [])).toEqual([]);
  });

  test("action present on both sides → onLeft and onRight true", () => {
    const a = action("//foo:bar", { type: "GoCompile", cacheStatus: "remote cache hit" });
    const b = action("//foo:bar", { type: "GoCompile", cacheStatus: "local" });
    const rows = buildJoinedRows([a], [b]);
    expect(rows).toHaveLength(1);
    expect(rows[0].onLeft).toBe(true);
    expect(rows[0].onRight).toBe(true);
    expect(rows[0].leftStatus).toBe("remote cache hit");
    expect(rows[0].rightStatus).toBe("local");
  });

  test("action only on left → onRight false", () => {
    const rows = buildJoinedRows([action("//a:a")], []);
    expect(rows[0].onLeft).toBe(true);
    expect(rows[0].onRight).toBe(false);
    expect(rows[0].rightStatus).toBeNull();
  });

  test("action only on right → onLeft false", () => {
    const rows = buildJoinedRows([], [action("//b:b")]);
    expect(rows[0].onLeft).toBe(false);
    expect(rows[0].onRight).toBe(true);
    expect(rows[0].leftStatus).toBeNull();
  });

  test("differing-status matched rows sort before same-status matched rows", () => {
    const leftActions = [
      action("//same:same", { type: "T", cacheStatus: "remote cache hit" }),
      action("//diff:diff", { type: "T", cacheStatus: "remote cache hit" }),
    ];
    const rightActions = [
      action("//same:same", { type: "T", cacheStatus: "remote cache hit" }),
      action("//diff:diff", { type: "T", cacheStatus: "local" }),
    ];
    const rows = buildJoinedRows(leftActions, rightActions);
    expect(rows[0].label).toBe("//diff:diff"); // differs → first
    expect(rows[1].label).toBe("//same:same"); // matches → second
  });

  test("matched rows sort before one-sided rows", () => {
    const rows = buildJoinedRows(
      [action("//both:both"), action("//left-only:x")],
      [action("//both:both")],
    );
    expect(rows[0].label).toBe("//both:both");
    expect(rows[1].label).toBe("//left-only:x");
  });

  test("actions with different type are not joined", () => {
    const rows = buildJoinedRows(
      [action("//foo:foo", { type: "GoCompile" })],
      [action("//foo:foo", { type: "GoLink" })],
    );
    expect(rows).toHaveLength(2);
    expect(rows.every((r) => !r.onLeft || !r.onRight)).toBe(true);
  });

  test("coverage: all three cases present", () => {
    const left = [
      action("//match:a", { type: "T", cacheStatus: "remote" }),
      action("//left-only:b", { type: "T" }),
    ];
    const right = [
      action("//match:a", { type: "T", cacheStatus: "remote" }),
      action("//right-only:c", { type: "T" }),
    ];
    const rows = buildJoinedRows(left, right);
    const matched = rows.filter((r) => r.onLeft && r.onRight);
    const leftOnly = rows.filter((r) => r.onLeft && !r.onRight);
    const rightOnly = rows.filter((r) => !r.onLeft && r.onRight);
    expect(matched).toHaveLength(1);
    expect(leftOnly).toHaveLength(1);
    expect(rightOnly).toHaveLength(1);
  });
});
