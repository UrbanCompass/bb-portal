export interface ActionForJoin {
  id: string;
  label: string;
  type?: string | null;
  primaryOutput?: string | null;
  success?: boolean | null;
  exitCode?: number | null;
  startTime?: string | null;
  endTime?: string | null;
}

export interface JoinedActionRow {
  key: string;
  label: string;
  mnemonic: string | null | undefined;
  primaryOutput: string | null | undefined;
  leftOutcome: string | null;
  rightOutcome: string | null;
  leftDurationMs: number | null;
  rightDurationMs: number | null;
  onLeft: boolean;
  onRight: boolean;
  // Side that took much longer, which usually means it missed the cache.
  slowerSide: "left" | "right" | null;
}

// A matched action is flagged as a duration jump when the slower side took at
// least DURATION_JUMP_RATIO times as long and at least DURATION_JUMP_MIN_MS.
export const DURATION_JUMP_RATIO = 10;
export const DURATION_JUMP_MIN_MS = 1000;

export function slowerSide(
  leftMs: number | null,
  rightMs: number | null,
): "left" | "right" | null {
  if (leftMs === null || rightMs === null) return null;
  const slow = Math.max(leftMs, rightMs);
  const fast = Math.min(leftMs, rightMs);
  if (slow < DURATION_JUMP_MIN_MS || slow < fast * DURATION_JUMP_RATIO) {
    return null;
  }
  return leftMs > rightMs ? "left" : "right";
}

export function joinKey(a: ActionForJoin): string {
  return `${a.label}||${a.type ?? ""}||${a.primaryOutput ?? ""}`;
}

// actionOutcome summarizes how an action finished: "success", or the exit
// code for failed actions.
export function actionOutcome(a: ActionForJoin | undefined): string | null {
  if (!a) return null;
  if (a.success) return "success";
  return `exit ${a.exitCode ?? "?"}`;
}

export function actionDurationMs(a: ActionForJoin | undefined): number | null {
  if (!a?.startTime || !a.endTime) return null;
  const ms = Date.parse(a.endTime) - Date.parse(a.startTime);
  return Number.isNaN(ms) || ms < 0 ? null : ms;
}

export function buildJoinedRows(
  leftActions: ActionForJoin[],
  rightActions: ActionForJoin[],
): JoinedActionRow[] {
  const leftMap = new Map<string, ActionForJoin>();
  for (const a of leftActions) leftMap.set(joinKey(a), a);

  const rightMap = new Map<string, ActionForJoin>();
  for (const a of rightActions) rightMap.set(joinKey(a), a);

  const allKeys = new Set([...leftMap.keys(), ...rightMap.keys()]);
  const rows: JoinedActionRow[] = [];

  for (const k of allKeys) {
    const l = leftMap.get(k);
    const r = rightMap.get(k);
    const ref = (l ?? r) as ActionForJoin;
    const leftDurationMs = actionDurationMs(l);
    const rightDurationMs = actionDurationMs(r);
    rows.push({
      key: k,
      label: ref.label,
      mnemonic: ref.type,
      primaryOutput: ref.primaryOutput,
      leftOutcome: actionOutcome(l),
      rightOutcome: actionOutcome(r),
      leftDurationMs,
      rightDurationMs,
      onLeft: !!l,
      onRight: !!r,
      slowerSide: slowerSide(leftDurationMs, rightDurationMs),
    });
  }

  // Differing-outcome matched rows first, then duration jumps, then the rest
  // of the matched rows, then one-sided.
  const rank = (r: JoinedActionRow) => {
    if (!r.onLeft || !r.onRight) return 3;
    if (r.leftOutcome !== r.rightOutcome) return 0;
    if (r.slowerSide) return 1;
    return 2;
  };
  rows.sort((a, b) => rank(a) - rank(b) || a.label.localeCompare(b.label));

  return rows;
}

export interface MissReasonCount {
  reason: string;
  count?: number | null;
}

export interface MissReasonRow {
  reason: string;
  left: number;
  right: number;
}

// buildMissReasonRows lines up action cache miss counts by reason, largest
// change first.
export function buildMissReasonRows(
  left: MissReasonCount[],
  right: MissReasonCount[],
): MissReasonRow[] {
  const byReason = new Map<string, MissReasonRow>();
  const get = (reason: string) => {
    let row = byReason.get(reason);
    if (!row) {
      row = { reason, left: 0, right: 0 };
      byReason.set(reason, row);
    }
    return row;
  };
  for (const m of left) get(m.reason).left += m.count ?? 0;
  for (const m of right) get(m.reason).right += m.count ?? 0;
  return [...byReason.values()]
    .filter((r) => r.left > 0 || r.right > 0)
    .sort(
      (a, b) =>
        Math.abs(b.right - b.left) - Math.abs(a.right - a.left) ||
        a.reason.localeCompare(b.reason),
    );
}
