import { describe, expect, test } from "vitest";
import type { ActionForJoin } from "./actionComparison";
import {
  actionDurationMs,
  actionOutcome,
  buildJoinedRows,
  joinKey,
} from "./actionComparison";

function action(
  label: string,
  opts: Partial<Omit<ActionForJoin, "label">> = {},
): ActionForJoin {
  return {
    id: label,
    label,
    type: null,
    primaryOutput: null,
    success: true,
    ...opts,
  };
}

describe("joinKey", () => {
  test("uses label, type, and primaryOutput separated by ||", () => {
    expect(
      joinKey(
        action("//foo:bar", { type: "GoCompile", primaryOutput: "foo.a" }),
      ),
    ).toBe("//foo:bar||GoCompile||foo.a");
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

describe("actionOutcome", () => {
  test("success, exit code, or null when absent", () => {
    expect(actionOutcome(action("//a:a"))).toBe("success");
    expect(
      actionOutcome(action("//a:a", { success: false, exitCode: 2 })),
    ).toBe("exit 2");
    expect(actionOutcome(undefined)).toBeNull();
  });
});

describe("actionDurationMs", () => {
  test("end minus start, null when missing or negative", () => {
    expect(
      actionDurationMs(
        action("//a:a", {
          startTime: "2026-01-01T00:00:00Z",
          endTime: "2026-01-01T00:00:01.5Z",
        }),
      ),
    ).toBe(1500);
    expect(actionDurationMs(action("//a:a"))).toBeNull();
    expect(
      actionDurationMs(
        action("//a:a", {
          startTime: "2026-01-01T00:00:01Z",
          endTime: "2026-01-01T00:00:00Z",
        }),
      ),
    ).toBeNull();
  });
});

describe("buildJoinedRows", () => {
  test("empty inputs → empty output", () => {
    expect(buildJoinedRows([], [])).toEqual([]);
  });

  test("action present on both sides → onLeft and onRight true", () => {
    const a = action("//foo:bar", { type: "GoCompile", success: true });
    const b = action("//foo:bar", {
      type: "GoCompile",
      success: false,
      exitCode: 1,
    });
    const rows = buildJoinedRows([a], [b]);
    expect(rows).toHaveLength(1);
    expect(rows[0].onLeft).toBe(true);
    expect(rows[0].onRight).toBe(true);
    expect(rows[0].leftOutcome).toBe("success");
    expect(rows[0].rightOutcome).toBe("exit 1");
  });

  test("action only on left → onRight false", () => {
    const rows = buildJoinedRows([action("//a:a")], []);
    expect(rows[0].onLeft).toBe(true);
    expect(rows[0].onRight).toBe(false);
    expect(rows[0].rightOutcome).toBeNull();
  });

  test("action only on right → onLeft false", () => {
    const rows = buildJoinedRows([], [action("//b:b")]);
    expect(rows[0].onLeft).toBe(false);
    expect(rows[0].onRight).toBe(true);
    expect(rows[0].leftOutcome).toBeNull();
  });

  test("differing-outcome matched rows sort before same-outcome matched rows", () => {
    const leftActions = [
      action("//same:same", { type: "T", success: true }),
      action("//diff:diff", { type: "T", success: true }),
    ];
    const rightActions = [
      action("//same:same", { type: "T", success: true }),
      action("//diff:diff", { type: "T", success: false, exitCode: 1 }),
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
      action("//match:a", { type: "T", success: true }),
      action("//left-only:b", { type: "T" }),
    ];
    const right = [
      action("//match:a", { type: "T", success: true }),
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
